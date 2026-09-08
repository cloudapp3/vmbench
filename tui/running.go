package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func startBenchmark(m Model, opts vmbench.Options) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.eventCh = make(chan vmbench.Event, 100)
	m.phase = "Hardware"
	m.confirm = false
	m.startedAt = time.Now()
	m.eventLog = m.eventLog[:0]
	m.workloadDoneAt = nil
	m.runSamplesDone = 0
	m.runSamplesTotal = 0
	// Land on the running page: without this the hardware path stayed on the
	// config page until the run finished, making the progress view and the
	// cancel modal unreachable.
	m.page = pageRunning
	m.runKind = "run"
	m.engine = "external"

	// Prefill from the exact definition set the runner will execute (tools
	// + filter), so the running page never shows waiting ghost rows for
	// adapters that are not part of the configured run.
	defs := catalog.ExternalHardwareDefinitionsForTools("", opts.HardwareTools)
	if expr := strings.TrimSpace(opts.Filter); expr != "" {
		if re, err := regexp.Compile(expr); err == nil {
			filtered := defs[:0]
			for _, d := range defs {
				if re.MatchString(d.Name) || re.MatchString(d.Category) {
					filtered = append(filtered, d)
				}
			}
			defs = filtered
		}
	}
	m.workloads = make([]workloadState, 0, len(defs))
	for _, d := range defs {
		m.workloads = append(m.workloads, workloadState{
			name:     d.Name,
			category: d.Category,
			status:   "waiting",
		})
	}

	if m.spinner.Spinner.Frames == nil {
		m.spinner = comp.NewSpinner()
	}

	return m, tea.Batch(
		runBenchmarkCmd(ctx, opts, m.eventCh),
		waitForEvent(m.eventCh),
		m.spinner.Tick,
	)
}

func runBenchmarkCmd(ctx context.Context, opts vmbench.Options, ch chan<- vmbench.Event) tea.Cmd {
	return func() tea.Msg {
		opts.OnEvent = func(ev vmbench.Event) {
			ch <- ev
		}
		report := vmbench.RunCore(ctx, opts)
		return benchmarkDoneMsg{report: report}
	}
}

func waitForEvent(ch <-chan vmbench.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return benchmarkEventMsg{event: ev}
	}
}

func updateWorkloadEvent(m Model, ev vmbench.Event) (tea.Model, tea.Cmd) {
	addLog := func(msg string) {
		m.eventLog = append(m.eventLog, fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg))
		if len(m.eventLog) > 50 {
			m.eventLog = m.eventLog[len(m.eventLog)-50:]
		}
	}

	switch ev.Kind {
	case vmbench.EventSuiteStart:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
				m.workloads[i].status = "running"
				m.workloads[i].startedAt = time.Now()
				m.workloads[i].iterCur = 0
				m.workloads[i].iterTotal = 0
				break
			}
		}
		if strings.Contains(ev.Message, "multi-core") {
			m.phase = "Multi-Core"
		}
		addLog("▸ start  " + ev.Workload)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteProgress:
		// Run-wide sample counters plus the current workload's iteration
		// mini progress (ev.Total is samples across the whole run).
		m.runSamplesDone = ev.Current
		m.runSamplesTotal = ev.Total
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload && m.workloads[i].status == "running" {
				m.workloads[i].iterCur = ev.Iteration
				if ev.Iteration > m.workloads[i].iterTotal {
					m.workloads[i].iterTotal = ev.Iteration
				}
				break
			}
		}
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteDone:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
				m.recordWorkloadDone(i)
				m.workloads[i].status = "done"
				m.workloads[i].metric = ev.Metric
				m.workloads[i].duration = ev.Duration.String()
				break
			}
		}
		addLog("✓ " + i18n.PadCells(i18n.StatusLabel("done"), 7) + " " + ev.Workload + "  " + ev.Metric)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteFail:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
				m.recordWorkloadDone(i)
				m.workloads[i].status = "fail"
				if ev.Err != nil {
					m.workloads[i].metric = ev.Err.Error()
				}
				break
			}
		}
		errMsg := ""
		if ev.Err != nil {
			errMsg = ev.Err.Error()
		}
		addLog("✗ " + i18n.PadCells(i18n.StatusLabel("fail"), 7) + " " + ev.Workload + "  " + errMsg)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteSkip:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
				m.workloads[i].status = "skip"
				break
			}
		}
		addLog("⊘ " + i18n.PadCells(i18n.StatusLabel("skip"), 7) + " " + ev.Workload)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventBenchDone:
		addLog("● " + i18n.T("cli.progress.benchmarkComplete"))
		return m, waitForEvent(m.eventCh)
	}
	return m, waitForEvent(m.eventCh)
}

