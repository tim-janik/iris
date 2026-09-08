# Golden master tests for iris site builds and serve.
#
#   make -C golden            diff ssg and serve output against expected-*/
#   make -C golden update     regenerate expected-*/ from the current build
#
# input/ holds a small site exercising the main features: markdown with
# code, mermaid, emoji, tables and footnotes; asciidoctor with sections,
# source blocks and admonitions; issue frontmatter and a dashboard page;
# nested pages and static assets. All pages carry explicit published
# dates so output does not depend on git history or file mtimes.
#
# The downloaded minified assets (highlight.js, mermaid) are removed
# from the build output before diffing; they are pinned checksums, not
# build results.

IRIS ?= ../iris
NOW  ?= 2026-09-08
DIFF  = git -P diff --no-index --color=always

.PHONY: golden golden-ssg golden-serve ssg-build serve-build update help

golden: golden-ssg golden-serve ## Diff ssg and serve output against expected-*/

golden-ssg: ssg-build ## Build the site and diff against expected-ssg
	@$(DIFF) expected-ssg ssg || \
		{ echo '  rsync' -n -aHAXvP $(CURDIR)/ssg/ --del $(CURDIR)/expected-ssg/; exit 1; }

golden-serve: serve-build ## Record serve responses and diff against expected-serve
	@$(DIFF) expected-serve serve || \
		{ echo '  rsync' -n -aHAXvP $(CURDIR)/serve/ --del $(CURDIR)/expected-serve/; exit 1; }

ssg-build: $(IRIS)
	@$(IRIS) ssg -c input/_siteconfig.toml -now $(NOW) input ssg
	@rm -rf ssg/assets/highlight.js ssg/assets/mermaid

serve-build: $(IRIS)
	@$(IRIS) serve -record serve input 2>/dev/null

update: ssg-build serve-build ## Replace expected-* with the current output
	rm -rf expected-ssg && cp -a ssg/ expected-ssg/
	rm -rf expected-serve && cp -a serve/ expected-serve/

$(IRIS):
	$(MAKE) -C .. build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'
.PHONY: help
