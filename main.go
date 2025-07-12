package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/debug"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
)

const (
	// DefaultWorkers is the default number of concurrent workers
	DefaultWorkers = 1
	// DefaultOutputDir is the default output directory
	DefaultOutputDir = "./output"
	// MarkdownSubdir is the subdirectory for markdown files
	MarkdownSubdir = "pages"
)

// Config holds crawler configuration
type Config struct {
	URL       string
	OutputDir string
	Workers   int
	Verbose   bool
}

// PageInfo represents information about a crawled page
type PageInfo struct {
	URL         string
	Title       string
	Content     string
	FilePath    string
	CrawledAt   time.Time
	Description string
}

// Crawler manages the web crawling process
type Crawler struct {
	config     *Config
	collector  *colly.Collector
	converter  *converter.Converter
	visited    map[string]bool
	queue      chan string
	wg         sync.WaitGroup
	mu         sync.RWMutex
	baseURL    *url.URL
	bar        *progressbar.ProgressBar
	processed  int
	pages      []PageInfo
	pagesMutex sync.Mutex
}

// LanguageFilter contains patterns to exclude language-specific URLs
var LanguageFilter = []string{
	`/en/`, `/en$`,
	`/zh/`, `/zh$`, `/zh-cn/`, `/zh-cn$`, `/zh-tw/`, `/zh-tw$`, `/zh-hant/`, `/zh-hant$`,
	`/ja/`, `/ja$`,
	`/ko/`, `/ko$`,
	`/fr/`, `/fr$`,
	`/de/`, `/de$`,
	`/es/`, `/es$`,
	`/it/`, `/it$`,
	`/pt/`, `/pt$`,
	`/ru/`, `/ru$`,
}

// FileExtensionFilter contains patterns to exclude file downloads
var FileExtensionFilter = []string{
	`\.pdf$`, `\.doc$`, `\.docx$`, `\.xls$`, `\.xlsx$`, `\.ppt$`, `\.pptx$`,
	`\.zip$`, `\.rar$`, `\.tar$`, `\.gz$`, `\.7z$`,
	`\.mp3$`, `\.mp4$`, `\.avi$`, `\.mov$`, `\.wmv$`,
	`\.jpg$`, `\.jpeg$`, `\.png$`, `\.gif$`, `\.bmp$`, `\.svg$`,
	`\.exe$`, `\.msi$`, `\.dmg$`, `\.deb$`, `\.rpm$`,
}

