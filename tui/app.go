package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/sysinfo"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type page int

const (
	pageDashboard page = iota
	pageRunning
	pageResults
	pageCompare
	pageSuiteConfig
	pageSuiteRunning
	pageSuiteResults
)

type menuItem struct {
	label  string
	desc   string
	mode   string
	engine string
}

var menuItems = []menuItem{
	{label: "Run Hardware Benchmark", desc: "Tool-defined single/multi CPU, memory, and disk tests", mode: "single", engine: "external"},
	{label: "Run Suite (VPS Composite)", desc: "ECS-style: hardware + net + IP + media", mode: "suite"},
	{label: "Compare Reports", desc: "Compare two JSON reports", mode: "compare"},
	{label: "System Info", desc: "Show system information", mode: "sysinfo"},
	{label: "Quit", desc: "", mode: "quit"},
}

type benchmarkStartMsg struct {
	mode   string
	engine string
}
type benchmarkEventMsg struct{ event vmbench.Event }
type benchmarkDoneMsg struct{ report vmbench.Report }
type sysinfoDoneMsg struct {
	info     sysinfo.SystemInfo
	warnings []string
}
type tickMsg time.Time

type workloadState struct {
	name     string
	category string
	status   string
	metric   string
	duration string
}

type Model struct {
	page        page
	cursor      int
	sysInfo     sysinfo.SystemInfo
	sysWarnings []string
	showSysInfo bool

	workloads []workloadState
	report    *vmbench.Report
	eventCh   chan vmbench.Event
	cancel    context.CancelFunc
	phase     string
	confirm   bool
	engine    string
	startedAt time.Time
	eventLog  []string
	showLog   bool
	spinner   spinner.Model

	suiteConfig   suiteConfigState
	suiteSections []suiteSection
	suiteEventCh  chan suite.Event
	suiteReport   *suite.SuiteReport

	compareA string
	compareB string

	resultsTab int
	resultsCur int
	expanded   map[string]bool

	toast comp.Toast

	width  int
	height int
}

func NewModel(compareA, compareB string) Model {
	return Model{
		page:        pageDashboard,
		compareA:    compareA,
		compareB:    compareB,
		expanded:    make(map[string]bool),
		spinner:     comp.NewSpinner(),
		suiteConfig: newSuiteConfigState(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadSysinfo(), tickEvery(), m.spinner.Tick)
}

func loadSysinfo() tea.Cmd {
	return func() tea.Msg {
		info, warnings := sysinfo.Collect(context.Background())
		return sysinfoDoneMsg{info: info, warnings: warnings}
	}
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tickEvery()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.confirm {
			return handleConfirm(m, msg)
		}
		switch m.page {
		case pageDashboard:
			return updateDashboard(m, msg)
		case pageRunning:
			return updateRunning(m, msg)
		case pageResults:
			return updateResults(m, msg)
		case pageCompare:
			return updateCompare(m, msg)
		case pageSuiteConfig:
			return updateSuiteConfig(m, msg)
		case pageSuiteRunning:
			return updateSuiteRunning(m, msg)
		case pageSuiteResults:
			return updateSuiteResults(m, msg)
		}

	case sysinfoDoneMsg:
		m.sysInfo = msg.info
		m.sysWarnings = msg.warnings
		return m, nil

	case benchmarkStartMsg:
		return startBenchmark(m, msg.mode, msg.engine)

	case benchmarkEventMsg:
		return updateWorkloadEvent(m, msg.event)

	case benchmarkDoneMsg:
		m.report = &msg.report
		m.page = pageResults
		return m, nil

	case suiteStartMsg:
		return startSuite(m, msg.opts)

	case suiteEventMsg:
		return updateSuiteEvent(m, msg.event)

	case suiteDoneMsg:
		m.suiteReport = &msg.report
		m.page = pageSuiteResults
		return m, nil

	case comp.ToastExpireMsg:
		if msg.Stamp == m.toast.Until {
			m.toast = comp.Toast{}
		}
		return m, nil

	case saveDoneMsg:
		var t comp.Toast
		var c tea.Cmd
		if msg.err != nil {
			t, c = comp.ShowToast("save failed: "+msg.err.Error(), comp.ToastError, 4*time.Second)
		} else {
			t, c = comp.ShowToast("saved → "+msg.path, comp.ToastSuccess, 3*time.Second)
		}
		m.toast = t
		return m, c
	}
	return m, nil
}

