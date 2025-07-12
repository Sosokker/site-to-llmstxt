package main

import (
	"net/url"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "Valid config",
			config: &Config{
				URL:       "https://example.com",
				OutputDir: "./output",
				Workers:   1,
			},
			wantErr: false,
		},
		{
			name: "Empty URL",
			config: &Config{
				URL:       "",
				OutputDir: "./output",
				Workers:   1,
			},
			wantErr: true,
		},
		{
			name: "Invalid URL",
			config: &Config{
				URL:       "not-a-url",
				OutputDir: "./output",
				Workers:   1,
			},
			wantErr: true,
		},
		{
			name: "Zero workers",
			config: &Config{
				URL:       "https://example.com",
				OutputDir: "./output",
				Workers:   0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateFilename(t *testing.T) {
	config := &Config{
		URL:       "https://example.com",
		OutputDir: "./test-output",
		Workers:   1,
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	tests := []struct {
		name     string
		url      string
		title    string
		expected string
	}{
		{
			name:     "Normal title",
			url:      "https://example.com/about",
			title:    "About Us",
			expected: "about-us.md",
		},
		{
			name:     "Title with special characters",
			url:      "https://example.com/contact",
			title:    "Contact Us! (Get in Touch)",
			expected: "contact-us-get-in-touch.md",
		},
		{
			name:     "Empty title",
			url:      "https://example.com/services/web-design",
			title:    "",
			expected: "services-web-design.md",
		},
		{
			name:     "Root URL",
			url:      "https://example.com/",
			title:    "Homepage",
			expected: "homepage.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageURL, _ := url.Parse(tt.url)
			result := crawler.createFilename(pageURL, tt.title)
			if result != tt.expected {
				t.Errorf("createFilename(%q, %q) = %q, want %q", tt.url, tt.title, result, tt.expected)
			}
		})
	}
}

func TestShouldSkipURL(t *testing.T) {
	config := &Config{
		URL:       "https://example.com",
		OutputDir: "./test-output",
		Workers:   1,
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"Normal URL", "https://example.com/page", false},
		{"Language URL - en", "https://example.com/en/page", true},
		{"Language URL - zh", "https://example.com/zh/page", true},
		{"Language URL - zh-hant", "https://example.com/zh-hant/page", true},
		{"PDF file", "https://example.com/document.pdf", true},
		{"ZIP file", "https://example.com/archive.zip", true},
		{"Fragment URL", "https://example.com/page#section", true},
		{"Image file", "https://example.com/image.jpg", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crawler.shouldSkipURL(tt.url)
			if result != tt.expected {
				t.Errorf("shouldSkipURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestExtractFirstSentence(t *testing.T) {
	config := &Config{
		URL:       "https://example.com",
		OutputDir: "./test-output",
		Workers:   1,
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "Simple sentence",
			content:  "This is a simple sentence about something interesting. This is another sentence.",
			expected: "This is a simple sentence about something interesting.",
		},
		{
			name:     "With headers",
			content:  "# Header\n\nThis is the main content that should be extracted as the first sentence.",
			expected: "This is the main content that should be extracted as the first sentence.",
		},
		{
			name:     "Short content",
			content:  "Short text",
			expected: "",
		},
		{
			name:     "Empty content",
			content:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crawler.extractFirstSentence(tt.content)
			if result != tt.expected {
				t.Errorf("extractFirstSentence() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsMainDocPage(t *testing.T) {
	config := &Config{
		URL:       "https://example.com",
		OutputDir: "./test-output",
		Workers:   1,
	}

	crawler, err := NewCrawler(config)
	if err != nil {
		t.Fatalf("Failed to create crawler: %v", err)
	}

	tests := []struct {
		name     string
		page     PageInfo
		expected bool
	}{
		{
			name:     "Main documentation page",
			page:     PageInfo{URL: "https://example.com/docs/getting-started"},
			expected: true,
		},
		{
			name:     "Blog page",
			page:     PageInfo{URL: "https://example.com/blog/latest-news"},
			expected: false,
		},
		{
			name:     "About page",
			page:     PageInfo{URL: "https://example.com/about"},
			expected: false,
		},
		{
			name:     "API documentation",
			page:     PageInfo{URL: "https://example.com/api/reference"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crawler.isMainDocPage(tt.page)
			if result != tt.expected {
				t.Errorf("isMainDocPage() = %v, want %v", result, tt.expected)
			}
		})
	}
}
