package filters

import (
	"net/url"
	"strings"
)

// LanguageIndicators are URL patterns that indicate language-specific pages.
var LanguageIndicators = []string{
	"/en/", "/zh/", "/fr/", "/de/", "/es/", "/it/", "/ja/", "/ko/",
	"/pt/", "/ru/", "/ar/", "/hi/", "/th/", "/vi/", "/id/", "/ms/",
	"/tl/", "/zh-cn/", "/zh-tw/", "/zh-hk/", "/zh-hant/", "/zh-hans/",
	"/en-us/", "/en-gb/", "/fr-fr/", "/de-de/", "/es-es/", "/pt-br/",
	"/pt-pt/", "/ja-jp/", "/ko-kr/", "/it-it/", "/ru-ru/", "/ar-sa/",
}

// FileExtensions are file extensions that should be skipped.
var FileExtensions = []string{
	".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
	".zip", ".rar", ".tar", ".gz", ".7z", ".bz2",
	".mp3", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm",
	".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp",
	".exe", ".msi", ".dmg", ".deb", ".rpm", ".pkg",
}

// SecondaryPageIndicators are URL patterns for secondary content.
var SecondaryPageIndicators = []string{
	"/blog", "/news", "/archive", "/changelog", "/release",
	"/about", "/contact", "/legal", "/privacy", "/terms",
	"/community", "/forum", "/discuss",
}

// ShouldSkipURL determines if a URL should be skipped based on various filters.
func ShouldSkipURL(rawURL, baseHost, basePath string) bool {
	if rawURL == "" {
		return true
	}

	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}

	// Skip external domains
	if u.Host != "" && u.Host != baseHost {
		return true
	}

	// Skip fragments
	if u.Fragment != "" {
		return true
	}

	lowerURL := strings.ToLower(rawURL)
	basePathLower := strings.ToLower(basePath)
	if basePathLower != "" && !strings.HasPrefix(basePathLower, "/") {
		basePathLower = "/" + basePathLower
	}
	basePathLower = strings.TrimRight(basePathLower, "/")

	candidatePath := strings.ToLower(u.Path)
	baseClean := strings.TrimSuffix(basePathLower, "/")
	if baseClean != "" && baseClean != "/" {
		if candidatePath != baseClean && !strings.HasPrefix(candidatePath, baseClean+"/") {
			return true
		}
	}

	// Skip language variants
	for _, lang := range LanguageIndicators {
		if strings.Contains(lowerURL, lang) {
			if basePathLower != "" && strings.Contains(basePathLower, lang) {
				continue
			}
			return true
		}
	}

	// Skip file downloads
	for _, ext := range FileExtensions {
		if strings.HasSuffix(lowerURL, ext) {
			return true
		}
	}

	return false
}

// IsMainDocPage determines if a page is main documentation or secondary content.
func IsMainDocPage(pageURL string) bool {
	lowerURL := strings.ToLower(pageURL)

	for _, indicator := range SecondaryPageIndicators {
		// Check for the indicator followed by either / or end of URL
		if strings.Contains(lowerURL, indicator+"/") || strings.HasSuffix(lowerURL, indicator) {
			return false
		}
	}

	return true
}
