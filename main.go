package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/debug"
	"github.com/schollz/progressbar/v3"
)

// Config holds crawler configuration
type Config struct {
	URL       string
	OutputDir string
	Workers   int
	Verbose   bool
}

// Crawler manages the web crawling process
type Crawler struct {
	config    *Config
	collector *colly.Collector
	converter *converter.Converter
	visited   map[string]bool
	queue     chan string
	wg        sync.WaitGroup
	mu        sync.RWMutex
	baseURL   *url.URL
	bar       *progressbar.ProgressBar
	processed int
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
	config := parseFlags()

	if err := validateConfig(config); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		log.Fatalf("Failed to create crawler: %v", err)
	}

	ctx := context.Background()
	if err := crawler.Start(ctx); err != nil {
		log.Fatalf("Crawling failed: %v", err)
	}

	fmt.Printf("\nCrawling completed successfully! Files saved to: %s\n", config.OutputDir)
}

func parseFlags() *Config {
	config := &Config{}

	flag.StringVar(&config.URL, "url", "", "Root URL to crawl (required)")
	flag.StringVar(&config.OutputDir, "output", "./output", "Output directory for markdown files")
	flag.IntVar(&config.Workers, "workers", 5, "Number of concurrent workers")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	return config
}

func validateConfig(config *Config) error {
	if config.URL == "" {
		return fmt.Errorf("URL is required")
	}

	parsedURL, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check if URL has a valid scheme and host
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("URL must include scheme (http/https) and host")
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

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
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
		Delay:       100 * time.Millisecond,
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
	}

	crawler.setupCallbacks()

	return crawler, nil
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
	title := e.ChildText("title")
	if title == "" {
		title = "untitled"
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

	// Save to file
	if err := c.saveMarkdown(e.Request.URL, title, markdown); err != nil {
		log.Printf("Failed to save markdown for %s: %v", e.Request.URL, err)
		return
	}

	c.mu.Lock()
	c.processed++
	c.mu.Unlock()
}

func (c *Crawler) saveMarkdown(pageURL *url.URL, title, markdown string) error {
	// Create filename from URL path
	filename := c.createFilename(pageURL, title)
	filePath := filepath.Join(c.config.OutputDir, filename)

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Add metadata header
	content := fmt.Sprintf("# %s\n\nURL: %s\nCrawled: %s\n\n---\n\n%s",
		title, pageURL.String(), time.Now().Format(time.RFC3339), markdown)

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

	// Skip fragments and query parameters that might be irrelevant
	if strings.Contains(urlStr, "#") {
		return true
	}

	return false
}

func (c *Crawler) Start(ctx context.Context) error {
	fmt.Printf("Starting crawl of: %s\n", c.config.URL)
	fmt.Printf("Output directory: %s\n", c.config.OutputDir)
	fmt.Printf("Workers: %d\n", c.config.Workers)

	// Add seed URL to queue
	c.queue <- c.config.URL
	c.visited[c.config.URL] = true

	// Start workers
	for i := 0; i < c.config.Workers; i++ {
		c.wg.Add(1)
		go c.worker(ctx)
	}

	// Monitor progress
	go c.monitor(ctx)

	// Wait for completion
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
	ticker := time.NewTicker(5 * time.Second)
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
				if noProgressCount >= 6 && queueLen == 0 { // 30 seconds with no progress and empty queue
					fmt.Println("\nNo progress detected, stopping crawler...")
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
