# Site to LLMs.txt - Generate llms.txt from any docs site

[![Go](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/)

> ⚠️ I vibe-coded this project. Quality isn't guaranteed but it is usable.

A Go-based web crawler that scrapes websites and outputs documentation in [LLMs.txt format](https://llmstxt.org/) and markdown files.

---

## Features

* Generate `llms.txt` and `llms-full.txt` for specific websites
* Convert HTML pages to Markdown
* Filter out unwanted pages, files, and external domains
* Categorize main vs. secondary documentation
* Track crawl progress
* Save content to structured files

---

## Installation

```bash
git clone https://github.com/Sosokker/site-to-llmstxt.git
cd site-to-llmstxt
make build
```

---

## Quick Start

```bash
./bin/site-to-llmstxt --url https://docs.example.com
```

```bash
# Custom settings
./bin/site-to-llmstxt --url https://example.com --output ./my-docs --workers 3 --verbose
```

---

## CLI Options

* `--url, -u`: Start URL (required)
* `--output, -o`: Output directory (default: `./output`)
* `--workers, -w`: Number of concurrent workers (default: `1`)
* `--verbose`: Enable progress logging

---

## Output

```
output/
├── llms.txt         # Overview
├── llms-full.txt    # Full crawl
└── pages/           # Per-page Markdown
```

---

## Architecture

```
internal/
├── config/        # CLI and env config
├── crawler/       # Fetch and parse logic
├── filters/       # URL + content filter
├── generator/     # Output file creator
├── models/        # Shared types
├── progress/      # Progress reporting
└── utils/         # Misc functions
```

---

## Filtering Rules

**Excluded:**

* Language folders (`/en/`, `/zh/`, etc.)
* File types: `.pdf`, `.docx`, `.zip`, `.jpg`, `.mp4`, `.exe`, etc.
* Paths: `/blog`, `/news`, `/about`, `/contact`, etc.

**Categorization:**

* **Main:** Guides, API, setup
* **Secondary:** Blog, news, legal

---

## Development Setup

```bash
make dev-setup
```

### Common Commands

```bash
make build
make test
make lint
make demo
make clean
```

---

## Code Style

* `gofmt`, `goimports`
* Idiomatic Go patterns
* `%w` for error wrapping
* Table-driven tests
* Package names are short and focused

---

## Testing

```bash
make test
make test-coverage
go test -v ./internal/filters/
```

---

## Dependencies

* `colly` – web crawling
* `html-to-markdown` – content conversion
* `progressbar` – terminal progress
* `urfave/cli` – CLI handling

---

## License

MIT – see [LICENSE](LICENSE)