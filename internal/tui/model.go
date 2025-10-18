package tui

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Sosokker/site-to-llmstxt/internal/config"
	"github.com/Sosokker/site-to-llmstxt/internal/crawler"
	"github.com/Sosokker/site-to-llmstxt/internal/generator"
	"github.com/Sosokker/site-to-llmstxt/internal/models"
	"github.com/Sosokker/site-to-llmstxt/internal/utils"
)

type Options struct {
	OutputDir      string
	DefaultWorkers int
}

type appState int

const (
	stateInput appState = iota
	stateConfig
	stateDiscover
	stateRun
	stateSelect
	stateFinalize
	stateDone
	stateError
)

type runStage int

const (
	runStageInitial runStage = iota
	runStageManual
)

type configMode int

const (
	configModePreflight configMode = iota
	configModeSelection
)

type entryType int

const (
	entryGroup entryType = iota
	entryPage
)

type listEntry struct {
	Type        entryType
	Group       string
	Page        crawler.PageSummary
	DisplayName string
}

type discoveryResultMsg struct {
	pages []crawler.PageSummary
	stats *models.Stats
	err   error
}

type progressUpdateMsg struct {
	update crawler.ProgressUpdate
}

type logUpdateMsg struct {
	update crawler.LogUpdate
}

type scrapeResultMsg struct {
	stats *models.Stats
	pages []models.PageInfo
	err   error
}

type finalizeResultMsg struct {
	err error
}

// Model represents the interactive TUI state.
type Model struct {
	state          appState
	runStage       runStage
	configMode     configMode
	defaultWorkers int

	ctx    context.Context
	cancel context.CancelFunc

	baseURL     *url.URL
	outputDir   string
	verbose     bool
	discoverMsg string

	urlInput    textinput.Model
	workerInput textinput.Model
	outputInput textinput.Model

	spinner  spinner.Model
	progress progress.Model

	logViewport  viewport.Model
	listViewport viewport.Model

	width  int
	height int
	errMsg string

	pages      []crawler.PageSummary
	totalPages int
	groups     map[string][]crawler.PageSummary
	groupOrder []string
	expanded   map[string]bool
	selected   map[string]bool
	entries    []listEntry
	cursor     int

	progressCh chan crawler.ProgressUpdate
	logCh      chan crawler.LogUpdate
	logLines   []string

	scraped    map[string]models.PageInfo
	scrapedSeq []models.PageInfo
	lastStats  *models.Stats

	progressDone  int
	progressTotal int
	configFocus   int
}

// NewModel constructs a new Bubble Tea model.
func NewModel(ctx context.Context, opts Options) *Model {
	if opts.OutputDir == "" {
		opts.OutputDir = config.DefaultOutputDir
	}
	if opts.DefaultWorkers <= 0 {
		opts.DefaultWorkers = config.DefaultWorkers
	}

	c, cancel := context.WithCancel(ctx)

	urlInput := textinput.New()
	urlInput.Placeholder = "https://docs.example.com"
	urlInput.Focus()

	workerInput := textinput.New()
	workerInput.Placeholder = fmt.Sprintf("%d", opts.DefaultWorkers)
	workerInput.SetValue(fmt.Sprintf("%d", opts.DefaultWorkers))
	workerInput.CharLimit = 3

	outputInput := textinput.New()
	outputInput.Placeholder = opts.OutputDir
	outputInput.SetValue(opts.OutputDir)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	model := &Model{
		state:          stateInput,
		runStage:       runStageInitial,
		configMode:     configModePreflight,
		defaultWorkers: opts.DefaultWorkers,
		ctx:            c,
		cancel:         cancel,
		outputDir:      opts.OutputDir,
		urlInput:       urlInput,
		workerInput:    workerInput,
		outputInput:    outputInput,
		spinner:        sp,
		progress:       progress.New(progress.WithDefaultGradient()),
		logViewport:    viewport.New(0, 0),
		listViewport:   viewport.New(0, 0),
		groups:         make(map[string][]crawler.PageSummary),
		expanded:       make(map[string]bool),
		selected:       make(map[string]bool),
		scraped:        make(map[string]models.PageInfo),
	}
	model.width = 80
	model.height = 24
	model.recalculateLayout()
	return model
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles updates from Bubble Tea.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		m.recalculateLayout()
	case tea.KeyMsg:
		if typed.Type == tea.KeyCtrlC {
			m.cancel()
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateInput:
		return m.updateInput(msg)
	case stateConfig:
		return m.updateConfig(msg)
	case stateDiscover:
		return m.updateDiscover(msg)
	case stateRun:
		return m.updateRun(msg)
	case stateSelect:
		return m.updateSelect(msg)
	case stateFinalize:
		return m.updateFinalize(msg)
	case stateDone, stateError:
		return m.updateTerminal(msg)
	default:
		return m, nil
	}
}

