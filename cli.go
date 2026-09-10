// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/tim-janik/iris/adoc"
	"github.com/tim-janik/iris/frontmatter"
	"github.com/tim-janik/iris/globstar"
	"github.com/tim-janik/iris/mimetype"
	"github.com/tim-janik/iris/pandoc"
	"github.com/tim-janik/iris/serve"
	"github.com/tim-janik/iris/templates"
)

// ---------------------------------------------------------------------------
// Index subcommand — generate index.md lines from a list of .md files
// ---------------------------------------------------------------------------

type indexArgs struct {
	files []string
}

// parseIndexArgs parses command-line arguments for the index subcommand.
func parseIndexArgs() indexArgs {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s index <file.md> [file2.md ...]\n", os.Args[0])
		os.Exit(1)
	}
	return indexArgs{files: args}
}

// indexMain reads each .md file, extracts title and description from frontmatter
// (falling back to the first h1 for title), and prints index.md lines to stdout.
func indexMain() {
	args := parseIndexArgs()
	for _, filePath := range args.files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", filePath, err)
			continue
		}
		fm, body := frontmatter.Parse(data, filePath)

		title := fm.Title
		if title == "" {
			title = frontmatter.H1Title(body)
		}
		if title == "" {
			title = filepath.Base(strings.TrimSuffix(filePath, filepath.Ext(filePath)))
		}

		// Strip .md/.adoc extension for clean URLs (matches iris serve)
		link := filePath
		if ext := filepath.Ext(filePath); ext == ".md" || ext == ".adoc" {
			link = strings.TrimSuffix(filePath, ext)
		}

		if fm.Description != "" {
			fmt.Printf("- [%s](%s) — %s\n", title, link, fm.Description)
		} else {
			fmt.Printf("- [%s](%s)\n", title, link)
		}
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

type ssgArgs struct {
	clearOutput bool
	inputDir    string
	outputDir   string
	configFile  string    // explicit config file path (-c flag)
	templateDir string    // custom template directory (-t flag, overrides embedded)
	workers     int       // max concurrent pandoc/asciidoctor workers
	now         time.Time // override for time-dependent output (sitemap priorities/changefreq)
}

// parseSSGArgs parses command-line arguments for the ssg subcommand.
func parseSSGArgs() ssgArgs {
	fs := flag.NewFlagSet("ssg", flag.ExitOnError)
	clearOutput := fs.Bool("C", true, "clean output directory before building (default true)")
	configFile := fs.String("c", "", "path to site config file (TOML)")
	templateDir := fs.String("t", "", "custom template directory (overrides embedded templates)")
	workers := fs.Int("j", 0, "max concurrent pandoc/asciidoctor workers (0 = NumCPU)")
	nowFlag := fs.String("now", "", "override current time for time-dependent output (YYYY-MM-DD or RFC3339; default: IRIS_NOW env or real time)")
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s ssg [flags] <input-dir> <output-dir>\n", os.Args[0])
		os.Exit(1)
	}

	inputDir, err := filepath.Abs(args[0])
	if err != nil {
		log.Fatalf("input path: %v", err)
	}
	outputDir, err := filepath.Abs(args[1])
	if err != nil {
		log.Fatalf("output path: %v", err)
	}
	configPath := *configFile
	if configPath != "" {
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			log.Fatalf("config path: %v", err)
		}
	}
	templatePath := *templateDir
	if templatePath != "" {
		templatePath, err = filepath.Abs(templatePath)
		if err != nil {
			log.Fatalf("template path: %v", err)
		}
	}

	w := *workers
	if w < 0 {
		log.Fatalf("invalid worker count: %d", w)
	}
	if w == 0 {
		w = runtime.NumCPU()
	}

	return ssgArgs{clearOutput: *clearOutput, inputDir: inputDir, outputDir: outputDir, configFile: configPath, templateDir: templatePath, workers: w, now: parseNow(*nowFlag)}
}

// parseNow resolves the effective "current time" for a build: the -now flag
// wins, then the IRIS_NOW environment variable, then real time. Accepts
// YYYY-MM-DD or RFC3339. The result is normalized to UTC so time-dependent
// output (sitemap priorities/changefreq) is reproducible across machines.
func parseNow(nowFlag string) time.Time {
	value := nowFlag
	if value == "" {
		value = os.Getenv("IRIS_NOW")
	}
	if value == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(dateLayout, value); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC()
	}
	log.Fatalf("invalid -now value %q: use YYYY-MM-DD or RFC3339", value)
	return time.Time{}
}