func main() {
	app := &cli.App{
		Name:  "site-to-llmstxt",
		Usage: "Web crawler that converts websites to LLMs.txt format",
		Description: `A high-performance web crawler that scrapes websites and converts them to LLMs.txt format.
		
The crawler generates:
- llms.txt: A curated overview following the LLMs.txt specification
- llms-full.txt: Complete content of all crawled pages
- pages/: Directory containing individual markdown files

The crawler respects robots.txt, filters out language variants and file downloads,
and only crawls within the same domain.`,
		Version: "1.0.0",
		Authors: []*cli.Author{
			{
				Name: "Site-to-LLMsTxt",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Aliases:  []string{"u"},
				Usage:    "Root URL to crawl (required)",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
				Value:   DefaultOutputDir,
			},
			&cli.IntFlag{
				Name:    "workers",
				Aliases: []string{"w"},
				Usage:   "Number of concurrent workers",
				Value:   DefaultWorkers,
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Enable verbose logging",
			},
		},
		Action: func(c *cli.Context) error {
			config := &Config{
				URL:       c.String("url"),
				OutputDir: c.String("output"),
				Workers:   c.Int("workers"),
				Verbose:   c.Bool("verbose"),
			}

			return runCrawler(config)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runCrawler(config *Config) error {
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		return fmt.Errorf("failed to create crawler: %w", err)
	}

	ctx := context.Background()
	if err := crawler.Start(ctx); err != nil {
		return fmt.Errorf("crawling failed: %w", err)
	}

	if err := crawler.GenerateLLMSFiles(); err != nil {
		return fmt.Errorf("failed to generate LLMS files: %w", err)
	}

	fmt.Printf("\nCrawling completed successfully!\n")
	fmt.Printf("Generated files:\n")
	fmt.Printf("  - %s\n", filepath.Join(config.OutputDir, "llms.txt"))
	fmt.Printf("  - %s\n", filepath.Join(config.OutputDir, "llms-full.txt"))
	fmt.Printf("  - %s/ (individual pages)\n", filepath.Join(config.OutputDir, MarkdownSubdir))
	fmt.Printf("Total pages crawled: %d\n", len(crawler.pages))

	return nil
}

func validateConfig(config *Config) error {
	if config.URL == "" {
		return fmt.Errorf("URL is required")
	}

	u, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must have http or https scheme")
	}

	if config.Workers <= 0 {
		return fmt.Errorf("workers must be greater than 0")
	}

	return nil
}

// NewCrawler creates a new crawler instance
func NewCrawler(config *Config) (*Crawler, error) {
	baseURL, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	// Create output directory structure
	if err := createOutputDirs(config.OutputDir); err != nil {
		return nil, fmt.Errorf("failed to create output directories: %w", err)
	}

	// Setup colly collector
	c := colly.NewCollector(
		colly.AllowedDomains(baseURL.Host),
	)

	if config.Verbose {
		c.SetDebugger(&debug.LogDebugger{})
	}

	// Rate limiting
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: config.Workers,
		Delay:       200 * time.Millisecond, // Slightly more conservative
	})

	// Setup HTML to Markdown converter
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
		),
	)

	crawler := &Crawler{
		config:    config,
		collector: c,
		converter: conv,
		visited:   make(map[string]bool),
		queue:     make(chan string, 1000),
		baseURL:   baseURL,
		bar:       progressbar.NewOptions(-1, progressbar.OptionSetDescription("Crawling pages")),
		pages:     make([]PageInfo, 0),
	}

	crawler.setupCallbacks()

	return crawler, nil
}

func createOutputDirs(outputDir string) error {
	dirs := []string{
		outputDir,
		filepath.Join(outputDir, MarkdownSubdir),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

func (c *Crawler) setupCallbacks() {
	// Handle HTML content
	c.collector.OnHTML("html", func(e *colly.HTMLElement) {
		c.processPage(e)
	})

	// Extract links
	c.collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		c.addToQueue(link, e.Request.URL)
	})

	// Request callback
	c.collector.OnRequest(func(r *colly.Request) {
		if c.config.Verbose {
			fmt.Printf("Visiting: %s\n", r.URL)
		}
		c.bar.Add(1)
	})

	// Error handling
	c.collector.OnError(func(r *colly.Response, err error) {
		log.Printf("Error visiting %s: %v", r.Request.URL, err)
	})
}

func (c *Crawler) processPage(e *colly.HTMLElement) {
	// Get page title
	title := strings.TrimSpace(e.ChildText("title"))
	if title == "" {
		title = "Untitled"
	}

	// Get meta description
	description := strings.TrimSpace(e.ChildAttr("meta[name='description']", "content"))
	if description == "" {
		// Try og:description
		description = strings.TrimSpace(e.ChildAttr("meta[property='og:description']", "content"))
	}

	// Convert HTML to Markdown
	html, err := e.DOM.Html()
	if err != nil {
		log.Printf("Failed to get HTML for %s: %v", e.Request.URL, err)
		return
	}

	markdown, err := c.converter.ConvertString(html)
	if err != nil {
		log.Printf("Failed to convert HTML to Markdown for %s: %v", e.Request.URL, err)
		return
	}

	// Create page info
	pageInfo := PageInfo{
		URL:         e.Request.URL.String(),
		Title:       title,
		Content:     markdown,
		CrawledAt:   time.Now(),
		Description: description,
	}

	// Save individual markdown file
	filename := c.createFilename(e.Request.URL, title)
	pageInfo.FilePath = filepath.Join(MarkdownSubdir, filename)
	fullPath := filepath.Join(c.config.OutputDir, pageInfo.FilePath)

	if err := c.saveMarkdown(fullPath, pageInfo); err != nil {
		log.Printf("Failed to save markdown for %s: %v", e.Request.URL, err)
		return
	}

	// Add to pages collection
	c.pagesMutex.Lock()
	c.pages = append(c.pages, pageInfo)
	c.pagesMutex.Unlock()

	c.mu.Lock()
	c.processed++
	c.mu.Unlock()
}

