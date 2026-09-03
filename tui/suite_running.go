package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type suiteEventMsg struct{ event suite.Event }
type suiteDoneMsg struct{ report suite.SuiteReport }

type suiteSection struct {
	id      suite.SectionID
	label   string
	status  string
	message string
}

func newSuiteSections(sel suite.SectionSelector) []suiteSection {
	order := []struct {
		id      suite.SectionID
		label   string
		enabled bool
	}{
		{suite.SectionHardware, "Hardware", sel.Hardware},
		{suite.SectionNetworkInfo, "Network Info", sel.NetworkInfo},
		{suite.SectionRoute, "Route", sel.Route},
		{suite.SectionPing, "Ping", sel.Ping},
		{suite.SectionSpeed, "Speed", sel.Speed},
		{suite.SectionIPQuality, "IP Quality", sel.IPQuality},
		{suite.SectionReachability, "Reachability", sel.Reachability},
		{suite.SectionMail, "Mail Ports", sel.Mail},
		{suite.SectionMedia, "Media Unlock", sel.Media},
	}
	out := make([]suiteSection, 0, len(order))
	for _, o := range order {
		if !o.enabled {
			continue
		}
		out = append(out, suiteSection{id: o.id, label: o.label, status: "waiting"})
	}
	return out
}

func startSuite(m Model, opts suite.Options) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.suiteEventCh = make(chan suite.Event, 50)
	m.suiteSections = newSuiteSections(opts.Sections)
	m.eventLog = m.eventLog[:0]
	m.startedAt = time.Now()
	m.page = pageSuiteRunning

	return m, tea.Batch(
		runSuiteCmd(ctx, opts, m.suiteEventCh),
		waitForSuiteEvent(m.suiteEventCh),
		m.spinner.Tick,
	)
}

func runSuiteCmd(ctx context.Context, opts suite.Options, ch chan<- suite.Event) tea.Cmd {
	return func() tea.Msg {
		opts.OnEvent = func(ev suite.Event) {
			ch <- ev
		}
		report := suite.Run(ctx, opts)
		return suiteDoneMsg{report: report}
	}
}

func waitForSuiteEvent(ch <-chan suite.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return suiteEventMsg{event: ev}
	}
}

func updateSuiteEvent(m Model, ev suite.Event) (tea.Model, tea.Cmd) {
	addLog := func(msg string) {
		m.eventLog = append(m.eventLog, fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg))
		if len(m.eventLog) > 50 {
			m.eventLog = m.eventLog[len(m.eventLog)-50:]
		}
	}

	updateSection := func(status, msg string) {
		for i := range m.suiteSections {
			if m.suiteSections[i].id == ev.Section {
				m.suiteSections[i].status = status
				if msg != "" {
					m.suiteSections[i].message = msg
				}
				return
			}
		}
	}

	switch ev.Kind {
	case suite.EventSectionStart:
		updateSection("running", "")
		addLog("▸ start  " + string(ev.Section))
	case suite.EventSectionDone:
		updateSection("done", ev.Message)
		addLog("✓ done   " + string(ev.Section) + "  " + ev.Message)
	case suite.EventSectionFail:
		status := strings.ToLower(strings.TrimSpace(ev.Status))
		marker := "✗"
		switch status {
		case "partial":
			marker = "!"
		case "skip", "skipped":
			status = "skipped"
			marker = "⊘"
		case "error", "fail", "failed":
		default:
			status = "fail"
		}
		updateSection(status, ev.Message)
		addLog(marker + " " + status + "   " + string(ev.Section) + "  " + ev.Message)
	case suite.EventSectionSkip:
		updateSection("skip", "")
	case suite.EventSuiteDone:
		addLog("● suite complete  " + ev.Message)
	}
	return m, waitForSuiteEvent(m.suiteEventCh)
}