func updateRunning(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if !m.confirm {
			m.confirm = true
			return m, nil
		}
		return m, nil
	case "tab":
		m.showLog = !m.showLog
		return m, nil
	case "q":
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	return m, nil
}

// recordWorkloadDone banks the wall-clock duration of a finished workload
// for the ETA extrapolation (ev.Duration is the median sample time, not
// wall time, so it cannot be used here).
func (m *Model) recordWorkloadDone(i int) {
	if i < 0 || i >= len(m.workloads) {
		return
	}
	w := &m.workloads[i]
	if !w.startedAt.IsZero() {
		m.workloadDoneAt = append(m.workloadDoneAt, time.Since(w.startedAt).Truncate(time.Second))
	}
	w.startedAt = time.Time{}
}

// runETA extrapolates remaining time from the average wall-clock duration of
// finished workloads. Workloads run strictly serially, so average ×
// remaining is a reasonable estimate; it reports false before the first
// completion.
func runETA(m Model) (time.Duration, bool) {
	if len(m.workloadDoneAt) == 0 {
		return 0, false
	}
	remaining := 0
	for _, w := range m.workloads {
		switch w.status {
		case "waiting":
			remaining++
		}
	}
	if remaining == 0 {
		return 0, false
	}
	var total time.Duration
	for _, d := range m.workloadDoneAt {
		total += d
	}
	avg := total / time.Duration(len(m.workloadDoneAt))
	return (avg * time.Duration(remaining)).Truncate(time.Second), true
}

func handleConfirm(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y":
		m.confirm = false
		if m.cancel != nil {
			m.cancel()
		}
		m.page = pageDashboard
		return m, nil
	case "n", "esc":
		m.confirm = false
		return m, nil
	}
	return m, nil
}

func viewRunning(m Model) string {
	t := theme.Active
	width := m.width

	done := 0
	running := 0
	failed := 0
	total := len(m.workloads)
	for _, w := range m.workloads {
		switch w.status {
		case "done":
			done++
		case "running":
			running++
		case "fail":
			failed++
		case "skip":
			done++
		}
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(done) / float64(total)
	}

	elapsed := time.Duration(0)
	if !m.startedAt.IsZero() {
		elapsed = time.Since(m.startedAt).Truncate(time.Second)
	}
	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(
		i18n.Tf("tui.running.title", map[string]any{
			"Phase":  i18n.Tfallback("tui.phase."+m.phase, m.phase),
			"Engine": strings.ToUpper(firstStr(m.engine, "external")),
		}),
	)
	timeStr := lipgloss.NewStyle().Foreground(t.Muted).Render(
		i18n.Tf("tui.running.elapsed", map[string]any{"Elapsed": elapsed.String()}),
	)
	headParts := []string{headerTitle, "    ", timeStr}
	if eta, ok := runETA(m); ok {
		headParts = append(headParts, "   ",
			lipgloss.NewStyle().Foreground(t.Secondary).Render(
				i18n.Tf("tui.running.eta", map[string]any{"Eta": eta.String()}),
			))
	}
	if m.runSamplesTotal > 0 {
		headParts = append(headParts, "   ",
			lipgloss.NewStyle().Foreground(t.Subtle).Render(
				i18n.Tf("tui.running.samples", map[string]any{"Done": m.runSamplesDone, "Total": m.runSamplesTotal}),
			))
	}
	headLine := lipgloss.JoinHorizontal(lipgloss.Bottom, headParts...)

	barWidth := width - 30
	if barWidth < 20 {
		barWidth = 20
	}
	progressLine := comp.ProgressLine(barWidth, ratio, i18n.T("tui.running.overall"), t.Primary) +
		lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("  %d/%d  ✗%d", done, total, failed))

	groups := groupWorkloads(m.workloads)
	cardW := width - 4
	if width >= 120 {
		cardW = (width - 6) / 2
	}

	var cards []string
	for _, g := range groups {
		cards = append(cards, runningCard(m, g, cardW))
	}

	var grid string
	if width >= 120 {
		grid = pairCards(cards, width)
	} else {
		grid = strings.Join(cards, "\n")
	}

	parts := []string{headLine, "", progressLine, "", grid}

	if m.showLog && len(m.eventLog) > 0 {
		logLines := m.eventLog
		if len(logLines) > 8 {
			logLines = logLines[len(logLines)-8:]
		}
		body := lipgloss.NewStyle().Foreground(t.Muted).Render(strings.Join(logLines, "\n"))
		logCard := comp.Card{
			Title:  i18n.T("tui.running.eventLog"),
			Body:   body,
			Accent: t.Accent,
			Width:  width - 2,
		}
		parts = append(parts, "", logCard.Render())
	}

	view := strings.Join(parts, "\n")

	if m.confirm {
		modal := comp.Modal{
			Title: i18n.T("tui.modal.cancelBenchmark"),
			Body:  i18n.T("tui.modal.cancelBenchmarkBody"),
			Actions: []comp.ModalAction{
				{Key: "y", Label: i18n.T("tui.modal.cancelRun"), Selected: true, Danger: true},
				{Key: "n", Label: i18n.T("tui.modal.keepRunning")},
			},
			Width: 50,
		}
		view += "\n\n" + lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(modal.Render())
	}

	return view
}