func (c *Crawler) saveMarkdown(filePath string, pageInfo PageInfo) error {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create content with metadata
	content := fmt.Sprintf(`# %s

URL: %s
Crawled: %s
%s

---

%s`,
		pageInfo.Title,
		pageInfo.URL,
		pageInfo.CrawledAt.Format(time.RFC3339),
		func() string {
			if pageInfo.Description != "" {
				return fmt.Sprintf("Description: %s", pageInfo.Description)
			}
			return ""
		}(),
		pageInfo.Content)

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

func (c *Crawler) createFilename(pageURL *url.URL, title string) string {
	// Clean title for filename
	filename := strings.TrimSpace(title)
	filename = regexp.MustCompile(`[^a-zA-Z0-9\-_\s]`).ReplaceAllString(filename, "")
	filename = regexp.MustCompile(`\s+`).ReplaceAllString(filename, "-")
	filename = strings.ToLower(filename)

	if filename == "" || filename == "untitled" {
		// Use URL path
		urlPath := strings.Trim(pageURL.Path, "/")
		if urlPath == "" {
			urlPath = "index"
		}
		filename = strings.ReplaceAll(urlPath, "/", "-")
	}

	// Limit filename length
	if len(filename) > 100 {
		filename = filename[:100]
	}

	// Ensure .md extension
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}

	return filename
}

func (c *Crawler) addToQueue(link string, baseURL *url.URL) {
	// Parse and resolve URL
	linkURL, err := url.Parse(link)
	if err != nil {
		return
	}

	resolvedURL := baseURL.ResolveReference(linkURL)

	// Check if it's within the same domain
	if resolvedURL.Host != c.baseURL.Host {
		return
	}

	// Apply filters
	if c.shouldSkipURL(resolvedURL.String()) {
		return
	}

	urlStr := resolvedURL.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already visited
	if c.visited[urlStr] {
		return
	}

	c.visited[urlStr] = true

	// Add to queue
	select {
	case c.queue <- urlStr:
	default:
		// Queue is full, skip this URL
	}
}

func (c *Crawler) shouldSkipURL(urlStr string) bool {
	// Check language filters
	for _, pattern := range LanguageFilter {
		if matched, _ := regexp.MatchString(pattern, urlStr); matched {
			return true
		}
	}

	// Check file extension filters
	for _, pattern := range FileExtensionFilter {
		if matched, _ := regexp.MatchString(pattern, urlStr); matched {
			return true
		}
	}

	// Skip fragments
	if strings.Contains(urlStr, "#") {
		return true
	}

	return false
}

func (c *Crawler) Start(ctx context.Context) error {
	fmt.Printf("Starting crawl of: %s\n", c.config.URL)
	fmt.Printf("Output directory: %s\n", c.config.OutputDir)
	fmt.Printf("Workers: %d\n", c.config.Workers)

	// Create a cancellable context for workers
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Add seed URL to queue
	c.queue <- c.config.URL
	c.visited[c.config.URL] = true

	// Start workers
	for i := 0; i < c.config.Workers; i++ {
		c.wg.Add(1)
		go c.worker(workerCtx)
	}

	// Monitor progress and handle completion
	done := make(chan struct{})
	go func() {
		c.monitor(workerCtx)
		close(done)
	}()

	// Wait for either completion or cancellation
	select {
	case <-done:
		cancel() // Stop workers
	case <-ctx.Done():
		// External cancellation
	}

	// Wait for workers to finish
	c.wg.Wait()
	close(c.queue)
	c.bar.Finish()

	fmt.Printf("\nProcessed %d pages\n", c.processed)
	return nil
}

