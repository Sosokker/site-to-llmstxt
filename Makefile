# Makefile for site-to-llmstxt crawler

.PHONY: build test clean run help

# Default target
help:
	@echo "Available targets:"
	@echo "  build   - Build the crawler binary"
	@echo "  test    - Run tests"
	@echo "  clean   - Clean build artifacts"
	@echo "  run     - Run with example URL (requires URL variable)"
	@echo "  install - Install dependencies"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run URL=https://example.com"
	@echo "  make run URL=https://httpbin.org WORKERS=2 OUTPUT=./test-output"

# Build the crawler
build:
	@echo "Building crawler..."
	go build -o crawler main.go
	@echo "Build complete: ./crawler"

# Run tests
test:
	@echo "Running tests..."
	go test -v

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f crawler
	rm -rf output/
	rm -rf test-output/
	rm -rf example-output/

# Install dependencies
install:
	@echo "Installing dependencies..."
	go mod tidy

# Run with parameters (updated for new CLI)
run: build
	@if [ -z "$(URL)" ]; then \
		echo "Error: URL is required. Usage: make run URL=https://example.com"; \
		exit 1; \
	fi
	@echo "Running crawler with URL: $(URL)"
	./crawler \
		--url $(URL) \
		$(if $(WORKERS),--workers $(WORKERS)) \
		$(if $(OUTPUT),--output $(OUTPUT)) \
		$(if $(VERBOSE),--verbose)

# Build and test everything
all: clean install build test
	@echo "All tasks completed successfully!"

# Quick test with a small site
demo: build
	@echo "Running demo crawl of httpbin.org..."
	./crawler --url https://httpbin.org --output ./demo-output --workers 1 --verbose
