# This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0

define fetch-and-check
( printf '%s  %s\n' '$(strip $2)' '$(strip $1)' | sha256sum -c --status 2>/dev/null || \
    curl --retry 2 -# -fSL '$(strip $3)' -o '$(strip $1)' ) && \
printf '%s  %s\n' '$(strip $2)' '$(strip $1)' | sha256sum -c || \
  { sha256sum "$(strip $1)"; echo '$(strip $1): ERROR: failed to fetch: $(strip $3)' >&2; exit 1; }
endef

highlightjs/version := 10.7.3
highlightjs/js_sha := 2e027de64e1a747b39ef0d16c07e55751c8e31a4d3178d1e7e487b35f1d47404
highlightjs/css_sha := 554e678b27d0ddbcca9b262965c55fabbea13e902673d402a0b86384ddcbd064
highlightjs/js_url := https://cdnjs.cloudflare.com/ajax/libs/highlight.js/$(highlightjs/version)/highlight.min.js
highlightjs/css_url := https://cdnjs.cloudflare.com/ajax/libs/highlight.js/$(highlightjs/version)/styles/github.min.css

EXTERNAL_STAMPS := external/highlight.js/.sha-$(highlightjs/js_sha)-$(highlightjs/css_sha)

.PHONY: all
all: $(EXTERNAL_STAMPS)

external/highlight.js/.sha-$(highlightjs/js_sha)-$(highlightjs/css_sha):
	mkdir -p external/highlight.js/styles
	$(call fetch-and-check, external/highlight.js/highlight.min.js, $(highlightjs/js_sha), $(highlightjs/js_url))
	$(call fetch-and-check, external/highlight.js/styles/github.min.css, $(highlightjs/css_sha), $(highlightjs/css_url))
	touch $@

$(EXTERNAL_STAMPS): external/Makefile.mk
