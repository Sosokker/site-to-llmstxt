package generator

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sosokker/site-to-llmstxt/internal/filters"
	"github.com/Sosokker/site-to-llmstxt/internal/models"
	"github.com/Sosokker/site-to-llmstxt/internal/utils"
)

// LLMsGenerator generates LLMs.txt format files.
type LLMsGenerator struct {
	baseURL   *url.URL
	outputDir string
}

// New creates a new LLMs.txt generator.
func New(baseURL *url.URL, outputDir string) *LLMsGenerator {
	return &LLMsGenerator{
		baseURL:   baseURL,
		outputDir: outputDir,
	}
}

// Generate creates both llms.txt and llms-full.txt files.
func (g *LLMsGenerator) Generate(pages []models.PageInfo) error {
	if err := g.generateLLMsFile(pages); err != nil {
		return fmt.Errorf("failed to generate llms.txt: %w", err)
	}

	if err := g.generateFullFile(pages); err != nil {
		return fmt.Errorf("failed to generate llms-full.txt: %w", err)
	}

	return nil
}

func (g *LLMsGenerator) generateLLMsFile(pages []models.PageInfo) error {
	var content strings.Builder

	// Header
	siteName := g.baseURL.Host
	if siteName == "" {
		siteName = "Documentation"
	}

	content.WriteString(fmt.Sprintf("# %s\n\n", siteName))

	// Summary from first page or generate one
	summary := g.generateSummary(pages)
	if summary != "" {
		content.WriteString(fmt.Sprintf("> %s\n\n", summary))
	}

	content.WriteString(fmt.Sprintf("This documentation was automatically crawled from %s on %s.\n\n",
		g.baseURL.String(), time.Now().Format("January 2, 2006")))

	// Main documentation section
	mainPages := g.filterMainPages(pages)
	if len(mainPages) > 0 {
		content.WriteString("## Documentation\n\n")
		g.writePageLinks(&content, mainPages)
	}

	// Optional section for secondary content
	secondaryPages := g.filterSecondaryPages(pages)
	if len(secondaryPages) > 0 {
		content.WriteString("\n## Optional\n\n")
		g.writePageLinks(&content, secondaryPages)
	}

	return g.writeFile("llms.txt", content.String())
}

func (g *LLMsGenerator) generateFullFile(pages []models.PageInfo) error {
	var content strings.Builder

	// Header
	siteName := g.baseURL.Host
	content.WriteString(fmt.Sprintf("# %s - Complete Documentation\n\n", siteName))

	summary := g.generateSummary(pages)
	if summary != "" {
		content.WriteString(fmt.Sprintf("> %s\n\n", summary))
	}

	content.WriteString(fmt.Sprintf("This file contains the complete content of all pages crawled from %s on %s.\n\n",
		g.baseURL.String(), time.Now().Format("January 2, 2006")))

	content.WriteString(strings.Repeat("-", 80) + "\n\n")

	// Sort pages by URL for consistent output
	sortedPages := make([]models.PageInfo, len(pages))
	copy(sortedPages, pages)
	sort.Slice(sortedPages, func(i, j int) bool {
		return sortedPages[i].URL < sortedPages[j].URL
	})

	// Add each page's content
	for i, page := range sortedPages {
		if i > 0 {
			content.WriteString("\n" + strings.Repeat("-", 80) + "\n\n")
		}

		content.WriteString(fmt.Sprintf("## %s\n\n", page.Title))
		content.WriteString(fmt.Sprintf("**URL:** %s\n\n", page.URL))
		content.WriteString(fmt.Sprintf("**Crawled:** %s\n\n", page.CrawledAt.Format(time.RFC3339)))

		if page.Content != "" {
			content.WriteString(page.Content + "\n")
		}
	}

	return g.writeFile("llms-full.txt", content.String())
}

func (g *LLMsGenerator) generateSummary(pages []models.PageInfo) string {
	// Try to get summary from the first page (usually homepage)
	if len(pages) > 0 {
		for _, page := range pages {
			if page.Description != "" {
				return page.Description
			}
		}

		// Fallback to first sentence of first page content
		for _, page := range pages {
			if page.Content != "" {
				return utils.ExtractFirstSentence(page.Content)
			}
		}
	}

	return ""
}

func (g *LLMsGenerator) filterMainPages(pages []models.PageInfo) []models.PageInfo {
	var main []models.PageInfo
	for _, page := range pages {
		if filters.IsMainDocPage(page.URL) {
			main = append(main, page)
		}
	}

	// Sort by URL
	sort.Slice(main, func(i, j int) bool {
		return main[i].URL < main[j].URL
	})

	return main
}

func (g *LLMsGenerator) filterSecondaryPages(pages []models.PageInfo) []models.PageInfo {
	var secondary []models.PageInfo
	for _, page := range pages {
		if !filters.IsMainDocPage(page.URL) {
			secondary = append(secondary, page)
		}
	}

	// Sort by URL
	sort.Slice(secondary, func(i, j int) bool {
		return secondary[i].URL < secondary[j].URL
	})

	return secondary
}

func (g *LLMsGenerator) writePageLinks(content *strings.Builder, pages []models.PageInfo) {
	for _, page := range pages {
		title := page.Title
		if title == "" || title == "Untitled" {
			title = "Untitled"
		}

		description := page.Description
		if description == "" && page.Content != "" {
			description = utils.ExtractFirstSentence(page.Content)
		}

		if description != "" {
			content.WriteString(fmt.Sprintf("- [%s](%s): %s\n", title, page.URL, description))
		} else {
			content.WriteString(fmt.Sprintf("- [%s](%s)\n", title, page.URL))
		}
	}
}

func (g *LLMsGenerator) writeFile(filename, content string) error {
	path := filepath.Join(g.outputDir, filename)
	return os.WriteFile(path, []byte(content), 0644)
}
