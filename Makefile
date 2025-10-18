# Makefile for site-to-llmstxt crawler

.PHONY: build test clean run tui demo help fmt lint deps dev-setup all

# Variables
BINARY_NAME=site-to-llmstxt
CMD_PATH=./cmd/site-to-llmstxt
BUILD_DIR=./bin

# Default target
help:
	@echo "Available targets:"
	@echo "  build         - Build the crawler binary"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  clean         - Clean build artifacts"
	@echo "  run           - Run with example URL (requires URL variable)"
	@echo "  tui           - Launch interactive terminal UI"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  deps          - Install/update dependencies"
	@echo "  dev-setup     - Set up development environment"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run URL=https://example.com"
	@echo "  make run URL=https://httpbin.org WORKERS=2 OUTPUT=./test-output"
	@echo "  make tui OUTPUT=./docs WORKERS=4"

# Build the crawler
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -rf output/
	rm -rf test-output/
	rm -rf demo-output/
	rm -f coverage.out coverage.html

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@which goimports > /dev/null && goimports -w . || echo "goimports not found, skipping import formatting"

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	@which golangci-lint > /dev/null && golangci-lint run || echo "golangci-lint not found, skipping linting"

# Install/update dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Development setup
dev-setup: deps
	@echo "Setting up development environment..."
	@echo "Installing development tools..."
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "Development setup complete!"

# Run with parameters (updated for new CLI)
run: build
	@if [ -z "$(URL)" ]; then \
		echo "Error: URL is required. Usage: make run URL=https://example.com"; \
		exit 1; \
	fi
	@echo "Running crawler with URL: $(URL)"
	$(BUILD_DIR)/$(BINARY_NAME) \
		--url $(URL) \
		$(if $(WORKERS),--workers $(WORKERS)) \
		$(if $(OUTPUT),--output $(OUTPUT)) \
		$(if $(VERBOSE),--verbose)

# Build and test everything
all: clean deps fmt build test
	@echo "All tasks completed successfully!"

# Quick test with a small site
demo: build
	@echo "Running demo crawl of httpbin.org..."
	$(BUILD_DIR)/$(BINARY_NAME) --url https://httpbin.org --output ./demo-output --workers 1 --verbose
# Launch interactive TUI
tui:
	@echo "Launching TUI..."
	go run ./cmd/site-to-llmstxt tui $(if $(OUTPUT),--output $(OUTPUT)) $(if $(WORKERS),--workers $(WORKERS))
