// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/tim-janik/iris/templates"
)

type SiteConfig struct {
	Title         string   `toml:"title"          comment:"Site title (used in <title>, feeds, fallback page title)"`
	Slogan        string   `toml:"slogan"         comment:"Short tagline displayed in the header"`
	Description   string   `toml:"description"    comment:"Longer description for <meta> tags and feeds"`
	URL           string   `toml:"url"            comment:"Base URL for absolute links and sitemap"`
	Authors       []string `toml:"authors"        comment:"Default authors (used when page frontmatter has none)"`
	Copyright     string   `toml:"copyright"      comment:"Copyright text for footer"`
	FeedURL       string   `toml:"feed_url"       comment:"URL referenced in feed <link> tags"`
	FeedAge       int      `toml:"feed_age"       comment:"Max post age in days for feeds; -1 = no cutoff"`
	TeaserLen     int      `toml:"teaser_len"     comment:"Excerpt length for RSS/Atom feeds (characters)"`
	DescLen       int      `toml:"desc_len"       comment:"Excerpt length for directory index listings (characters)"`
	IconHref      string   `toml:"icon_href"      comment:"Favicon path"`
	LogoHref      string   `toml:"logo_href"      comment:"Logo image path"`
	CommentsDir   string   `toml:"comments_dir"   comment:"Directory containing .eml comment files"`
	CommentsEmail string   `toml:"comments_email" comment:"Email template for comment posting (%s = page LUID)"`
	IncludeGlob   []string `toml:"include_glob"   comment:"Glob patterns for files to process (e.g. \"20*/**/*.md\", \"**/*.adoc\")"`
	AssetGlob     []string `toml:"asset_glob"     comment:"Glob patterns for static assets (copied verbatim, no sitemap entry)"`
	ExcludeGlob   []string `toml:"exclude_glob"   comment:"Glob patterns for files to skip (default: skip _-prefixed files)"`
	Stylesheet    string   `toml:"stylesheet"     comment:"Custom stylesheet path (e.g. \"assets/site.css\"); empty = use converter defaults"`
	TitlePrefix   string   `toml:"title_prefix"   comment:"Prefix prepended to page titles in <title> (e.g. \"📚 \")"`
}

func defaultSiteConfig() SiteConfig {
	return SiteConfig{
		Title:       "Site",
		Slogan:      "",
		Description: "",
		URL:         "https://example.com",
		Authors:     []string{},
		Copyright:   "",
		FeedURL:     "",
		FeedAge:     -1, // -1 = unlimited (no age cutoff)
		TeaserLen:   300,
		DescLen:     240,
		IconHref:    "/favicon.ico",
		IncludeGlob: []string{},
		AssetGlob:   []string{},
		ExcludeGlob: []string{"_*"}, // default: skip config/template prefixes
		TitlePrefix: "",             // "📜 ",
	}
}

// loadSiteConfig loads site configuration from a TOML file.
// If configFile is empty, returns default configuration without reading any file.
// Fields not present in the file keep their default values.
func loadSiteConfig(inputDir, configFile string) SiteConfig {
	site := defaultSiteConfig()

	if configFile == "" {
		log.Printf("no config file specified, using defaults")
		return site
	}

	_, err := toml.DecodeFile(configFile, &site)
	if err != nil {
		log.Printf("failed to read config %s, using defaults: %v", configFile, err)
		return defaultSiteConfig()
	}
	// Excerpt lengths must be positive; negative values would truncate to
	// empty output, so fall back to the defaults instead.
	def := defaultSiteConfig()
	if site.TeaserLen < 0 {
		log.Printf("config %s: teaser_len %d is negative, using default %d", configFile, site.TeaserLen, def.TeaserLen)
		site.TeaserLen = def.TeaserLen
	}
	if site.DescLen < 0 {
		log.Printf("config %s: desc_len %d is negative, using default %d", configFile, site.DescLen, def.DescLen)
		site.DescLen = def.DescLen
	}
	log.Printf("loaded site config from %s", configFile)
	return site
}

// renderPage renders a single InputPage to HTML using the appropriate template.

const defaultConfigPath = "_siteconfig.toml"

// initConfig writes a default config file to the given path.
// If path is empty, defaults to "_siteconfig.toml" in the current directory.
// ---------------------------------------------------------------------------
// Init subcommand — scaffold a default config file
// ---------------------------------------------------------------------------

func initConfig(path string) {
	if path == "" {
		path = defaultConfigPath
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create config: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	cfg := defaultSiteConfig()
	if err := writeConfigTOML(f, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "encode config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created config file: %s\n", path)
}

// writeConfigTOML serializes a SiteConfig to TOML with per-field comments
// sourced from the struct's "comment" tag.
func writeConfigTOML(w io.Writer, cfg SiteConfig) error {
	t := reflect.TypeOf(cfg)
	v := reflect.ValueOf(cfg)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		comment := field.Tag.Get("comment")
		tomlKey := field.Tag.Get("toml")
		if tomlKey == "" {
			tomlKey = field.Name
		}

		// Write comment
		if comment != "" {
			fmt.Fprintf(w, "# %s\n", comment)
		}

		// Format value
		val := v.Field(i)
		switch val.Kind() {
		case reflect.String:
			fmt.Fprintf(w, "%s = %q\n\n", tomlKey, val.String())
		case reflect.Int:
			fmt.Fprintf(w, "%s = %d\n\n", tomlKey, val.Int())
		case reflect.Slice:
			elems := make([]string, val.Len())
			for j := 0; j < val.Len(); j++ {
				elems[j] = strconv.Quote(val.Index(j).String())
			}
			fmt.Fprintf(w, "%s = [%s]\n\n", tomlKey, strings.Join(elems, ", "))
		}
	}
	return nil
}

func toTemplateSite(site SiteConfig) templates.SiteConfig {
	return templates.SiteConfig{
		Title:       site.Title,
		Slogan:      site.Slogan,
		Description: site.Description,
		URL:         site.URL,

		Authors:     site.Authors,
		Copyright:   site.Copyright,
		FeedURL:     site.FeedURL,
		FeedAge:     site.FeedAge,
		TeaserLen:   site.TeaserLen,
		DescLen:     site.DescLen,
		IconHref:    site.IconHref,
		LogoHref:    site.LogoHref,
		Stylesheet:  site.Stylesheet,
		TitlePrefix: site.TitlePrefix,
	}
}
