package utils

import "testing"

func TestCreateFilename(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		rawURL string
		want   string
	}{
		{
			name:   "Normal title",
			title:  "Getting Started",
			rawURL: "https://example.com/getting-started",
			want:   "Getting-Started.md",
		},
		{
			name:   "Title with special characters",
			title:  "API Reference: <Advanced>",
			rawURL: "https://example.com/api",
			want:   "API-Reference-Advanced.md",
		},
		{
			name:   "Empty title",
			title:  "",
			rawURL: "https://example.com/api/reference",
			want:   "reference.md",
		},
		{
			name:   "Root URL",
			title:  "",
			rawURL: "https://example.com/",
			want:   "index.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CreateFilename(tt.title, tt.rawURL); got != tt.want {
				t.Errorf("CreateFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractFirstSentence(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "Simple sentence",
			content: "This is a simple sentence. This is another sentence.",
			want:    "This is a simple sentence.",
		},
		{
			name:    "With headers",
			content: "# Header\n\nThis is the first sentence. Another sentence follows.",
			want:    "This is the first sentence.",
		},
		{
			name:    "Short content",
			content: "Short content without period",
			want:    "Short content without period",
		},
		{
			name:    "Empty content",
			content: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractFirstSentence(tt.content); got != tt.want {
				t.Errorf("ExtractFirstSentence() = %v, want %v", got, tt.want)
			}
		})
	}
}