func (m *Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		if typed.Type == tea.KeyEnter {
			raw := strings.TrimSpace(m.urlInput.Value())
			if raw == "" {
				m.errMsg = "URL is required"
				return m, nil
			}

			parsed, err := url.Parse(raw)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				m.errMsg = "Enter a valid URL with scheme and host"
				return m, nil
			}

			m.baseURL = parsed
			m.errMsg = ""
			m.configMode = configModePreflight
			m.workerInput.SetValue(fmt.Sprintf("%d", m.defaultWorkers))
			m.outputInput.SetValue(m.outputDir)
			m.state = stateConfig
			m.discoverMsg = "Preparing crawl..."
			return m, m.setConfigFocus(0)
		}
	}

	var cmd tea.Cmd
	m.urlInput, cmd = m.urlInput.Update(msg)
	return m, cmd
}

func (m *Model) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		switch typed.String() {
		case "enter":
			output := strings.TrimSpace(m.outputInput.Value())
			if output == "" {
				output = m.outputDir
			}
			if output == "" {
				m.errMsg = "Output directory cannot be empty"
				return m, nil
			}
			m.outputDir = output

			if m.configMode == configModePreflight {
				workers, err := parseWorkers(m.workerInput.Value(), m.defaultWorkers)
				if err != nil {
					m.errMsg = err.Error()
					return m, nil
				}
				m.defaultWorkers = workers
				m.errMsg = ""
				m.state = stateDiscover
				host := "site"
				if m.baseURL != nil && m.baseURL.Host != "" {
					host = m.baseURL.Host
				}
				m.discoverMsg = fmt.Sprintf("Crawling %s...", host)
				return m, tea.Batch(
					m.spinner.Tick,
					startDiscovery(m.ctx, m.baseURL, workers),
				)
			}

			m.errMsg = ""
			return m, m.generateOutputs()
		case "r":
			if m.configMode == configModeSelection {
				workers, err := parseWorkers(m.workerInput.Value(), m.defaultWorkers)
				if err != nil {
					m.errMsg = err.Error()
					return m, nil
				}
				m.defaultWorkers = workers
				m.errMsg = ""
				return m, m.beginScrape(m.pages, workers, runStageManual)
			}
		case "tab", "shift+tab", "down", "up":
			next := m.configFocus
			if typed.String() == "tab" || typed.String() == "down" {
				next = (next + 1) % 2
			} else {
				next = (next - 1 + 2) % 2
			}
			return m, m.setConfigFocus(next)
		case "v":
			m.verbose = !m.verbose
			return m, nil
		case "esc":
			if m.configMode == configModePreflight {
				m.state = stateInput
			} else {
				m.state = stateSelect
			}
			m.errMsg = ""
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.configFocus {
	case 0:
		m.workerInput, cmd = m.workerInput.Update(msg)
	case 1:
		m.outputInput, cmd = m.outputInput.Update(msg)
	default:
		cmd = nil
	}
	return m, cmd
}

