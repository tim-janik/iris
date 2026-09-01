// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/tim-janik/iris/adoc"
	"github.com/tim-janik/iris/frontmatter"
	"github.com/tim-janik/iris/globstar"
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

	inputDir, _ := filepath.Abs(args[0])
	outputDir, _ := filepath.Abs(args[1])

	w := *workers
	if w <= 0 {
		w = runtime.NumCPU()
	}

	return ssgArgs{clearOutput: *clearOutput, inputDir: inputDir, outputDir: outputDir, configFile: *configFile, templateDir: *templateDir, workers: w, now: parseNow(*nowFlag)}
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
	editLinkCmd string // command template for edit links (empty = disabled)
	templateDir string // custom template directory (overrides embedded templates)
	faviconPath string // path to favicon file served at /favicon.ico
}

// parseServeArgs parses command-line arguments for the serve subcommand.
func parseServeArgs() serveArgs {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 9454, "TCP port to listen on (default: 9454)")
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
	return serveArgs{root: root, port: *port, editLinkCmd: *editLinkCmd, templateDir: *templateDir, faviconPath: *faviconPath}
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
		Root:         args.root,
		Port:         args.port,
		PandocConfig: pandoc.DefaultConfig(),
		AdocConfig:   adoc.DefaultConfig(),
		EditLinkCmd:  args.editLinkCmd,
		TemplateDir:  args.templateDir,
		FaviconPath:  args.faviconPath,
		Site:         toTemplateSite(site),
	}

	if err := srv.Serve(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func prepareOutputDir(dir string, clear bool) {
	if clear {
		log.Printf("Cleaning output directory: %s", dir)
		if err := os.RemoveAll(dir); err != nil {
			log.Fatalf("remove output dir: %v", err)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
}

// initEngine creates the template engine.
// If templateDir is non-empty, templates are loaded from that directory.
// Otherwise, embedded templates are used.
func initEngine(templateDir string) *templates.Engine {
	eng, err := templates.New(templateDir)
	if err != nil {
		log.Fatalf("init templates: %v", err)
	}
	return eng
}

// toTemplateSite converts a SiteConfig to templates.SiteConfig.

func ssgMain() {
	args := parseSSGArgs()
	log.Printf("Input:  %s", args.inputDir)
	log.Printf("Output: %s", args.outputDir)

	prepareOutputDir(args.outputDir, args.clearOutput)

	eng := initEngine(args.templateDir)
	site := loadSiteConfig(args.inputDir, args.configFile)
	siteGo := toTemplateSite(site)
	// Default the template feed link (page <link rel="alternate">) to the RSS
	// feed path when feed_url is unset; generateFeeds keeps site.FeedURL raw.
	if siteGo.FeedURL == "" {
		siteGo.FeedURL = site.URL + "/rss2.xml"
	}

	// Candidate files = union(include_glob, asset_glob); files matching neither are skipped
	allInclude := append(append([]string{}, site.IncludeGlob...), site.AssetGlob...)
	fileFilter, err := globstar.NewFilter(allInclude, site.ExcludeGlob)
	if err != nil {
		log.Fatalf("compile file filter: %v", err)
	}

	// Compile asset matcher (copy-only, no sitemap entry)
	assetMatcher, err := globstar.NewMatcher(site.AssetGlob)
	if err != nil {
		log.Fatalf("compile asset matcher: %v", err)
	}

	// Walk + filter
	allFiles, err := walkFiles(args.inputDir, args.outputDir, fileFilter)
	if err != nil {
		log.Fatalf("walk files: %v", err)
	}

	// Sort so classification order is stable regardless of FS walk order
	sort.Strings(allFiles)

	// Unified parallel queue: process all files (convert+render for .md/.adoc,
	// copy for static files, git dates for pageclass.PageCopy)
	pages, err := processAllFiles(allFiles, args.inputDir, args.outputDir, args.workers, assetMatcher, eng, siteGo)
	if err != nil {
		log.Fatalf("process files: %v", err)
	}
	log.Printf("Found %d pages", len(pages))

	// Load comments from .eml files
	loadCommentsForPages(MailboxConfig{
		CommentsDir:   site.CommentsDir,
		CommentsEmail: site.CommentsEmail,
	}, pages)

	// Render pages (only types that need template rendering)
	renderAllPages(eng, pages, siteGo, args.outputDir)

	// Generate directory indices (returns sitemap entries for each dirindex)
	dirIndexEntries := generateDirIndices(eng, pages, siteGo, args.outputDir, args.now)

	// Generate RSS and Atom feeds (returns sitemap entries for feed files)
	feedEntries := generateFeeds(eng, pages, site, siteGo, args.outputDir, args.now)

	// Generate sitemap (after dirindices and feeds so all entries are known)
	allExtra := append(dirIndexEntries, feedEntries...)
	generateSitemap(eng, pages, site, args.outputDir, allExtra, args.now)

	log.Printf("Done. Output in %s", args.outputDir)
}

// loadCommentsForPages loads comments from .eml files and attaches them
// to the corresponding InputPage structs.
