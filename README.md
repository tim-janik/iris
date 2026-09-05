# Iris

Fast and lean tool that turns Markdown and AsciiDoc files into a website.

Renders `.md` or `.adoc` files, interprets frontmatter like title and date, and Iris builds HTML, a sitemap, and feeds.
Iris can also serve static files (images, HTML) and render `.md` or `.adoc` on the fly, to show your files in a browser while you write.
 

## What it does

* builds a site from a folder of source files (`ssg`)
* shows files live in a browser while you edit (`serve`)
* makes link lines for an `index.md` (`index`)

Iris keeps things small.
It is for personal sites, blogs, and docs.
It depends on `pandoc` for Markdown and `asciidoctor` for AsciiDoc to render those file types.


## Features

* Converts Markdown and AsciiDoc to HTML with pandoc and asciidoctor
* Reads frontmatter (title, date, keywords, authors) from source files
* Takes publication and update dates from git history
* Builds RSS, Atom, and sitemap files
* Converts and renders in parallel, using all CPU cores
* Picks which files to process with include and exclude patterns
* Keeps URLs clean, without `.html` suffixes
* Shows email comments (`.eml` files) on post pages
* While serving, lists frontmatter files as a sortable, filterable board in the browser, and creates new ones from a form


## Install

For building, Go 1.25, curl, and sha256sum are needed. Build from a checkout:

```sh
make build
./iris -h
```


## Use

Run the tool with `-h` to see all flags and examples:

```sh
iris -h
iris ssg -h
iris serve -h
iris index -h
iris init -h
```

Typical steps:

```sh
iris init
iris ssg ./content ./public
iris serve /home/user/
```
