package utils

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	filenameRegex = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	spaceRegex    = regexp.MustCompile(`\s+`)
)

// CreateFilename creates a safe filename from a title and URL.
func CreateFilename(title, rawURL string) string {
	if title == "" || title == "Untitled" {
		// Extract from URL path
		if rawURL != "" {
			u, err := url.Parse(rawURL)
			if err == nil && u.Path != "" && u.Path != "/" {
				parts := strings.Split(strings.Trim(u.Path, "/"), "/")
				if len(parts) > 0 && parts[len(parts)-1] != "" {
					title = parts[len(parts)-1]
				}
			}
		}
		if title == "" {
			title = "index"
		}
	}

	// Clean the filename
	cleaned := filenameRegex.ReplaceAllString(title, "")
	cleaned = spaceRegex.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, "-.")

	if cleaned == "" {
		cleaned = "untitled"
	}

	return cleaned + ".md"
}

// ExtractFirstSentence extracts the first meaningful sentence from content.
func ExtractFirstSentence(content string) string {
	if content == "" {
		return ""
	}

	// Remove markdown headers and clean up
	lines := strings.Split(content, "\n")
	var text strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Remove markdown formatting
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "*", "")
		line = strings.ReplaceAll(line, "`", "")

		if line != "" {
			text.WriteString(line)
			text.WriteString(" ")
		}
	}

	cleaned := strings.TrimSpace(text.String())
	if len(cleaned) == 0 {
		return ""
	}

	// Find first sentence ending
	for i, r := range cleaned {
		if r == '.' || r == '!' || r == '?' {
			// Make sure it's not just a decimal or abbreviation
			if i+1 < len(cleaned) && unicode.IsSpace(rune(cleaned[i+1])) {
				sentence := strings.TrimSpace(cleaned[:i+1])
				if len(sentence) > 20 { // Only return substantial sentences
					return sentence
				}
			}
		}
	}

	// If no sentence ending found, return first ~200 chars
	if len(cleaned) > 200 {
		words := strings.Fields(cleaned[:200])
		if len(words) > 1 {
			// Remove last word to avoid cutting mid-word
			return strings.Join(words[:len(words)-1], " ") + "..."
		}
	}

	return cleaned
}

// FormatDuration formats a duration into a human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// CreateOutputDirs creates all necessary output directories.
func CreateOutputDirs(outputDir string) error {
	dirs := []string{
		outputDir,
		filepath.Join(outputDir, "pages"),
	}

	for _, dir := range dirs {
		if err := EnsureDir(dir); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}