func (m *Model) updateDiscover(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(typed)
		return m, cmd
	case discoveryResultMsg:
		if typed.err != nil {
			m.state = stateError
			m.errMsg = typed.err.Error()
			return m, nil
		}

		m.pages = typed.pages
		m.totalPages = len(typed.pages)
		m.buildGroups()
		m.rebuildEntries()
		return m, m.beginScrape(typed.pages, m.defaultWorkers, runStageInitial)
	}
	return m, nil
}

func (m *Model) updateRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case progressUpdateMsg:
		m.progressDone = typed.update.Completed
		m.progressTotal = typed.update.Total
		pct := 0.0
		if m.progressTotal > 0 {
			pct = float64(m.progressDone) / float64(m.progressTotal)
		}
		cmd := m.progress.SetPercent(pct)
		if m.progressDone < m.progressTotal {
			return m, tea.Batch(cmd, listenProgress(m.progressCh))
		}
		return m, cmd
	case logUpdateMsg:
		m.logLines = append(m.logLines, typed.update.Message)
		m.logViewport.SetContent(strings.Join(m.logLines, "\n"))
		if len(m.logLines) > 0 {
			m.logViewport.GotoBottom()
		}
		return m, listenLogs(m.logCh)
	case scrapeResultMsg:
		m.lastStats = typed.stats
		if typed.err != nil {
			m.state = stateError
			m.errMsg = typed.err.Error()
			return m, nil
		}

		if typed.pages != nil {
			m.scrapedSeq = typed.pages
			m.scraped = make(map[string]models.PageInfo, len(typed.pages))
			for _, info := range typed.pages {
				m.scraped[info.URL] = info
			}
			if m.runStage == runStageInitial && len(m.selected) == 0 {
				for _, info := range typed.pages {
					m.selected[info.URL] = true
				}
			}
			if m.runStage == runStageManual {
				for url := range m.selected {
					if _, ok := m.scraped[url]; !ok {
						delete(m.selected, url)
					}
				}
				for _, info := range typed.pages {
					m.selected[info.URL] = true
				}
			}
		}

		m.state = stateSelect
		m.configMode = configModeSelection
		m.cursor = 0
		m.logLines = nil
		m.logViewport.SetContent("")
		m.listViewport.SetYOffset(0)
		m.runStage = runStageInitial
		return m, nil
	}
	return m, nil
}

func (m *Model) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		switch typed.String() {
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.ensureListCursorVisible()
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureListCursorVisible()
			}
		case "right", "l":
			m.toggleExpand(true)
			m.ensureListCursorVisible()
		case "left", "h":
			m.toggleExpand(false)
			m.ensureListCursorVisible()
		case "tab":
			m.jumpGroup(true)
			m.ensureListCursorVisible()
		case "shift+tab":
			m.jumpGroup(false)
			m.ensureListCursorVisible()
		case " ":
			m.toggleSelection()
			m.ensureListCursorVisible()
		case "enter":
			if len(m.selected) == 0 {
				m.errMsg = "Select at least one page"
				return m, nil
			}
			m.errMsg = ""
			return m, m.generateOutputs()
		case "c":
			m.workerInput.SetValue(fmt.Sprintf("%d", m.defaultWorkers))
			m.outputInput.SetValue(m.outputDir)
			m.configMode = configModeSelection
			m.errMsg = ""
			m.state = stateConfig
			return m, m.setConfigFocus(0)
		case "v":
			m.verbose = !m.verbose
			return m, nil
		case "esc":
			m.state = stateInput
			m.errMsg = ""
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) updateFinalize(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(typed)
		return m, cmd
	case finalizeResultMsg:
		if typed.err != nil {
			m.state = stateError
			m.errMsg = typed.err.Error()
			return m, nil
		}
		m.state = stateDone
		return m, nil
	}
	return m, nil
}

