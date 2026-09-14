APP_NAME := kue
BUILD_DIR := runtime/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)'

.DEFAULT_GOAL := help

GO ?= go

.PHONY: all cli clean test test-race test-integration test-e2e test-all test-trace cover fmt-check vet ci install uninstall tag generate-modules check-modules test-install test-release build-snapshot

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the kue CLI tool
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/cli/

all: cli ## Build all components (currently just the CLI)

cli: ## Build the kue CLI tool
	@echo "Building CLI..."
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/cli

# RUN, if set, is passed to `go test -run` (e.g. `make test-e2e RUN=TestRunHello`).
RUN ?=
RUNFLAG := $(if $(RUN),-run '$(RUN)',)

test: ## Run unit + integration tests (quiet)
	$(GO) test $(RUNFLAG) ./...

test-race: ## Run unit + integration tests with the race detector
	$(GO) test -race -count=1 $(RUNFLAG) ./...

test-integration: ## Run integration tests, verbose — narrates every workflow, status & timing
	$(GO) test -count=1 -v $(RUNFLAG) ./tests/integration/...

test-e2e: ## Run e2e tests, verbose — narrates every `kue` invocation, exit code & output
	$(GO) test -tags e2e -count=1 -v $(RUNFLAG) ./tests/e2e/...

test-install: ## Test scripts/install.sh end-to-end against a local fake release server
	bash tests/install/install_test.sh

build-snapshot: ## Cross-compile every release target locally without publishing (needs goreleaser)
	goreleaser build --snapshot --clean

test-release: check-modules build-snapshot test-install ## Test the whole release pipeline: module list, cross-platform build, installer

test-all: test test-e2e test-release ## Run every test layer, including the release pipeline

test-trace: ## Run integration + e2e verbose, one test binary at a time (best for debugging)
	$(GO) test -count=1 -v $(RUNFLAG) ./tests/integration/...
	$(GO) test -tags e2e -count=1 -v $(RUNFLAG) ./tests/e2e/...

cover: ## Run tests with coverage and print a summary
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

fmt-check: ## Fail if any tracked .go file needs gofmt
	@out=$$(git ls-files '*.go' | grep -v '^vendor/' | xargs gofmt -l); \
	if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

vet: ## Run go vet
	$(GO) vet ./...

generate-modules: ## Regenerate modules/modules.go + modules/MODULES.md from modules/modules.yaml
	$(GO) run ./cmd/gen-modules

check-modules: ## Fail if modules/modules.go or MODULES.md are out of sync with modules.yaml
	$(GO) run ./cmd/gen-modules
	@git diff --exit-code -- modules/modules.go modules/MODULES.md || \
		(echo "modules/modules.go or MODULES.md is out of sync with modules.yaml — run 'make generate-modules' and commit the result" && exit 1)

ci: fmt-check vet check-modules build ## Run the full CI gate locally (quiet; -v output only on failure)
	$(GO) test -race -count=1 ./...
	$(GO) test -tags e2e -count=1 ./tests/e2e/...

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

install: build ## Install kue binary (GOBIN > GOPATH/bin > HOME/go/bin > /usr/local/bin)
	INSTALL_DIR=$(INSTALL_DIR) ./install.sh kue

uninstall: ## Uninstall kue binary from the resolved install directory
	@echo "Uninstalling kue from $(INSTALL_DIR)..."
	@if [ -f "$(INSTALL_DIR)/kue" ]; then \
		echo "Removing kue..."; \
		if rm "$(INSTALL_DIR)/kue" 2>/dev/null; then \
			echo "✓ Removed kue"; \
		else \
			echo "Note: Elevated privileges required for $(INSTALL_DIR)"; \
			if sudo rm -f "$(INSTALL_DIR)/kue"; then \
				echo "✓ Removed kue"; \
			else \
				echo "✗ Failed to remove kue"; \
			fi \
		fi \
	fi
	@echo "Uninstall complete!"

tag: ## Create an annotated release tag (usage: make tag TAG=v0.x.y)
	@if [ -z "$(TAG)" ]; then echo "Usage: make tag TAG=v0.x.y"; exit 1; fi
	@case "$(TAG)" in v[0-9]*) ;; *) echo "TAG must look like vX.Y.Z"; exit 1;; esac
	@if [ -n "$$(git status --porcelain)" ]; then echo "working tree not clean"; exit 1; fi
	git tag -a $(TAG) -m "Release $(TAG)"
	@echo "Created tag $(TAG). Push it with:  git push origin $(TAG)"
	@echo "The release workflow builds and publishes the GitHub Release."
