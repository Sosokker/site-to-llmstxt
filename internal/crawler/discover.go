package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"

	"github.com/Sosokker/site-to-llmstxt/internal/filters"
	"github.com/Sosokker/site-to-llmstxt/internal/models"
)

// PageSummary captures metadata for a crawled URL prior to full scraping.
type PageSummary struct {
	URL         string
	Title       string
	Description string
	Path        string
	Depth       int
}

// DiscoverOptions configure the URL discovery stage.
type DiscoverOptions struct {
	BaseURL    *url.URL
	Workers    int
	OnLog      func(string, ...interface{})
	OnProgress func(processed, queued int)
}

// Discover traverses links starting from the base URL and returns unique pages.
func Discover(ctx context.Context, opts DiscoverOptions) ([]PageSummary, *models.Stats, error) {
	if opts.BaseURL == nil {
		return nil, nil, errors.New("base URL is required")
	}

	stats := &models.Stats{StartTime: time.Now()}
	basePath := ""
	if opts.BaseURL != nil {
		basePath = opts.BaseURL.Path
	}
	statsMu := &sync.Mutex{}
	var (
		mu        sync.Mutex
		pages     = make([]PageSummary, 0, 128)
		seen      = make(map[string]struct{})
		queued    int64
		processed int64
	)

	collector := colly.NewCollector(
		colly.AllowedDomains(allowedDomains(opts.BaseURL.Host)...),
		colly.Async(true),
	)
	collector.SetRequestTimeout(30 * time.Second)

	if opts.Workers > 0 {
		if err := collector.Limit(&colly.LimitRule{
			DomainGlob:  "*",
			Parallelism: opts.Workers,
			RandomDelay: 500 * time.Millisecond,
		}); err != nil {
			return nil, nil, fmt.Errorf("configure collector: %w", err)
		}
	}

	collector.OnRequest(func(r *colly.Request) {
		select {
		case <-ctx.Done():
			r.Abort()
			return
		default:
		}

		if opts.OnLog != nil {
			opts.OnLog("discover: visiting %s", r.URL.String())
		}
	})

	collector.OnError(func(r *colly.Response, err error) {
		statsMu.Lock()
		stats.AddError()
		statsMu.Unlock()
		if opts.OnLog != nil {
			opts.OnLog("discover: error fetching %s: %v", r.Request.URL, err)
		}
	})

	collector.OnHTML("html", func(e *colly.HTMLElement) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		pageURL := e.Request.URL.String()
		atomic.AddInt64(&queued, -1)
		currentProcessed := atomic.AddInt64(&processed, 1)
		defer func() {
			if opts.OnProgress != nil {
				opts.OnProgress(int(currentProcessed), int(max64(atomic.LoadInt64(&queued), 0)))
			}
		}()

		mu.Lock()
		if _, ok := seen[pageURL]; ok {
			mu.Unlock()
		} else {
			seen[pageURL] = struct{}{}
			mu.Unlock()

			statsMu.Lock()
			stats.TotalPages++
			if filters.IsMainDocPage(pageURL) {
				stats.MainDocPages++
			} else {
				stats.SecondaryPages++
			}
			statsMu.Unlock()

			title := strings.TrimSpace(e.DOM.Find("title").First().Text())
			if title == "" {
				title = "Untitled"
			}

			description := strings.TrimSpace(e.DOM.Find(`meta[name="description"]`).AttrOr("content", ""))
			if description == "" {
				description = guessDescription(e.DOM)
			}

			summary := PageSummary{
				URL:         pageURL,
				Title:       title,
				Description: description,
				Path:        e.Request.URL.Path,
				Depth:       e.Request.Depth,
			}

			mu.Lock()
			pages = append(pages, summary)
			mu.Unlock()
		}

		e.DOM.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
			href, exists := sel.Attr("href")
			if !exists || href == "" {
				return
			}

			absolute := e.Request.AbsoluteURL(href)
			if absolute == "" || !strings.HasPrefix(absolute, "http") {
				return
			}

			if filters.ShouldSkipURL(absolute, opts.BaseURL.Host, basePath) {
				statsMu.Lock()
				stats.AddSkipped()
				statsMu.Unlock()
				return
			}

			if err := collector.Visit(absolute); err != nil {
				var alreadyVisited *colly.AlreadyVisitedError
				if errors.As(err, &alreadyVisited) {
					return
				}
				statsMu.Lock()
				stats.AddError()
				statsMu.Unlock()
				if opts.OnLog != nil {
					opts.OnLog("discover: failed to queue %s: %v", absolute, err)
				}
				return
			}

			atomic.AddInt64(&queued, 1)
		})
	})

	atomic.AddInt64(&queued, 1)
	if err := collector.Visit(opts.BaseURL.String()); err != nil {
		var alreadyVisited *colly.AlreadyVisitedError
		if !errors.As(err, &alreadyVisited) {
			return nil, nil, fmt.Errorf("start discovery: %w", err)
		}
	}

	collector.Wait()

	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, nil, err
	}

	stats.Finish()
	return pages, stats, nil
}

func guessDescription(sel *goquery.Selection) string {
	selection := sel.Find("p")
	for i := range selection.Nodes {
		paragraph := selection.Eq(i).Text()
		paragraph = strings.TrimSpace(paragraph)
		if paragraph != "" {
			if len(paragraph) > 240 {
				return paragraph[:240] + "..."
			}
			return paragraph
		}
	}
	return ""
}
