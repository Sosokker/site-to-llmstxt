package progress

import (
	"fmt"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"

	"github.com/Sosokker/site-to-llmstxt/internal/models"
)

// Manager handles progress tracking and UI updates.
type Manager struct {
	bar     *progressbar.ProgressBar
	verbose bool
	stats   *models.Stats
}

// New creates a new progress manager.
func New(verbose bool, stats *models.Stats) *Manager {
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetDescription("Crawling"),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetWidth(50),
		progressbar.OptionThrottle(100*time.Millisecond),
	)

	return &Manager{
		bar:     bar,
		verbose: verbose,
		stats:   stats,
	}
}

// Update updates the progress bar with current status.
func (m *Manager) Update(processed, queued int) {
	if m.verbose {
		fmt.Printf("\rProgress: %d pages processed, %d in queue", processed, queued)
	}
	m.bar.Add(1)
}

// Finish completes the progress bar and shows final statistics.
func (m *Manager) Finish() {
	m.bar.Finish()
	m.showSummary()
}

// showSummary displays a comprehensive summary of the crawling session.
func (m *Manager) showSummary() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📊 CRAWLING SUMMARY")
	fmt.Println(strings.Repeat("=", 60))

	// Basic stats
	fmt.Printf("🔍 Total pages crawled: %d\n", m.stats.TotalPages)
	fmt.Printf("📚 Main documentation: %d pages\n", m.stats.MainDocPages)
	fmt.Printf("📝 Secondary content: %d pages\n", m.stats.SecondaryPages)

	// Performance stats
	if m.stats.Duration > 0 {
		pagesPerSecond := float64(m.stats.TotalPages) / m.stats.Duration.Seconds()
		fmt.Printf("⏱️  Duration: %v (%.1f pages/sec)\n",
			m.stats.Duration.Round(time.Second), pagesPerSecond)
	}

	// Error stats
	if m.stats.ErrorCount > 0 || m.stats.SkippedURLs > 0 {
		fmt.Printf("⚠️  Errors: %d, Skipped URLs: %d\n", m.stats.ErrorCount, m.stats.SkippedURLs)
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("✅ Crawling completed successfully!")
}

// Log outputs a message if verbose mode is enabled.
func (m *Manager) Log(format string, args ...interface{}) {
	if m.verbose {
		fmt.Printf(format+"\n", args...)
	}
}
