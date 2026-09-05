# This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0

SHELL := /bin/bash

include external/Makefile.mk

# == all ==
all: build ## Compile all packages
.PHONY: all

# == test ==
test: $(EXTERNAL_STAMPS) ## Run all tests
	go test ./...
.PHONY: test

# == vet ==
vet: $(EXTERNAL_STAMPS) ## Run go vet
	go vet ./...
.PHONY: vet

# == bench ==
bench: $(EXTERNAL_STAMPS) ## Run all benchmarks
	go test -bench=. ./...
.PHONY: bench

# == build ==
build: $(EXTERNAL_STAMPS) ## Build a portable binary
	CGO_ENABLED=0 go build -trimpath # -buildvcs=true -ldflags="-s -w" .
.PHONY: build

# == clean ==
clean: ## Remove cached build artifacts
	go clean ./...
.PHONY: clean

# == meta ==
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'
.PHONY: help