func updateSuiteRunning(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func viewSuiteRunning(m Model) string {
	t := theme.Active
	width := m.width

	total := len(m.suiteSections)
	done := 0
	failed := 0
	for _, s := range m.suiteSections {
		switch s.status {
		case "done":
			done++
		case "partial":
			done++
			failed++
		case "fail", "failed", "error", "skipped":
			done++
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

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	header := lipgloss.JoinHorizontal(lipgloss.Bottom,
		titleStyle.Render("◈ Running Suite"),
		"    ",
		lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("elapsed %s", elapsed)),
	)

	barWidth := width - 30
	if barWidth < 20 {
		barWidth = 20
	}
	progressLine := comp.ProgressLine(barWidth, ratio, "Overall", t.Primary) +
		lipgloss.NewStyle().Foreground(t.Muted).Render(fmt.Sprintf("  %d/%d  ✗%d", done, total, failed))
	if m.height < 40 {
		return viewSuiteRunningCompact(m, header, progressLine)
	}

	cardW := width - 4
	if width >= 120 {
		cardW = (width - 6) / 2
	}

	var cards []string
	for _, s := range m.suiteSections {
		cards = append(cards, suiteSectionCard(m, s, cardW))
	}

	var grid string
	if width >= 120 {
		grid = pairCards(cards, width)
	} else {
		grid = strings.Join(cards, "\n")
	}

	parts := []string{header, "", progressLine, "", grid}

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
			Title: "Cancel suite?",
			Body:  "Running section will be aborted.",
			Actions: []comp.ModalAction{
				{Key: "y", Label: "Cancel suite", Selected: true, Danger: true},
				{Key: "n", Label: "Keep running"},
			},
			Width: 50,
		}
		view += "\n\n" + lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(modal.Render())
	}

	return view
}

func viewSuiteRunningCompact(m Model, header, progressLine string) string {
	t := theme.Active
	lineWidth := m.width - 4
	parts := []string{header, progressLine, ""}
	for _, section := range m.suiteSections {
		parts = append(parts, suiteSectionCompactLine(m, section, lineWidth))
	}

	if m.showLog && len(m.eventLog) > 0 {
		logLines := m.eventLog
		if len(logLines) > 2 {
			logLines = logLines[len(logLines)-2:]
		}
		parts = append(parts, "", lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("Recent events"))
		for _, line := range logLines {
			parts = append(parts, lipgloss.NewStyle().Foreground(t.Muted).Render(truncStr(line, lineWidth)))
		}
	}
	if m.confirm {
		prompt := lipgloss.NewStyle().Bold(true).Foreground(t.Danger).Render("Cancel suite?") +
			lipgloss.NewStyle().Foreground(t.Muted).Render("  y cancel  ·  n keep running")
		parts = append(parts, "", prompt)
	}
	return strings.Join(parts, "\n")
}

func suiteSectionCompactLine(m Model, s suiteSection, width int) string {
	const labelWidth = 20
	label := lipgloss.NewStyle().Bold(true).Foreground(sectionAccent(s.id)).Width(labelWidth).
		Render(truncStr(s.label, labelWidth))
	return label + suiteSectionStatus(m, s, width-labelWidth)
}

func suiteSectionStatus(m Model, s suiteSection, maxWidth int) string {
	if maxWidth < 8 {
		maxWidth = 8
	}
	switch s.status {
	case "done":
		return comp.StatusPill(comp.StatusDone, truncStr(firstStr(s.message, "ok"), maxWidth-2))
	case "partial":
		return comp.StatusPill(comp.StatusPartial, truncStr(firstStr(s.message, "partial"), maxWidth-2))
	case "fail", "failed", "error":
		return comp.StatusPill(comp.StatusFail, truncStr(firstStr(s.message, "failed"), maxWidth-2))
	case "skip", "skipped":
		return comp.StatusPill(comp.StatusSkip, "skipped")
	case "running":
		return m.spinner.View() + " " + lipgloss.NewStyle().Foreground(theme.Active.Warning).Render("running...")
	default:
		return comp.StatusPill(comp.StatusWaiting, "waiting")
	}
}

func suiteSectionCard(m Model, s suiteSection, width int) string {
	accent := sectionAccent(s.id)

	card := comp.Card{
		Title:  s.label,
		Body:   suiteSectionStatus(m, s, 42),
		Accent: accent,
		Width:  width,
	}
	return card.Render()
}

func sectionAccent(id suite.SectionID) lipgloss.AdaptiveColor {
	t := theme.Active
	switch id {
	case suite.SectionHardware:
		return t.CategorySystem
	case suite.SectionNetworkInfo:
		return t.Info
	case suite.SectionRoute, suite.SectionPing:
		return t.CategoryNetwork
	case suite.SectionSpeed:
		return t.CategoryInteger
	case suite.SectionIPQuality:
		return t.Accent
	case suite.SectionReachability:
		return t.CategoryNetwork
	case suite.SectionMail:
		return t.CategoryFloat
	case suite.SectionMedia:
		return t.CategoryDisk
	default:
		return t.Primary
	}
}
