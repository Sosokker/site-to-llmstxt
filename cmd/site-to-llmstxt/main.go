package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"github.com/urfave/cli/v2"

	"github.com/Sosokker/site-to-llmstxt/internal/config"
	"github.com/Sosokker/site-to-llmstxt/internal/filters"
	"github.com/Sosokker/site-to-llmstxt/internal/generator"
	"github.com/Sosokker/site-to-llmstxt/internal/models"
	"github.com/Sosokker/site-to-llmstxt/internal/progress"
	"github.com/Sosokker/site-to-llmstxt/internal/tui"
	"github.com/Sosokker/site-to-llmstxt/internal/utils"
)

func main() {
	log.SetFlags(0)

	app := &cli.App{
		Name:  "site-to-llmstxt",
		Usage: "Crawl a documentation site and generate llms.txt outputs",
		Commands: []*cli.Command{
			{
				Name:  "tui",
				Usage: "Launch interactive terminal UI",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "output",
						Usage: "Output directory",
						Value: config.DefaultOutputDir,
					},
					&cli.IntFlag{
						Name:  "workers",
						Usage: "Default worker count used during discovery",
						Value: config.DefaultWorkers,
					},
				},
				Action: func(cliCtx *cli.Context) error {
					ctx, cancel := signal.NotifyContext(cliCtx.Context, os.Interrupt)
					defer cancel()

					opts := tui.Options{
						OutputDir:      cliCtx.String("output"),
						DefaultWorkers: cliCtx.Int("workers"),
					}

					return tui.Run(ctx, opts)
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "url",
				Aliases: []string{"u"},
				Usage:   "Start `URL` to crawl (required)",
				EnvVars: []string{"SITE_TO_LLMSTXT_URL"},
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
				EnvVars: []string{"SITE_TO_LLMSTXT_OUTPUT"},
				Value:   config.DefaultOutputDir,
			},
			&cli.IntFlag{
				Name:    "workers",
				Aliases: []string{"w"},
				Usage:   "Number of concurrent workers",
				EnvVars: []string{"SITE_TO_LLMSTXT_WORKERS"},
				Value:   config.DefaultWorkers,
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Usage:   "Enable verbose progress logging",
				EnvVars: []string{"SITE_TO_LLMSTXT_VERBOSE"},
			},
		},
		Action: func(cliCtx *cli.Context) error {
			ctx, cancel := signal.NotifyContext(cliCtx.Context, os.Interrupt)
			defer cancel()

			cfg := &config.Config{
				URL:       cliCtx.String("url"),
				OutputDir: cliCtx.String("output"),
				Workers:   cliCtx.Int("workers"),
				Verbose:   cliCtx.Bool("verbose"),
			}

			if cfg.Workers <= 0 {
				cfg.Workers = config.DefaultWorkers
			}
			if cfg.OutputDir == "" {
				cfg.OutputDir = config.DefaultOutputDir
			}

			if err := cfg.Validate(); err != nil {
				return err
			}

			baseURL, err := url.Parse(cfg.URL)
			if err != nil {
				return fmt.Errorf("parse URL: %w", err)
			}

			return crawlAndGenerate(ctx, baseURL, cfg)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func crawlAndGenerate(ctx context.Context, baseURL *url.URL, cfg *config.Config) error {
	if err := utils.CreateOutputDirs(cfg.OutputDir); err != nil {
		return err
	}

	stats := &models.Stats{StartTime: time.Now()}
	basePath := ""
	if baseURL != nil {
		basePath = baseURL.Path
	}
	statsMu := &sync.Mutex{}
	progressManager := progress.New(cfg.Verbose, stats)
	defer func() {
		statsMu.Lock()
		stats.Finish()
		statsMu.Unlock()
		progressManager.Finish()
	}()

	pages := make([]models.PageInfo, 0, 128)
	pagesMu := &sync.Mutex{}
	namer := utils.NewUniqueNamer()

	var queued, processed int64

	collector := colly.NewCollector(
		colly.AllowedDomains(allowedDomains(baseURL.Host)...),
		colly.Async(true),
	)
	collector.SetRequestTimeout(30 * time.Second)

	if cfg.Workers > 0 {
		if err := collector.Limit(&colly.LimitRule{
			DomainGlob:  "*",
			Parallelism: cfg.Workers,
			RandomDelay: 500 * time.Millisecond,
		}); err != nil {
			return fmt.Errorf("configure collector: %w", err)
		}
	}

	collector.OnRequest(func(r *colly.Request) {
		select {
		case <-ctx.Done():
			r.Abort()
			return
		default:
		}

		if cfg.Verbose {
			progressManager.Log("Visiting %s", r.URL.String())
		}
	})

	collector.OnError(func(r *colly.Response, err error) {
		statsMu.Lock()
		stats.AddError()
		statsMu.Unlock()
		progressManager.Log("Error fetching %s: %v", r.Request.URL, err)
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
			progressManager.Update(int(currentProcessed), int(max64(atomic.LoadInt64(&queued), 0)))
		}()

		title := strings.TrimSpace(e.DOM.Find("title").First().Text())
		if title == "" {
			title = "Untitled"
		}

		description := strings.TrimSpace(e.DOM.Find(`meta[name="description"]`).AttrOr("content", ""))

		markdown, err := htmltomarkdown.ConvertString(string(e.Response.Body))
		if err != nil {
			statsMu.Lock()
			stats.AddError()
			statsMu.Unlock()
			progressManager.Log("Failed to convert %s: %v", pageURL, err)
			return
		}
		markdown = strings.TrimSpace(markdown)

		if description == "" {
			description = utils.ExtractFirstSentence(markdown)
		}

		filename := utils.CreateFilename(title, pageURL)
		filename = namer.Reserve(filename)
		relativePath := filepath.Join(config.MarkdownSubdir, filename)
		fullPath := filepath.Join(cfg.OutputDir, relativePath)

		if err := os.WriteFile(fullPath, []byte(markdown), 0644); err != nil {
			statsMu.Lock()
			stats.AddError()
			statsMu.Unlock()
			progressManager.Log("Failed to write %s: %v", fullPath, err)
			return
		}

		pageInfo := models.PageInfo{
			URL:         pageURL,
			Title:       title,
			Content:     markdown,
			FilePath:    relativePath,
			CrawledAt:   time.Now(),
			Description: description,
		}

		pagesMu.Lock()
		pages = append(pages, pageInfo)
		pagesMu.Unlock()

		statsMu.Lock()
		stats.TotalPages++
		if filters.IsMainDocPage(pageURL) {
			stats.MainDocPages++
		} else {
			stats.SecondaryPages++
		}
		statsMu.Unlock()

		e.DOM.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
			href, exists := sel.Attr("href")
			if !exists || href == "" {
				return
			}

			absolute := e.Request.AbsoluteURL(href)
			if absolute == "" {
				return
			}

			if !strings.HasPrefix(absolute, "http") {
				return
			}

			if filters.ShouldSkipURL(absolute, baseURL.Host, basePath) {
				statsMu.Lock()
				stats.AddSkipped()
				statsMu.Unlock()
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			if err := collector.Visit(absolute); err != nil {
				var alreadyVisited *colly.AlreadyVisitedError
				if errors.As(err, &alreadyVisited) {
					return
				}

				statsMu.Lock()
				stats.AddError()
				statsMu.Unlock()
				progressManager.Log("Failed to queue %s: %v", absolute, err)
				return
			}

			atomic.AddInt64(&queued, 1)
		})
	})

	atomic.AddInt64(&queued, 1)
	if err := collector.Visit(baseURL.String()); err != nil {
		var alreadyVisited *colly.AlreadyVisitedError
		if !errors.As(err, &alreadyVisited) {
			return fmt.Errorf("start crawl: %w", err)
		}
	}

	collector.Wait()

	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	if len(pages) == 0 {
		return errors.New("no pages were crawled; check URL or filters")
	}

	gen := generator.New(baseURL, cfg.OutputDir)
	if err := gen.Generate(pages); err != nil {
		return fmt.Errorf("generate outputs: %w", err)
	}

	return nil
}

func allowedDomains(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}

	domains := map[string]struct{}{
		host: {},
	}

	if strings.HasPrefix(host, "www.") {
		domains[strings.TrimPrefix(host, "www.")] = struct{}{}
	} else {
		domains["www."+host] = struct{}{}
	}

	list := make([]string, 0, len(domains))
	for d := range domains {
		list = append(list, d)
	}
	return list
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
