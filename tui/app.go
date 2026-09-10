package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
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
	pageConfig
	pageCheckupResults
	pageHelp
	pageComparePicker
	pageResultDetail
	pageHistory
)

type menuItem struct {
	label string
	desc  string
	mode  string
}

// menuItems is a function, not a package var: package vars initialize
// before main() runs, before the active language is selected.
func menuItems() []menuItem {
	items := []menuItem{
		{label: i18n.T("tui.menu.benchmark"), desc: i18n.T("tui.menu.benchmarkDesc"), mode: "bench"},
	}
	if vmbench.FeatureCompare {
		items = append(items, menuItem{label: i18n.T("tui.menu.compare"), desc: i18n.T("tui.menu.compareDesc"), mode: "compare"})
	}
	items = append(items, menuItem{label: i18n.T("tui.menu.history"), desc: i18n.T("tui.menu.historyDesc"), mode: "history"})
	return append(items,
		menuItem{label: i18n.T("tui.menu.sysinfo"), desc: i18n.T("tui.menu.sysinfoDesc"), mode: "sysinfo"},
		menuItem{label: i18n.T("tui.menu.quit"), desc: "", mode: "quit"},
	)
}

type benchmarkEventMsg struct{ event vmbench.Event }
type benchmarkDoneMsg struct{ report vmbench.Report }
type sysinfoDoneMsg struct {
	info     sysinfo.SystemInfo
	warnings []string
}
type netIdentityDoneMsg struct {
	v4, v6 *netio.PublicIPIdentity
}
type tickMsg time.Time

// netIdentState is the dashboard's public IP/ASN hint, fetched once at
// startup. Offline leaves both identities nil and the rows stay hidden
// (offline must not nag).
type netIdentState struct {
	loading bool
	v4, v6  *netio.PublicIPIdentity
}

type workloadState struct {
	name      string
	category  string
	status    string
	metric    string
	duration  string
	startedAt time.Time // set on EventCheckupStart; wall clock for elapsed/ETA
	iterCur   int       // completed iteration samples of the current workload
	iterTotal int       // grown as iterations are observed
}

type Model struct {
	page        page
	cursor      int
	sysInfo     sysinfo.SystemInfo
	sysWarnings []string
	showSysInfo bool
	netIdent    netIdentState

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

	// ETA bookkeeping: wall-clock durations of finished workloads and the
	// run-wide sample counters from EventCheckupProgress.
	workloadDoneAt  []time.Duration
	runSamplesDone  int
	runSamplesTotal int

	// config is the single benchmark configuration page; runKind records
	// which report kind a started benchmark produces ("run" or "checkup").
	config  configState
	runKind string

	checkupSections []checkupSection
	checkupEventCh  chan checkup.Event
	checkupReport   *checkup.CheckupReport

	historyStats historyStats

	compareA string
	compareB string

	compareDocs        []gbreport.Document
	compareErr         error
	compareLoading     bool
	compareKind        string
	checkupCompareText string

	picker  pickerState
	history historyState

	// reportFrom records the page that opened the currently viewed report
	// (compare picker or history page); esc on the report pages returns
	// there. pageDashboard means the report came from a fresh run.
	reportFrom page

	resultsTab    int
	resultsCur    int
	resultsDetail int
	expanded      map[string]bool

	toast comp.Toast

	scroll scrollState

	helpFrom page

	width  int
	height int
}