func (m Model) View() string {
	if m.width < 60 || m.height < 18 {
		return lipgloss.NewStyle().
			Foreground(theme.Active.Warning).
			Render(fmt.Sprintf("\n  Terminal too small (need ≥60x18, got %dx%d)\n  Resize or zoom out.", m.width, m.height))
	}

	header := renderHeader(m)
	footer := renderFooter(m)

	contentHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if contentHeight < 5 {
		contentHeight = 5
	}

	var content string
	switch m.page {
	case pageDashboard:
		content = viewDashboard(m)
	case pageRunning:
		content = viewRunning(m)
	case pageResults:
		content = viewResults(m)
	case pageCompare:
		content = viewCompare(m)
	case pageSuiteConfig:
		content = viewSuiteConfig(m)
	case pageSuiteRunning:
		content = viewSuiteRunning(m)
	case pageSuiteResults:
		content = viewSuiteResults(m)
	}

	bodyStyle := lipgloss.NewStyle().
		Foreground(theme.Active.Fg).
		Width(m.width).
		Padding(1, 0, 1, 2)
	body := bodyStyle.Render(content)

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return view
}

func renderHeader(m Model) string {
	cpu := m.sysInfo.CPU.Model
	if len(cpu) > 30 {
		cpu = cpu[:30] + "…"
	}
	if cpu == "" {
		cpu = "loading..."
	}
	return comp.Header(comp.HeaderInfo{
		Brand:     " VMBENCH ",
		Version:   "v" + vmbench.Version,
		CPU:       cpu,
		Cores:     fmt.Sprintf("(%dC/%dT)", m.sysInfo.CPU.PhysicalCores, m.sysInfo.CPU.LogicalCores),
		Now:       time.Now(),
		ThemeName: theme.Active.Name,
		Width:     m.width,
	})
}

func renderFooter(m Model) string {
	var hints []comp.Hint
	switch m.page {
	case pageDashboard:
		hints = []comp.Hint{
			{Key: "↑↓", Desc: "nav"},
			{Key: "↵", Desc: "select"},
			{Key: "t", Desc: "theme"},
			{Key: "q", Desc: "quit"},
		}
	case pageRunning:
		hints = []comp.Hint{
			{Key: "esc", Desc: "cancel"},
			{Key: "tab", Desc: "log"},
			{Key: "q", Desc: "quit"},
		}
	case pageResults:
		hints = []comp.Hint{
			{Key: "tab", Desc: "view"},
			{Key: "↑↓", Desc: "nav"},
			{Key: "↵", Desc: "expand"},
			{Key: "s", Desc: "save"},
			{Key: "esc", Desc: "back"},
		}
	case pageCompare:
		hints = []comp.Hint{
			{Key: "esc", Desc: "back"},
			{Key: "q", Desc: "quit"},
		}
	case pageSuiteConfig:
		hints = []comp.Hint{
			{Key: "↑↓", Desc: "field"},
			{Key: "←→", Desc: "choose"},
			{Key: "spc", Desc: "toggle"},
			{Key: "↵", Desc: "start"},
			{Key: "esc", Desc: "back"},
		}
	case pageSuiteRunning:
		hints = []comp.Hint{
			{Key: "esc", Desc: "cancel"},
			{Key: "tab", Desc: "log"},
			{Key: "q", Desc: "quit"},
		}
	case pageSuiteResults:
		hints = []comp.Hint{
			{Key: "esc", Desc: "back"},
			{Key: "q", Desc: "quit"},
		}
	}
	return comp.Footer(m.width, hints)
}
