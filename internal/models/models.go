package models

import "time"

// PageInfo represents information about a crawled page.
type PageInfo struct {
	URL         string
	Title       string
	Content     string
	FilePath    string
	CrawledAt   time.Time
	Description string
}

// Stats holds crawling statistics.
type Stats struct {
	TotalPages     int
	MainDocPages   int
	SecondaryPages int
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	ErrorCount     int
	SkippedURLs    int
}

// AddError increments the error count.
func (s *Stats) AddError() {
	s.ErrorCount++
}

// AddSkipped increments the skipped URL count.
func (s *Stats) AddSkipped() {
	s.SkippedURLs++
}

// Finish sets the end time and calculates duration.
func (s *Stats) Finish() {
	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
}