func NewModel(compareA, compareB string) Model {
	m := Model{
		page:     pageDashboard,
		compareA: compareA,
		compareB: compareB,
		expanded: make(map[string]bool),
		spinner:  comp.NewSpinner(),
		config:   newConfigState(),
		picker:   newPickerState(compareA, compareB),
		netIdent: netIdentState{loading: true},
	}
	// Flag users passing both reports land on the comparison directly.
	// Guarded so library callers can't reach the hidden page either.
	if vmbench.FeatureCompare && compareA != "" && compareB != "" {
		m.page = pageCompare
		m.compareLoading = true
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{loadSysinfo(), loadNetIdentity(), tickEvery(), m.spinner.Tick}
	if m.page == pageCompare && m.compareA != "" && m.compareB != "" {
		cmds = append(cmds, loadCompareCmd(m.compareA, m.compareB))
	}
	return tea.Batch(cmds...)
}

func loadSysinfo() tea.Cmd {
	return func() tea.Msg {
		info, warnings := sysinfo.Collect(context.Background())
		return sysinfoDoneMsg{info: info, warnings: warnings}
	}
}

func loadNetIdentity() tea.Cmd {
	return func() tea.Msg {
		// The 20s budget bounds the worst case (ipify then ipwho.is, 8s each
		// per family, families in parallel) so the placeholder never lingers.
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		result, _ := netio.ProbePublicIdentityLite(ctx, "dual")
		if result == nil {
			return netIdentityDoneMsg{}
		}
		return netIdentityDoneMsg{v4: result.PublicIPv4, v6: result.PublicIPv6}
	}
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	// syncScrollPage after the fact so page transitions performed inside
	// update reset the scroll offset immediately.
	if nm, ok := next.(Model); ok {
		next = syncScrollPage(nm)
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scroll.offset = 0
		return m, nil

	case tickMsg:
		return m, tickEvery()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.MouseMsg:
		return updateMouse(m, msg)

	case tea.KeyMsg:
		// ctrl+c: raw mode clears ISIG, so ^C arrives here as a plain key
		// instead of SIGINT. No page binds it, so quit globally — like "q"
		// on the running page, cancel any in-flight run first.
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if m.confirm {
			return handleConfirm(m, msg)
		}
		if msg.String() == "?" && !textEntryActive(m) {
			toggled, _ := toggleHelp(m)
			return toggled, nil
		}
		if scrolled, ok := handleScrollKeys(m, msg); ok {
			return scrolled, nil
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
		case pageConfig:
			return updateConfig(m, msg)
		case pageCheckupResults:
			return updateCheckupResults(m, msg)
		case pageHelp:
			return updateHelp(m, msg)
		case pageComparePicker:
			return updateComparePicker(m, msg)
		case pageResultDetail:
			return updateResultDetail(m, msg)
		case pageHistory:
			return updateHistory(m, msg)
		}

	case sysinfoDoneMsg:
		m.sysInfo = msg.info
		m.sysWarnings = msg.warnings
		return m, nil

	case netIdentityDoneMsg:
		m.netIdent.loading = false
		m.netIdent.v4, m.netIdent.v6 = msg.v4, msg.v6
		return m, nil

	case hardwareStartMsg:
		return startBenchmark(m, msg.opts)

	case historyStatsMsg:
		m.historyStats = msg.stats
		return m, nil

	case benchmarkEventMsg:
		return updateWorkloadEvent(m, msg.event)

	case benchmarkDoneMsg:
		m.report = &msg.report
		m.page = pageResults
		return m, nil

	case checkupStartMsg:
		return startCheckup(m, msg.opts)

	case checkupEventMsg:
		return updateCheckupEvent(m, msg.event)

	case checkupDoneMsg:
		m.checkupReport = &msg.report
		m.page = pageCheckupResults
		return m, nil

	case compareLoadedMsg:
		m.compareLoading = false
		if msg.err != nil {
			m.compareErr = msg.err
			m.compareDocs = nil
		} else {
			m.compareErr = nil
			m.compareDocs = msg.docs
			m.page = pageCompare
		}
		return m, nil

	case historyListMsg:
		// loadHistoryCmd serves both the compare picker and the history
		// page; route the result to whichever one is waiting.
		if m.page == pageHistory {
			m.history.loading = false
			if msg.err != nil {
				m.history.err = msg.err
				m.history.records = nil
			} else {
				m.history.err = nil
				m.history.records = msg.records
				if m.history.cursor >= len(msg.records) {
					m.history.cursor = len(msg.records) - 1
				}
				if m.history.cursor < 0 {
					m.history.cursor = 0
				}
			}
			return followFocus(m), nil
		}
		m.picker.loading = false
		if msg.err != nil {
			m.picker.err = msg.err
			m.picker.records = nil
		} else {
			m.picker.err = nil
			m.picker.records = msg.records
			if m.picker.a >= len(msg.records) {
				m.picker.a = -1
			}
			if m.picker.b >= len(msg.records) {
				m.picker.b = -1
			}
		}
		return followFocus(m), nil

	case checkupCompareMsg:
		m.compareLoading = false
		if msg.err != nil {
			m.compareErr = msg.err
			m.checkupCompareText = ""
			m.page = pageComparePicker
		} else {
			m.compareErr = nil
			m.checkupCompareText = msg.text
			m.page = pageCompare
		}
		return m, nil

	case recordViewMsg:
		if msg.err != nil {
			var cmd tea.Cmd
			m.toast, cmd = comp.ShowToast(msg.err.Error(), comp.ToastError, 4*time.Second)
			return m, cmd
		}
		if msg.kind == history.KindCheckup && msg.checkup != nil {
			m.checkupReport = msg.checkup
			m.reportFrom = m.page
			m.page = pageCheckupResults
			return m, nil
		}
		if msg.run != nil {
			doc := *msg.run
			m.report = &doc
			m.reportFrom = m.page
			m.resultsCur = 0
			m.page = pageResults
		}
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
			t, c = comp.ShowToast(i18n.Tf("tui.toast.saveFailed", map[string]any{"Err": msg.err.Error()}), comp.ToastError, 4*time.Second)
		} else {
			t, c = comp.ShowToast(i18n.Tf("tui.toast.saved", map[string]any{"Path": msg.path}), comp.ToastSuccess, 3*time.Second)
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
			Render(fmt.Sprintf("\n  %s\n  %s", i18n.Tf("tui.tooSmall", map[string]any{"Width": m.width, "Height": m.height}), i18n.T("tui.tooSmallHint")))
	}

	header := renderHeader(m)
	// The body is clipped to the viewport so tall pages (checkup results,
	// compare tables) scroll instead of overflowing the alt screen.
	content, pos := viewScrollPos(m)

	bodyStyle := lipgloss.NewStyle().
		Foreground(theme.Active.Fg).
		Width(m.width).
		Padding(1, 0, 1, 2)
	body := bodyStyle.Render(content)

	footer := renderFooter(m, pos)

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return view
}

func renderHeader(m Model) string {
	cpu := m.sysInfo.CPU.Model
	cpu = i18n.TruncateCells(cpu, 30)
	if cpu == "" {
		cpu = i18n.T("tui.header.loading")
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

func renderFooter(m Model, pos scrollPos) string {
	hints := helpFooterHints(m.page)
	if pos.scrollable {
		scrollHint := comp.Hint{
			Key:  fmt.Sprintf("%d/%d", pos.line, pos.total),
			Desc: i18n.T("tui.hint.scroll"),
		}
		// The position hint is dropped first when space runs out.
		footer := comp.Footer(m.width, append(hints, scrollHint))
		if line := strings.Split(footer, "\n")[1]; i18n.StyledWidth(line) <= m.width {
			return footer
		}
	}
	return comp.Footer(m.width, hints)
}