// parseInitArgs parses command-line arguments for the init subcommand.
func parseInitArgs() string {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	path := fs.String("o", "", "output file (default: _siteconfig.toml; overwrites if exists)")
	fs.Parse(os.Args[2:])
	return *path
}

// ---------------------------------------------------------------------------
// Serve subcommand — HTTP server for on-the-fly markdown rendering
// ---------------------------------------------------------------------------

type serveArgs struct {
	root        string // directory containing markdown files
	port        int    // TCP port to listen on
	listenHost  string // host address to listen on
	record      string // record serve responses under this directory, then exit
	editLinkCmd string // command template for edit links (empty = disabled)
	templateDir string // custom template directory (overrides embedded templates)
	faviconPath string // path to favicon file served at /favicon.ico
}

// parseServeArgs parses command-line arguments for the serve subcommand.
func parseServeArgs() serveArgs {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 9454, "TCP port to listen on (default: 9454)")
	listenHost := fs.String("listen", "127.0.0.1", "host address to listen on (default: 127.0.0.1)")
	record := fs.String("record", "", "record serve responses for all served files under root into this directory (cleared first) and exit")
	editLinkCmd := fs.String("editlink", "", "command template to open source file in editor (empty = disabled); use %s for file path, %u for line number")
	templateDir := fs.String("t", "", "custom template directory (overrides embedded templates)")
	faviconPath := fs.String("favicon", "", "path to favicon file served at /favicon.ico")
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s serve [flags] <path>\n", os.Args[0])
		os.Exit(1)
	}

	root, _ := filepath.Abs(args[0])
	return serveArgs{root: root, port: *port, listenHost: *listenHost, record: *record, editLinkCmd: *editLinkCmd, templateDir: *templateDir, faviconPath: *faviconPath}
}

// serveMain is the main entry point for the serve subcommand.
func serveMain() {
	args := parseServeArgs()

	if _, err := os.Stat(args.root); os.IsNotExist(err) {
		log.Fatalf("root path does not exist: %s", args.root)
	}

	// Auto-detect config file in root directory; falls back to defaults if missing.
	configFile := ""
	if _, err := os.Stat(filepath.Join(args.root, defaultConfigPath)); err == nil {
		configFile = filepath.Join(args.root, defaultConfigPath)
	}
	site := loadSiteConfig(args.root, configFile)

	srv := &serve.Server{
		Root:            args.root,
		Port:            args.port,
		ListenHost:      args.listenHost,
		PandocConfig:    pandoc.DefaultConfig(),
		AdocConfig:      adoc.DefaultConfig(),
		EditLinkCmd:     args.editLinkCmd,
		TemplateDir:     args.templateDir,
		FaviconPath:     args.faviconPath,
		Site:            toTemplateSite(site),
		HighlightScript: highlightScriptAsset,
		HighlightStyle:  highlightStyleAsset,
		MermaidScript:   mermaidScriptAsset,
	}

	if args.record != "" {
		handler, err := srv.Handler()
		if err != nil {
			log.Fatalf("Server failed: %v", err)
		}
		if err := recordServe(handler, args.root, args.record); err != nil {
			log.Fatalf("Record failed: %v", err)
		}
		return
	}

	if err := srv.Serve(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// recordServe walks root and records the response body of every URL that
// iris serve answers, into outDir (cleared first). Paths mirror the served
// URLs verbatim: /2005/hello (converted from 2005/hello.md) is written as
// 2005/hello. Directories and non-passthrough files yield 404 in serve
// mode and are not recorded. The outDir itself is excluded from the walk.
func recordServe(handler http.Handler, root, outDir string) error {
	rootAbs, outAbs, err := validate_output_paths(root, outDir)
	if err != nil {
		return err
	}
	root, outDir = rootAbs, outAbs
	if err := os.RemoveAll(outAbs); err != nil {
		return fmt.Errorf("clear record dir: %w", err)
	}
	if err := os.MkdirAll(outAbs, 0755); err != nil {
		return err
	}
	seen := map[string]string{}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if abs, aerr := filepath.Abs(path); aerr == nil &&
				(abs == outAbs || strings.HasPrefix(abs, outAbs+string(os.PathSeparator))) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(slash))
		var urlPath string
		switch {
		case ext == ".md" || ext == ".adoc":
			urlPath = "/" + strings.TrimSuffix(slash, ext)
		case mimetype.IsPassthrough(ext):
			urlPath = "/" + slash
		default:
			return nil // serve answers 404 for these
		}
		if prev, ok := seen[urlPath]; ok {
			log.Printf("[skip] %s: same URL already served from %s", urlPath, prev)
			return nil
		}
		seen[urlPath] = rel

		req := httptest.NewRequest(http.MethodGet, (&url.URL{Path: urlPath}).String(), nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code >= http.StatusInternalServerError {
			return fmt.Errorf("record %s: server returned %d", urlPath, rec.Code)
		}
		if rec.Code != http.StatusOK {
			log.Printf("[%d] %s (not recorded)", rec.Code, urlPath)
			return nil
		}
		recordPath := strings.TrimPrefix(urlPath, "/")
		full := filepath.Join(outDir, filepath.FromSlash(recordPath))
		recordRel, err := filepath.Rel(outDir, full)
		if err != nil || !filepath.IsLocal(recordRel) || recordRel == "." {
			return fmt.Errorf("record path escapes output directory: %s", recordPath)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(full, rec.Body.Bytes(), 0644); err != nil {
			return err
		}
		log.Printf("[200] %s -> %s", urlPath, full)
		return nil
	})
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func pathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix []string
	for current := absPath; ; current = filepath.Dir(current) {
		realPath, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				realPath = filepath.Join(realPath, suffix[i])
			}
			return realPath, nil
		}
		if !os.IsNotExist(err) || filepath.Dir(current) == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
	}
}