type workloadGroup struct {
	name      string
	category  string
	workloads []workloadState
}

func groupWorkloads(ws []workloadState) []workloadGroup {
	order := []string{}
	byCat := map[string][]workloadState{}
	for _, w := range ws {
		cat := w.category
		if cat == "" {
			cat = "Other"
		}
		if _, ok := byCat[cat]; !ok {
			order = append(order, cat)
		}
		byCat[cat] = append(byCat[cat], w)
	}
	out := make([]workloadGroup, 0, len(order))
	for _, c := range order {
		out = append(out, workloadGroup{name: c, category: c, workloads: byCat[c]})
	}
	return out
}

func runningCard(m Model, g workloadGroup, width int) string {
	t := theme.Active

	var lines []string
	for _, w := range g.workloads {
		var status string
		switch w.status {
		case "done":
			status = comp.StatusPill(comp.StatusDone, w.metric)
		case "running":
			status = m.spinner.View() + " " + lipgloss.NewStyle().Foreground(t.Warning).Render(i18n.StatusLabel("running"))
			if w.iterTotal > 0 {
				ratio := float64(w.iterCur) / float64(w.iterTotal)
				if ratio > 1 {
					ratio = 1
				}
				status += " " + comp.ProgressBar(6, ratio, t.Warning) +
					" " + fmt.Sprintf("%d/%d", w.iterCur, w.iterTotal)
			}
			if !w.startedAt.IsZero() {
				status += " " + lipgloss.NewStyle().Foreground(t.Subtle).Render(
					time.Since(w.startedAt).Truncate(time.Second).String())
			}
		case "fail":
			msg := i18n.TruncateCells(w.metric, 30)
			status = comp.StatusPill(comp.StatusFail, msg)
		case "skip":
			status = comp.StatusPill(comp.StatusSkip, i18n.StatusLabel("skipped"))
		default:
			status = comp.StatusPill(comp.StatusWaiting, i18n.StatusLabel("waiting"))
		}
		nameW := width - 35
		if nameW < 14 {
			nameW = 14
		}
		name := truncStr(w.name, nameW)
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(nameW).Render(name)
		lines = append(lines, nameStyled+" "+status)
	}

	card := comp.Card{
		Title:  g.name,
		Body:   strings.Join(lines, "\n"),
		Accent: t.CategoryColor(g.category),
		Width:  width,
	}
	return card.Render()
}

func pairCards(cards []string, width int) string {
	var rows []string
	for i := 0; i < len(cards); i += 2 {
		if i+1 < len(cards) {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards[i], "  ", cards[i+1]))
		} else {
			rows = append(rows, cards[i])
		}
	}
	return strings.Join(rows, "\n")
}

func firstStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
