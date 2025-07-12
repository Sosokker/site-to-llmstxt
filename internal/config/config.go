package config

import (
	"fmt"
	"net/url"
)

const (
	DefaultWorkers   = 1
	DefaultOutputDir = "./output"
	MarkdownSubdir   = "pages"
)

// Config holds crawler configuration.
type Config struct {
	URL       string
	OutputDir string
	Workers   int
	Verbose   bool
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("URL is required")
	}

	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must have http or https scheme")
	}

	if c.Workers <= 0 {
		return fmt.Errorf("workers must be greater than 0")
	}

	return nil
}