func (m *Model) updateTerminal(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		return m, tea.Quit
	}
	return m, nil
}

// View renders the active state.
func (m *Model) View() string {
	switch m.state {
	case stateInput:
		return m.viewInput()
	case stateConfig:
		return m.viewConfig()
	case stateDiscover:
		return m.viewDiscover()
	case stateRun:
		return m.viewRun()
	case stateSelect:
		return m.viewSelect()
	case stateFinalize:
		return m.viewFinalize()
	case stateDone:
		return m.viewDone()
	case stateError:
		return m.viewError()
	default:
		return ""
	}
}

func (m *Model) viewInput() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Site to LLMs.txt – TUI"))
	b.WriteString("\n\n")
	b.WriteString(instructionStyle.Render("Enter the base documentation URL:"))
	b.WriteString("\n\n")
	b.WriteString(m.urlInput.View())
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("Press Enter to configure crawl settings, Ctrl+C to quit."))
	if m.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.errMsg))
	}
	return panelStyle.Render(b.String())
}

func (m *Model) viewConfig() string {
	var b strings.Builder
	if m.configMode == configModePreflight {
		b.WriteString(titleStyle.Render("Configure Crawl"))
	} else {
		b.WriteString(titleStyle.Render("Export Settings"))
	}
	b.WriteString("\n")

	b.WriteString(infoStyle.Render(fmt.Sprintf("Workers: %s", m.workerInput.View())))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf("Output directory: %s", m.outputInput.View())))
	b.WriteString("\n")
	if m.verbose {
		b.WriteString(infoStyle.Render("Verbose logging: on (toggle with 'v')"))
	} else {
		b.WriteString(infoStyle.Render("Verbose logging: off (toggle with 'v')"))
	}
	b.WriteString("\n\n")

	if m.configMode == configModePreflight {
		b.WriteString(instructionStyle.Render("Enter to start crawling · Tab to switch fields · Esc to go back"))
	} else {
		b.WriteString(instructionStyle.Render("Enter to generate outputs · 'r' to rescrape · Esc to return"))
	}

	if m.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.errMsg))
	}

	return panelStyle.Render(b.String())
}

func (m *Model) viewDiscover() string {
	content := fmt.Sprintf("%s %s\n\n%s",
		m.spinner.View(),
		instructionStyle.Render(m.discoverMsg),
		hintStyle.Render("Press Ctrl+C to cancel."),
	)
	return panelStyle.Render(content)
}

func (m *Model) viewRun() string {
	title := "Scraping pages..."
	if m.runStage == runStageManual {
		title = "Rescraping pages..."
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		accentStyle.Render(fmt.Sprintf("%s (%d pages)", title, m.progressTotal)),
		m.progress.View(),
		"",
		accentStyle.Render("Logs:"),
		m.logViewport.View(),
	)
	return panelStyle.Render(content)
}

