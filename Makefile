# This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0

# == all ==
all: build ## Compile all packages
.PHONY: all

# == test ==
test: ## Run all tests
	go test ./...
.PHONY: test

# == vet ==
vet: ## Run go vet
	go vet ./...
.PHONY: vet

# == bench ==
bench: ## Run all benchmarks
	go test -bench=. ./...
.PHONY: bench

# == build ==
build: ## Build all packages
	go build .
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