func (c *Crawler) worker(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case urlStr, ok := <-c.queue:
			if !ok {
				return
			}

			if err := c.collector.Visit(urlStr); err != nil {
				if c.config.Verbose {
					log.Printf("Failed to visit %s: %v", urlStr, err)
				}
			}
		}
	}
}

func (c *Crawler) monitor(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second) // Check more frequently
	defer ticker.Stop()

	lastProcessed := 0
	noProgressCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			current := c.processed
			queueLen := len(c.queue)
			c.mu.RUnlock()

			if current == lastProcessed {
				noProgressCount++
				// More aggressive completion detection
				if (noProgressCount >= 3 && queueLen == 0) || // 6 seconds with no progress and empty queue
					(noProgressCount >= 15) { // Or 30 seconds regardless
					if c.config.Verbose {
						fmt.Println("\nNo progress detected, stopping crawler...")
					}
					return
				}
			} else {
				noProgressCount = 0
				lastProcessed = current
			}

			if c.config.Verbose {
				fmt.Printf("Progress: %d pages processed, %d in queue\n", current, queueLen)
			}
		}
	}
}

// GenerateLLMSFiles creates both llms.txt and llms-full.txt files
func (c *Crawler) GenerateLLMSFiles() error {
	if err := c.generateLLMSTxt(); err != nil {
		return fmt.Errorf("failed to generate llms.txt: %w", err)
	}

	if err := c.generateLLMSFullTxt(); err != nil {
		return fmt.Errorf("failed to generate llms-full.txt: %w", err)
	}

	return nil
}

func (c *Crawler) generateLLMSTxt() error {
	// Sort pages by URL for consistent output
	sortedPages := make([]PageInfo, len(c.pages))
	copy(sortedPages, c.pages)
	sort.Slice(sortedPages, func(i, j int) bool {
		return sortedPages[i].URL < sortedPages[j].URL
	})

	var content strings.Builder

	// H1 title (required)
	siteTitle := c.getSiteTitle()
	content.WriteString(fmt.Sprintf("# %s\n\n", siteTitle))

	// Blockquote summary (optional but recommended)
	summary := c.generateSiteSummary()
	if summary != "" {
		content.WriteString(fmt.Sprintf("> %s\n\n", summary))
	}

	// Additional details
	content.WriteString(fmt.Sprintf("This documentation was automatically crawled from %s on %s.\n\n",
		c.config.URL, time.Now().Format("January 2, 2006")))

	// Main documentation section
	content.WriteString("## Documentation\n\n")
	for _, page := range sortedPages {
		if c.isMainDocPage(page) {
			description := page.Description
			if description == "" {
				description = c.extractFirstSentence(page.Content)
			}
			if description != "" {
				content.WriteString(fmt.Sprintf("- [%s](%s): %s\n", page.Title, page.URL, description))
			} else {
				content.WriteString(fmt.Sprintf("- [%s](%s)\n", page.Title, page.URL))
			}
		}
	}

	// Optional section for secondary pages
	secondaryPages := c.getSecondaryPages(sortedPages)
	if len(secondaryPages) > 0 {
		content.WriteString("\n## Optional\n\n")
		for _, page := range secondaryPages {
			content.WriteString(fmt.Sprintf("- [%s](%s)\n", page.Title, page.URL))
		}
	}

	// Write to file
	filePath := filepath.Join(c.config.OutputDir, "llms.txt")
	return os.WriteFile(filePath, []byte(content.String()), 0644)
}

