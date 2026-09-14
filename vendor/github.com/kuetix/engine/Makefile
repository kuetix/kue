MCP_PATH := ./cmd/mcp-server
MCP_SERVER := mcp-server
BUILD_DIR := runtime/bin
INSTALL_DIR := $(or $(GOBIN),$(if $(GOPATH),$(GOPATH)/bin,$(if $(HOME),$(HOME)/go/bin,/usr/local/bin)))
VERSION := $(shell git describe --tags --always --dirty)
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)'

.DEFAULT_GOAL := help

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

test: ## Run all tests
	go test ./...

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

fmt: ## Format Go code
	go fmt ./...

deps: ## Tidy Go module dependencies
	go mod tidy

tag: ## Create a release tag and update embedded build time (usage: make tag TAG=v0.x.y)
	@if [ -z "$(TAG)" ]; then \
		echo "Usage: make tag TAG=v0.x.y"; \
		exit 1; \
	fi
	@TAG_TIME=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	printf 'package main\n\n// defaultBuildTime is the build time embedded at release time.\n// This variable is updated by `make tag` when creating a new release,\n// ensuring `kue version` shows the correct build time even when installed\n// via `go install` (where VCS metadata is not available from the module proxy).\nvar defaultBuildTime = "%s"\n' "$$TAG_TIME" > cmd/kue/build_time.go; \
	git add cmd/kue/build_time.go; \
	git commit -m "chore: update build time for $(TAG)"; \
	git tag -a $(TAG) -m "Release $(TAG)"; \
	echo "Created tag $(TAG) with build time $$TAG_TIME"

build: ## Build the MCP server binary
	go build -ldflags "$(LDFLAGS)" -o $(MCP_SERVER) $(MCP_PATH)

build_linux_arm: ## Build the MCP server binary for Linux ARM64
	 GOARCH="arm64" GOOS="linux" go build -ldflags "$(LDFLAGS)" -o "$(MCP_SERVER)_arm64" $(MCP_PATH)

run: ## Build and run the MCP server
	$(MAKE) build
	./$(MCP_SERVER)

.PHONY: help all kue test clean fmt deps install uninstall tag