func validateSSGArgs(args ssgArgs) error {
	if args.workers <= 0 {
		return fmt.Errorf("worker count must be positive")
	}
	inputDir, err := filepath.Abs(args.inputDir)
	if err != nil {
		return fmt.Errorf("input path: %w", err)
	}
	outputDir, err := filepath.Abs(args.outputDir)
	if err != nil {
		return fmt.Errorf("output path: %w", err)
	}
	canonicalInput, err := canonicalPath(inputDir)
	if err != nil {
		return fmt.Errorf("resolve input path: %w", err)
	}
	canonicalOutput, err := canonicalPath(outputDir)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	inputInfo, err := os.Stat(inputDir)
	if err != nil {
		return fmt.Errorf("stat input directory: %w", err)
	}
	if !inputInfo.IsDir() {
		return fmt.Errorf("input path is not a directory: %s", inputDir)
	}
	if pathWithin(canonicalInput, canonicalOutput) || pathWithin(canonicalOutput, canonicalInput) {
		return fmt.Errorf("input and output paths overlap: %s and %s", inputDir, outputDir)
	}
	if info, err := os.Lstat(outputDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("output path is not a directory: %s", outputDir)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat output directory: %w", err)
	}
	if args.templateDir != "" {
		templateDir, err := filepath.Abs(args.templateDir)
		if err != nil {
			return fmt.Errorf("template path: %w", err)
		}
		canonicalTemplate, err := canonicalPath(templateDir)
		if err != nil {
			return fmt.Errorf("resolve template path: %w", err)
		}
		if pathWithin(canonicalOutput, canonicalTemplate) || pathWithin(canonicalTemplate, canonicalOutput) {
			return fmt.Errorf("template and output paths overlap: %s and %s", templateDir, outputDir)
		}
	}
	return nil
}

func copyOutputTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if d.Type()&os.ModeSymlink != 0 || !d.Type().IsRegular() {
			return fmt.Errorf("cannot copy non-regular output entry: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

func installOutput(stage, output string) error {
	parent := filepath.Dir(output)
	base := filepath.Base(output)
	backup, err := os.MkdirTemp(parent, "."+base+".old-")
	if err != nil {
		return fmt.Errorf("create output backup: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare output backup: %w", err)
	}
	oldExists := false
	if _, err := os.Lstat(output); err == nil {
		oldExists = true
		if err := os.Rename(output, backup); err != nil {
			return fmt.Errorf("move previous output: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check previous output: %w", err)
	}
	if err := os.Rename(stage, output); err != nil {
		if oldExists {
			_ = os.Rename(backup, output)
		}
		return fmt.Errorf("install output: %w", err)
	}
	if oldExists {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous output: %w", err)
		}
	}
	return nil
}

func runSSG(args ssgArgs) error {
	var err error
	args.inputDir, err = filepath.Abs(args.inputDir)
	if err != nil {
		return fmt.Errorf("input path: %w", err)
	}
	args.outputDir, err = filepath.Abs(args.outputDir)
	if err != nil {
		return fmt.Errorf("output path: %w", err)
	}
	if args.configFile != "" {
		args.configFile, err = filepath.Abs(args.configFile)
		if err != nil {
			return fmt.Errorf("config path: %w", err)
		}
	}
	if args.templateDir != "" {
		args.templateDir, err = filepath.Abs(args.templateDir)
		if err != nil {
			return fmt.Errorf("template path: %w", err)
		}
	}
	if err := validateSSGArgs(args); err != nil {
		return err
	}
	log.Printf("Input:  %s", args.inputDir)
	log.Printf("Output: %s", args.outputDir)

	eng, err := templates.New(args.templateDir)
	if err != nil {
		return fmt.Errorf("init templates: %w", err)
	}
	site, err := loadSiteConfigChecked(args.inputDir, args.configFile)
	if err != nil {
		return err
	}
	siteGo := toTemplateSite(site)
	if siteGo.FeedURL == "" {
		siteGo.FeedURL = templates.JoinURLPath(site.URL, templates.RSSFeedPath)
	}

	allInclude := append(append([]string{}, site.IncludeGlob...), site.AssetGlob...)
	fileFilter, err := globstar.NewFilter(allInclude, site.ExcludeGlob)
	if err != nil {
		return fmt.Errorf("compile file filter: %w", err)
	}
	assetMatcher, err := globstar.NewMatcher(site.AssetGlob)
	if err != nil {
		return fmt.Errorf("compile asset matcher: %w", err)
	}
	allFiles, err := walkFiles(args.inputDir, args.outputDir, fileFilter)
	if err != nil {
		return fmt.Errorf("walk files: %w", err)
	}
	sort.Strings(allFiles)
	if err := validateOutputCollisions(allFiles); err != nil {
		return err
	}

	parent := filepath.Dir(args.outputDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(args.outputDir)+".tmp-")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if !args.clearOutput {
		if info, statErr := os.Stat(args.outputDir); statErr == nil && info.IsDir() {
			if err := copyOutputTree(args.outputDir, stage); err != nil {
				return fmt.Errorf("copy previous output: %w", err)
			}
		}
	}
	if err := templates.WriteAssets(stage, highlightScriptAsset, highlightStyleAsset, mermaidScriptAsset); err != nil {
		return fmt.Errorf("write template assets: %w", err)
	}
	pages, err := processAllFiles(allFiles, args.inputDir, stage, args.workers, assetMatcher, eng, siteGo)
	if err != nil {
		return fmt.Errorf("process files: %w", err)
	}
	log.Printf("Found %d pages", len(pages))
	loadCommentsForPages(MailboxConfig{CommentsDir: site.CommentsDir, CommentsEmail: site.CommentsEmail}, pages)
	if err := renderAllPages(eng, pages, siteGo, stage); err != nil {
		return err
	}
	dirIndexEntries, err := generateDirIndices(eng, pages, siteGo, stage, args.now)
	if err != nil {
		return err
	}
	feedEntries, err := generateFeeds(eng, pages, site, siteGo, stage, args.now)
	if err != nil {
		return err
	}
	if err := generateSitemap(eng, pages, site, stage, append(dirIndexEntries, feedEntries...), args.now); err != nil {
		return err
	}
	if err := installOutput(stage, args.outputDir); err != nil {
		return err
	}
	log.Printf("Done. Output in %s", args.outputDir)
	return nil
}

func ssgMain() {
	if err := runSSG(parseSSGArgs()); err != nil {
		log.Fatalf("ssg: %v", err)
	}
}

// loadCommentsForPages loads comments from .eml files and attaches them
// to the corresponding InputPage structs.
