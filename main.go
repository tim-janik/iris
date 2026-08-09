// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		initConfig(parseInitArgs())
	case "index":
		indexMain()
	case "ssg":
		ssgMain()
	case "serve":
		serveMain()
	case "version":
		printVersion()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// printUsage prints the usage summary to stderr.
func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: iris <subcommand> [flags]

Subcommands:
  init      Create _siteconfig.toml with default settings
  index     Generate index.md lines from a list of .md files
  ssg       Build the site (convert, render, generate feeds/sitemap)
  serve     Start an HTTP server that renders .md files to HTML on-the-fly
  version   Print version and build information

Run "iris <subcommand> -h" for more information on a subcommand.

Examples:
  iris index page1.md page2.md
	Print markdown link lines suitable for index.md
  iris serve ./docs --port 9454
	Serves all .md files under ./docs as rendered HTML on localhost:9454
`)
}

// printVersion prints version and VCS build information.
func printVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("iris (no build info)")
		return
	}
	fmt.Printf("iris %s\n", info.Main.Version)
	if info.Main.Sum != "" {
		fmt.Printf("  module: %s %s\n", info.Main.Path, info.Main.Sum)
	}
	for _, dep := range info.Deps {
		fmt.Printf("  %s %s\n", dep.Path, dep.Version)
	}
	if settings := info.Settings; len(settings) > 0 {
		fmt.Println()
		for _, s := range settings {
			fmt.Printf("  %s=%s\n", s.Key, s.Value)
		}
	}
}

// ssgMain is the main entry point for the ssg subcommand.
