package filters

import "testing"

func TestShouldSkipURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		baseHost string
		want     bool
	}{
		{
			name:     "Normal URL",
			url:      "https://example.com/docs",
			baseHost: "example.com",
			want:     false,
		},
		{
			name:     "Language URL - en",
			url:      "https://example.com/en/docs",
			baseHost: "example.com",
			want:     true,
		},
		{
			name:     "Language URL - zh",
			url:      "https://example.com/zh/docs",
			baseHost: "example.com",
			want:     true,
		},
		{
			name:     "PDF file",
			url:      "https://example.com/doc.pdf",
			baseHost: "example.com",
			want:     true,
		},
		{
			name:     "ZIP file",
			url:      "https://example.com/download.zip",
			baseHost: "example.com",
			want:     true,
		},
		{
			name:     "Fragment URL",
			url:      "https://example.com/docs#section",
			baseHost: "example.com",
			want:     true,
		},
		{
			name:     "External domain",
			url:      "https://other.com/docs",
			baseHost: "example.com",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldSkipURL(tt.url, tt.baseHost); got != tt.want {
				t.Errorf("ShouldSkipURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMainDocPage(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "Main documentation page",
			url:  "https://example.com/docs/api",
			want: true,
		},
		{
			name: "Blog page",
			url:  "https://example.com/blog/latest-news",
			want: false,
		},
		{
			name: "About page",
			url:  "https://example.com/about",
			want: false,
		},
		{
			name: "API documentation",
			url:  "https://example.com/api/reference",
			want: true,
		},
		{
			name: "Contact page",
			url:  "https://example.com/contact",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMainDocPage(tt.url); got != tt.want {
				t.Errorf("IsMainDocPage() = %v, want %v", got, tt.want)
			}
		})
	}
}
