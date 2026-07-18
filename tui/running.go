package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func startBenchmark(m Model, mode string, engine string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.eventCh = make(chan vmbench.Event, 100)
	m.phase = "Hardware"
	m.confirm = false
	m.startedAt = time.Now()
	m.eventLog = m.eventLog[:0]

	defs := catalog.ExternalHardwareDefinitions("")
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
		runBenchmarkCmd(ctx, mode, engine, m.eventCh),
		waitForEvent(m.eventCh),
		m.spinner.Tick,
	)
}

func runBenchmarkCmd(ctx context.Context, mode string, engine string, ch chan<- vmbench.Event) tea.Cmd {
	return func() tea.Msg {
		report := vmbench.RunCore(ctx, vmbench.Options{
			Mode:       mode,
			Engine:     engine,
			Scope:      vmbench.ScopeHardware,
			Iterations: 3,
			OnEvent: func(ev vmbench.Event) {
				ch <- ev
			},
		})
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
				break
			}
		}
		if strings.Contains(ev.Message, "multi-core") {
			m.phase = "Multi-Core"
		}
		addLog("▸ start  " + ev.Workload)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteDone:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
				m.workloads[i].status = "done"
				m.workloads[i].metric = ev.Metric
				m.workloads[i].duration = ev.Duration.String()
				break
			}
		}
		addLog("✓ done   " + ev.Workload + "  " + ev.Metric)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteFail:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
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
		addLog("✗ fail   " + ev.Workload + "  " + errMsg)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventSuiteSkip:
		for i := range m.workloads {
			if m.workloads[i].name == ev.Workload {
				m.workloads[i].status = "skip"
				break
			}
		}
		addLog("⊘ skip   " + ev.Workload)
		return m, waitForEvent(m.eventCh)

	case vmbench.EventBenchDone:
		addLog("● benchmark complete")
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
		fmt.Sprintf("▸ Running: %s (%s engine)", m.phase, strings.ToUpper(firstStr(m.engine, "external"))),
	)
	timeStr := lipgloss.NewStyle().Foreground(t.Muted).Render(
		fmt.Sprintf("elapsed %s", elapsed),
	)
	headLine := lipgloss.JoinHorizontal(lipgloss.Bottom, headerTitle, "    ", timeStr)

	barWidth := width - 30
	if barWidth < 20 {
		barWidth = 20
	}
	progressLine := comp.ProgressLine(barWidth, ratio, "Overall", t.Primary) +
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
			Title:  "Event Log",
			Body:   body,
			Accent: t.Accent,
			Width:  width - 2,
		}
		parts = append(parts, "", logCard.Render())
	}

	view := strings.Join(parts, "\n")

	if m.confirm {
		modal := comp.Modal{
			Title: "Cancel benchmark?",
			Body:  "Workloads in progress will be aborted.",
			Actions: []comp.ModalAction{
				{Key: "y", Label: "Cancel run", Selected: true, Danger: true},
				{Key: "n", Label: "Keep running"},
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
			status = m.spinner.View() + " " + lipgloss.NewStyle().Foreground(t.Warning).Render("running")
		case "fail":
			msg := w.metric
			if len(msg) > 30 {
				msg = msg[:29] + "…"
			}
			status = comp.StatusPill(comp.StatusFail, msg)
		case "skip":
			status = comp.StatusPill(comp.StatusSkip, "skipped")
		default:
			status = comp.StatusPill(comp.StatusWaiting, "waiting")
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

func truncate(s string, max int) string {
	return truncStr(s, max)
}