func (m *Model) viewSelect() string {
	listWidth := max(m.width-6, 20)
	listHeight := max(m.height-8, 6)
	m.listViewport.Width = listWidth
	m.listViewport.Height = listHeight

	var list strings.Builder
	for i, entry := range m.entries {
		cursor := "  "
		if i == m.cursor {
			cursor = accentStyle.Render("> ")
		}
		switch entry.Type {
		case entryGroup:
			selectedCount := m.countSelectedInGroup(entry.Group)
			total := len(m.groups[entry.Group])
			symbol := plusStyle.Render("+")
			if m.expanded[entry.Group] {
				symbol = plusStyle.Render("−")
			}
			check := checkboxEmptyStyle.Render("[ ]")
			if selectedCount == total && total > 0 {
				check = checkboxFilledStyle.Render("[x]")
			} else if selectedCount > 0 {
				check = checkboxMixedStyle.Render("[•]")
			}
			display := entry.DisplayName
			if display == "" {
				display = "(root)"
			}
			line := fmt.Sprintf("%s%s %s %s (%d/%d)", cursor, symbol, check, groupStyle.Render(display), selectedCount, total)
			if i == m.cursor {
				line = cursorStyle.Render(line)
			}
			list.WriteString(line)
		case entryPage:
			check := checkboxEmptyStyle.Render("[ ]")
			if m.selected[entry.Page.URL] {
				check = checkboxFilledStyle.Render("[x]")
			}
			line := fmt.Sprintf("%s   %s %s", cursor, check, pageStyle.Render(entry.DisplayName))
			if i == m.cursor {
				line = cursorStyle.Render(line)
			}
			list.WriteString(line)
		}
		if i < len(m.entries)-1 {
			list.WriteString("\n")
		}
	}

	m.listViewport.SetContent(list.String())
	m.ensureListCursorVisible()

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Discovered %d pages", m.totalPages)))
	b.WriteString("\n")
	b.WriteString(instructionStyle.Render("↑/↓ move · Space toggle · Tab jump groups · Enter export · 'c' configure"))
	b.WriteString("\n\n")
	b.WriteString(panelStyle.Width(listWidth).Render(m.listViewport.View()))
	b.WriteString("\n\n")
	if m.errMsg != "" {
		b.WriteString(statusStyle.Render(m.errMsg))
		b.WriteString("\n\n")
	}
	b.WriteString(accentStyle.Render(fmt.Sprintf("Selected: %d pages", len(m.selected))))
	return b.String()
}

func (m *Model) viewFinalize() string {
	content := fmt.Sprintf("%s %s", m.spinner.View(), instructionStyle.Render("Generating llms outputs..."))
	return panelStyle.Render(content)
}

func (m *Model) viewDone() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("✅ Scrape completed!"))
	b.WriteString("\n")
	if m.lastStats != nil {
		b.WriteString(infoStyle.Render(fmt.Sprintf("Pages processed: %d (main: %d, secondary: %d)", m.lastStats.TotalPages, m.lastStats.MainDocPages, m.lastStats.SecondaryPages)))
		b.WriteString("\n")
		b.WriteString(infoStyle.Render(fmt.Sprintf("Errors: %d, Skipped: %d", m.lastStats.ErrorCount, m.lastStats.SkippedURLs)))
		b.WriteString("\n")
		b.WriteString(infoStyle.Render(fmt.Sprintf("Duration: %s", m.lastStats.Duration.Round(time.Second))))
		b.WriteString("\n")
	}
	b.WriteString(infoStyle.Render(fmt.Sprintf("Pages selected for llms.txt: %d", len(m.selected))))
	b.WriteString("\n\n")
	b.WriteString(accentStyle.Render(fmt.Sprintf("Output written to %s", m.outputDir)))
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("Press Enter to exit."))
	return panelStyle.Render(b.String())
}

func (m *Model) viewError() string {
	content := fmt.Sprintf("❌ %s\n\n%s", statusStyle.Render(m.errMsg), hintStyle.Render("Press Enter to exit."))
	return panelStyle.Render(content)
}

func (m *Model) buildGroups() {
	m.groups = make(map[string][]crawler.PageSummary)
	for _, page := range m.pages {
		group := groupForPath(page.Path)
		m.groups[group] = append(m.groups[group], page)
	}
	m.groupOrder = m.groupOrder[:0]
	for g := range m.groups {
		m.groupOrder = append(m.groupOrder, g)
	}
	sort.Strings(m.groupOrder)
}

