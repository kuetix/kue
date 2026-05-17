APP_NAME := kue
BUILD_DIR := runtime/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)'

.DEFAULT_GOAL := help

.PHONY: all cli clean test install uninstall

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the kue CLI tool
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/cli/

all: cli ## Build all components (currently just the CLI)

cli: ## Build the kue CLI tool
	@echo "Building CLI..."
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/cli

test: ## Run all tests
	go test ./...
	kue test --dir tests

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

install: ## Install kue binary (GOBIN > GOPATH/bin > HOME/go/bin > /usr/local/bin)
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