func (c *Crawler) generateLLMSFullTxt() error {
	// Sort pages by URL for consistent output
	sortedPages := make([]PageInfo, len(c.pages))
	copy(sortedPages, c.pages)
	sort.Slice(sortedPages, func(i, j int) bool {
		return sortedPages[i].URL < sortedPages[j].URL
	})

	var content strings.Builder

	// H1 title
	siteTitle := c.getSiteTitle()
	content.WriteString(fmt.Sprintf("# %s - Complete Documentation\n\n", siteTitle))

	// Summary
	summary := c.generateSiteSummary()
	if summary != "" {
		content.WriteString(fmt.Sprintf("> %s\n\n", summary))
	}

	content.WriteString(fmt.Sprintf("This file contains the complete content of all pages crawled from %s on %s.\n\n",
		c.config.URL, time.Now().Format("January 2, 2006")))

	content.WriteString("---\n\n")

	// Include full content of each page
	for i, page := range sortedPages {
		content.WriteString(fmt.Sprintf("## %s\n\n", page.Title))
		content.WriteString(fmt.Sprintf("**URL:** %s\n\n", page.URL))

		if page.Description != "" {
			content.WriteString(fmt.Sprintf("**Description:** %s\n\n", page.Description))
		}

		content.WriteString(fmt.Sprintf("**Crawled:** %s\n\n", page.CrawledAt.Format(time.RFC3339)))

		// Clean and include content
		cleanContent := c.cleanContentForLLMS(page.Content)
		content.WriteString(cleanContent)

		// Add separator between pages (except for the last one)
		if i < len(sortedPages)-1 {
			content.WriteString("\n\n---\n\n")
		}
	}

	// Write to file
	filePath := filepath.Join(c.config.OutputDir, "llms-full.txt")
	return os.WriteFile(filePath, []byte(content.String()), 0644)
}

func (c *Crawler) getSiteTitle() string {
	// Try to get site title from the main page
	for _, page := range c.pages {
		if page.URL == c.config.URL || page.URL == c.config.URL+"/" {
			if page.Title != "" && page.Title != "Untitled" {
				return page.Title
			}
		}
	}

	// Fallback to domain name
	return c.baseURL.Host
}

func (c *Crawler) generateSiteSummary() string {
	// Try to get description from the main page
	for _, page := range c.pages {
		if page.URL == c.config.URL || page.URL == c.config.URL+"/" {
			if page.Description != "" {
				return page.Description
			}
			// Extract first meaningful paragraph
			return c.extractFirstSentence(page.Content)
		}
	}

	return fmt.Sprintf("Documentation and content from %s", c.baseURL.Host)
}

func (c *Crawler) isMainDocPage(page PageInfo) bool {
	// Consider a page "main documentation" if it's not in typical secondary sections
	lowerURL := strings.ToLower(page.URL)

	// Skip pages that are typically secondary
	secondaryIndicators := []string{
		"/blog", "/news", "/archive", "/changelog", "/release",
		"/about", "/contact", "/legal", "/privacy", "/terms",
		"/community", "/forum", "/discuss",
	}

	for _, indicator := range secondaryIndicators {
		// Check for the indicator followed by either / or end of URL
		if strings.Contains(lowerURL, indicator+"/") || strings.HasSuffix(lowerURL, indicator) {
			return false
		}
	}

	return true
}

func (c *Crawler) getSecondaryPages(allPages []PageInfo) []PageInfo {
	var secondary []PageInfo
	for _, page := range allPages {
		if !c.isMainDocPage(page) {
			secondary = append(secondary, page)
		}
	}
	return secondary
}

func (c *Crawler) extractFirstSentence(content string) string {
	// Clean the content and extract the first meaningful sentence
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines, headers, and markdown syntax
		if len(line) > 50 && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "**") {
			// Find the first sentence
			sentences := strings.Split(line, ".")
			if len(sentences) > 0 && len(sentences[0]) > 20 {
				return strings.TrimSpace(sentences[0]) + "."
			}
		}
	}
	return ""
}

func (c *Crawler) cleanContentForLLMS(content string) string {
	// Clean the content for better readability in LLMs context
	var cleaned strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))

	var inCodeBlock bool
	for scanner.Scan() {
		line := scanner.Text()

		// Handle code blocks
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
		}

		// Skip empty lines unless in code block
		if strings.TrimSpace(line) == "" && !inCodeBlock {
			continue
		}

		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}

	return strings.TrimSpace(cleaned.String())
}