func (m *Model) rebuildEntries() {
	m.entries = m.entries[:0]
	for _, group := range m.groupOrder {
		display := group
		if display == "" {
			display = "(root)"
		}
		m.entries = append(m.entries, listEntry{
			Type:        entryGroup,
			Group:       group,
			DisplayName: display,
		})
		if m.expanded[group] {
			groupPages := m.groups[group]
			sort.Slice(groupPages, func(i, j int) bool {
				return groupPages[i].URL < groupPages[j].URL
			})
			for _, page := range groupPages {
				title := page.Title
				if title == "" {
					title = page.URL
				}
				m.entries = append(m.entries, listEntry{
					Type:        entryPage,
					Group:       group,
					Page:        page,
					DisplayName: fmt.Sprintf("%s — %s", title, page.URL),
				})
			}
		}
	}
	if len(m.entries) == 0 {
		m.entries = append(m.entries, listEntry{
			Type:        entryGroup,
			Group:       "",
			DisplayName: "No pages discovered",
		})
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) toggleExpand(expand bool) {
	if len(m.entries) == 0 {
		return
	}
	entry := m.entries[m.cursor]
	if entry.Type != entryGroup {
		if !expand {
			m.jumpToGroup(entry.Group)
		}
		return
	}
	m.expanded[entry.Group] = expand
	m.rebuildEntries()
}

func (m *Model) jumpGroup(next bool) {
	if len(m.entries) == 0 {
		return
	}
	start := m.cursor
	for {
		if next {
			m.cursor++
			if m.cursor >= len(m.entries) {
				m.cursor = 0
			}
		} else {
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.entries) - 1
			}
		}
		if m.entries[m.cursor].Type == entryGroup {
			break
		}
		if m.cursor == start {
			break
		}
	}
}

func (m *Model) jumpToGroup(group string) {
	for i, entry := range m.entries {
		if entry.Type == entryGroup && entry.Group == group {
			m.cursor = i
			return
		}
	}
}

func (m *Model) toggleSelection() {
	if len(m.entries) == 0 {
		return
	}
	entry := m.entries[m.cursor]
	switch entry.Type {
	case entryGroup:
		groupPages := m.groups[entry.Group]
		allSelected := true
		for _, page := range groupPages {
			if !m.selected[page.URL] {
				allSelected = false
				break
			}
		}
		for _, page := range groupPages {
			if allSelected {
				delete(m.selected, page.URL)
			} else {
				m.selected[page.URL] = true
			}
		}
	case entryPage:
		if entry.Page.URL == "" {
			return
		}
		if m.selected[entry.Page.URL] {
			delete(m.selected, entry.Page.URL)
		} else {
			m.selected[entry.Page.URL] = true
		}
	}
}

func (m *Model) countSelectedInGroup(group string) int {
	count := 0
	for _, page := range m.groups[group] {
		if m.selected[page.URL] {
			count++
		}
	}
	return count
}

func (m *Model) selectedPageInfos() []models.PageInfo {
	selected := make([]models.PageInfo, 0, len(m.selected))
	for _, info := range m.scrapedSeq {
		if m.selected[info.URL] {
			selected = append(selected, info)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].URL < selected[j].URL
	})
	return selected
}

func (m *Model) beginScrape(pages []crawler.PageSummary, workers int, stage runStage) tea.Cmd {
	if len(pages) == 0 {
		m.errMsg = "Nothing to scrape"
		return nil
	}
	m.runStage = stage
	m.state = stateRun
	m.errMsg = ""
	m.progressDone = 0
	m.progressTotal = len(pages)
	m.logLines = nil
	m.logViewport.SetContent("")
	m.listViewport.SetYOffset(0)
	m.progress = progress.New(progress.WithDefaultGradient())
	m.progressCh = make(chan crawler.ProgressUpdate)
	m.logCh = make(chan crawler.LogUpdate, 32)
	return tea.Batch(
		startScrape(m.ctx, m.baseURL, m.outputDir, workers, m.verbose, pages, m.progressCh, m.logCh),
		listenProgress(m.progressCh),
		listenLogs(m.logCh),
	)
}

