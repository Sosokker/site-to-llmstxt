package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"

	"github.com/Sosokker/site-to-llmstxt/internal/config"
	"github.com/Sosokker/site-to-llmstxt/internal/filters"
	"github.com/Sosokker/site-to-llmstxt/internal/models"
	"github.com/Sosokker/site-to-llmstxt/internal/utils"
)

// ProgressUpdate conveys scraping progress for interactive UIs.
type ProgressUpdate struct {
	Completed int
	Total     int
	URL       string
}

// LogUpdate captures a log line emitted during scraping.
type LogUpdate struct {
	Message string
}

// ScrapeOptions configure the download stage for selected pages.
type ScrapeOptions struct {
	BaseURL  *url.URL
	Pages    []PageSummary
	Output   string
	Workers  int
	Verbose  bool
	Logs     chan<- LogUpdate
	Progress chan<- ProgressUpdate
}

// Scrape fetches each provided page, writes Markdown output, and returns results.
func Scrape(ctx context.Context, opts ScrapeOptions) ([]models.PageInfo, *models.Stats, error) {
	if opts.BaseURL == nil {
		return nil, nil, errors.New("base URL is required")
	}
	if len(opts.Pages) == 0 {
		return nil, nil, errors.New("no pages selected for scraping")
	}

	if opts.Workers <= 0 {
		opts.Workers = config.DefaultWorkers
	}

	if err := utils.CreateOutputDirs(opts.Output); err != nil {
		return nil, nil, err
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	stats := &models.Stats{
		StartTime:      time.Now(),
		TotalPages:     len(opts.Pages),
		MainDocPages:   0,
		SecondaryPages: 0,
		SkippedURLs:    0,
		ErrorCount:     0,
	}
	statsMu := &sync.Mutex{}

	for _, page := range opts.Pages {
		if filters.IsMainDocPage(page.URL) {
			stats.MainDocPages++
		} else {
			stats.SecondaryPages++
		}
	}

	namer := utils.NewUniqueNamer()
	results := make([]models.PageInfo, 0, len(opts.Pages))
	resultsMu := &sync.Mutex{}

	var completed int32
	progressTotal := len(opts.Pages)

	pageCh := make(chan PageSummary)
	wg := sync.WaitGroup{}
	errOnce := sync.Once{}
	var firstErr error

	sendLog := func(always bool, format string, args ...interface{}) {
		if !always && !opts.Verbose {
			return
		}
		if opts.Logs == nil {
			return
		}
		msg := fmt.Sprintf(format, args...)
		select {
		case opts.Logs <- LogUpdate{Message: msg}:
		default:
		}
	}

	sendProgress := func(url string) {
		if opts.Progress == nil {
			return
		}
		done := int(atomic.AddInt32(&completed, 1))
		select {
		case opts.Progress <- ProgressUpdate{
			Completed: done,
			Total:     progressTotal,
			URL:       url,
		}:
		default:
		}
	}

	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range pageCh {
				select {
				case <-ctx.Done():
					return
				default:
				}

				info, err := fetchPage(ctx, client, page, opts.Output, namer)
				if err != nil {
					errOnce.Do(func() { firstErr = err })
					statsMu.Lock()
					stats.AddError()
					statsMu.Unlock()
					sendLog(true, "error scraping %s: %v", page.URL, err)
					sendProgress(page.URL)
					continue
				}

				resultsMu.Lock()
				results = append(results, info)
				resultsMu.Unlock()

				sendLog(false, "scraped %s", page.URL)
				sendProgress(page.URL)
			}
		}()
	}

	go func() {
		for _, page := range opts.Pages {
			pageCh <- page
		}
		close(pageCh)
	}()

	wg.Wait()

	stats.Finish()

	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return results, stats, err
	}

	return results, stats, firstErr
}

func fetchPage(ctx context.Context, client *http.Client, page PageSummary, outputDir string, namer *utils.UniqueNamer) (models.PageInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page.URL, nil)
	if err != nil {
		return models.PageInfo{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return models.PageInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return models.PageInfo{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.PageInfo{}, err
	}

	htmlContent := string(bodyBytes)
	markdown, err := htmltomarkdown.ConvertString(htmlContent)
	if err != nil {
		return models.PageInfo{}, err
	}
	markdown = strings.TrimSpace(markdown)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return models.PageInfo{}, err
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title == "" {
		title = page.Title
	}

	description := strings.TrimSpace(doc.Find(`meta[name="description"]`).AttrOr("content", ""))
	if description == "" {
		description = page.Description
	}
	if description == "" {
		description = utils.ExtractFirstSentence(markdown)
	}

	filename := utils.CreateFilename(title, page.URL)
	filename = namer.Reserve(filename)
	relativePath := filepath.Join(config.MarkdownSubdir, filename)
	fullPath := filepath.Join(outputDir, relativePath)

	if err := os.WriteFile(fullPath, []byte(markdown), 0o644); err != nil {
		return models.PageInfo{}, err
	}

	info := models.PageInfo{
		URL:         page.URL,
		Title:       title,
		Content:     markdown,
		FilePath:    relativePath,
		CrawledAt:   time.Now(),
		Description: description,
	}

	return info, nil
}
