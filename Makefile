.PHONY: help install test clean lint format check build build-release build-all run checksums generate-api

# Default target
help: ## Show this help message
	@echo "Port CLI - Makefile Commands"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "\033[36m\033[0m"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36mmake %-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST) | sort

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Ensure GOPATH/bin is in the PATH so tools installed via 'go install' are found
export PATH := $(PATH):$(shell go env GOPATH)/bin

# Build flags
LDFLAGS = -s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.buildDate=$(BUILD_DATE)' \
	-X 'main.commit=$(COMMIT)'
BUILD_FLAGS = -trimpath -ldflags "$(LDFLAGS)"

# Optimized build flags for smaller binaries
RELEASE_FLAGS = -trimpath -ldflags "$(LDFLAGS)" -buildmode=pie -tags=release

# Build targets
build: ## Build the Go binary (dev version)
	@echo "Building Port CLI..."
	@mkdir -p bin
	@go build $(BUILD_FLAGS) -o bin/port ./cmd/port
	@echo "Build complete: bin/port"
	@echo "Version: $(VERSION)"

build-release: ## Build optimized release binary
	@echo "Building optimized Port CLI release..."
	@mkdir -p bin
	@CGO_ENABLED=0 go build $(RELEASE_FLAGS) -o bin/port ./cmd/port
	@echo "Release build complete: bin/port"
	@ls -lh bin/port
	@echo "Binary size optimization applied (stripped symbols, compressed DWARF)"

build-all: ## Build for all platforms
	@echo "Building Port CLI for all platforms..."
	@echo "Version: $(VERSION)"
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(RELEASE_FLAGS) -o dist/port-linux-amd64 ./cmd/port
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(RELEASE_FLAGS) -o dist/port-linux-arm64 ./cmd/port
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(RELEASE_FLAGS) -o dist/port-darwin-amd64 ./cmd/port
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(RELEASE_FLAGS) -o dist/port-darwin-arm64 ./cmd/port
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(RELEASE_FLAGS) -o dist/port-windows-amd64.exe ./cmd/port
	@echo "Build complete: dist/"
	@echo "Binaries:"
	@ls -lh dist/

# Test targets
test: ## Run all tests
	@echo "Running tests..."
	@go test -v ./...

test-cov: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Code quality targets
lint: ## Run linter (golangci-lint)
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

format: ## Format code (gofmt)
	@echo "Formatting code..."
	@go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	else \
		echo "goimports not installed. Install with: go install golang.org/x/tools/cmd/goimports@latest"; \
	fi

check: lint test ## Run all quality checks
	@echo "All quality checks passed!"

# Clean build artifacts
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf bin dist
	@rm -rf coverage.out coverage.html
	@go clean ./...

# Run target
run: build ## Run the CLI
	@echo "Running Port CLI..."
	@./bin/port --help

# Install target
install: build ## Install the CLI binary
	@echo "Installing Port CLI..."
	@mkdir -p /usr/local/bin 2>/dev/null || mkdir -p ~/.local/bin
	@cp bin/port /usr/local/bin/port 2>/dev/null || cp bin/port ~/.local/bin/port
	@echo "Installed: port"

# Development targets
dev-install: ## Install CLI in development mode
	@echo "Installing CLI in development mode..."
	@go install ./cmd/port

# All quality checks
quality: format lint test ## Run all quality checks
	@echo "All quality checks passed!"

# Complete workflow
all: quality build-release ## Complete workflow
	@echo "All steps completed successfully!"

# Generate checksums
checksums: ## Generate checksums for binaries
	@echo "Generating checksums..."
	@cd dist && sha256sum port-* > checksums.txt 2>/dev/null || shasum -a 256 port-* > checksums.txt || echo "Checksum generation failed"
	@echo "Checksums: dist/checksums.txt"

# Generate OpenAPI client code
generate-api: ## Generate OpenAPI client code from spec
	@echo "Generating OpenAPI client code..."
	@chmod +x scripts/generate-api.sh
	@./scripts/generate-api.sh