func (m *Model) generateOutputs() tea.Cmd {
	selected := m.selectedPageInfos()
	if len(selected) == 0 {
		m.errMsg = "Select at least one page"
		return nil
	}
	if m.baseURL == nil {
		m.errMsg = "Base URL missing"
		return nil
	}
	m.errMsg = ""
	m.state = stateFinalize
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			if err := utils.CreateOutputDirs(m.outputDir); err != nil {
				return finalizeResultMsg{err: err}
			}
			gen := generator.New(m.baseURL, m.outputDir)
			err := gen.Generate(selected)
			return finalizeResultMsg{err: err}
		},
	)
}

func (m *Model) ensureListCursorVisible() {
	if len(m.entries) == 0 {
		m.listViewport.SetYOffset(0)
		return
	}
	line := m.cursor
	start := m.listViewport.YOffset
	end := start + m.listViewport.Height
	if line < start {
		m.listViewport.SetYOffset(line)
	} else if line >= end {
		m.listViewport.SetYOffset(line - m.listViewport.Height + 1)
	}
}

func (m *Model) recalculateLayout() {
	listWidth := max(m.width-6, 20)
	listHeight := max(m.height-8, 6)
	m.listViewport.Width = listWidth
	m.listViewport.Height = listHeight

	logWidth := max(m.width-6, 20)
	logHeight := max(m.height/3, 6)
	m.logViewport.Width = logWidth
	m.logViewport.Height = logHeight
}

func parseWorkers(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	var workers int
	if _, err := fmt.Sscanf(raw, "%d", &workers); err != nil || workers <= 0 {
		return 0, fmt.Errorf("workers must be a positive number")
	}
	return workers, nil
}

func groupForPath(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[0]
}

func startDiscovery(ctx context.Context, baseURL *url.URL, workers int) tea.Cmd {
	return func() tea.Msg {
		pages, stats, err := crawler.Discover(ctx, crawler.DiscoverOptions{
			BaseURL: baseURL,
			Workers: workers,
		})
		return discoveryResultMsg{
			pages: pages,
			stats: stats,
			err:   err,
		}
	}
}

func listenProgress(ch <-chan crawler.ProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return progressUpdateMsg{update: update}
	}
}

func listenLogs(ch <-chan crawler.LogUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return logUpdateMsg{update: update}
	}
}

func (m *Model) setConfigFocus(index int) tea.Cmd {
	if index < 0 {
		index = 0
	}
	if index > 1 {
		index = 1
	}
	inputs := []*textinput.Model{&m.workerInput, &m.outputInput}
	cmds := make([]tea.Cmd, 0, len(inputs))
	for i, input := range inputs {
		if i == index {
			cmds = append(cmds, (*input).Focus())
		} else {
			(*input).Blur()
		}
	}
	m.configFocus = index
	return tea.Batch(cmds...)
}

func startScrape(ctx context.Context, baseURL *url.URL, outputDir string, workers int, verbose bool, pages []crawler.PageSummary, progressCh chan crawler.ProgressUpdate, logCh chan crawler.LogUpdate) tea.Cmd {
	return func() tea.Msg {
		resultPages, stats, err := crawler.Scrape(ctx, crawler.ScrapeOptions{
			BaseURL:  baseURL,
			Pages:    pages,
			Output:   outputDir,
			Workers:  workers,
			Verbose:  verbose,
			Logs:     logCh,
			Progress: progressCh,
		})
		close(progressCh)
		close(logCh)

		return scrapeResultMsg{
			stats: stats,
			pages: resultPages,
			err:   err,
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	cursorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	statusStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	titleStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	instructionStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	hintStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	accentStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	infoStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("249"))
	groupStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	pageStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("249"))
	checkboxEmptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	checkboxFilledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("120")).Bold(true)
	checkboxMixedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	plusStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
	panelStyle          = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).MaxWidth(96)
)

// Run launches the TUI program.
func Run(ctx context.Context, opts Options) error {
	model := NewModel(ctx, opts)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	_, err := prog.Run()
	return err
}
