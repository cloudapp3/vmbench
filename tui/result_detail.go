package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func updateResultDetail(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "d":
		m.page = pageResults
		return m, nil
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

// viewResultDetail renders one workload's full evidence: metrics, samples,
// raw Detail output, and structured error.
func viewResultDetail(m Model) string {
	t := theme.Active
	width := m.width

	if m.report == nil || m.resultsDetail < 0 || m.resultsDetail >= len(m.report.Results.Workloads) {
		return lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  " + i18n.T("tui.detail.noDetail"))
	}
	entry := m.report.Results.Workloads[m.resultsDetail]

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.detail.title"))
	name := lipgloss.NewStyle().Bold(true).Foreground(t.Fg).Render(truncStr(entry.Name, width-8))
	category := comp.StatusPill(comp.StatusOK, entry.Category)

	var parts []string
	parts = append(parts, title, "", name+"  "+category, "")

	cardW := width - 4
	if cardW < 32 {
		cardW = 32
	}

	if entry.Result == nil {
		parts = append(parts, lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.detail.noDetail")))
		return strings.Join(parts, "\n")
	}
	res := entry.Result

	rows := []comp.KV{
		{Key: i18n.T("tui.col.time"), Value: tuiTime(res)},
		{Key: i18n.T("tui.col.throughput"), Value: tuiThroughput(res)},
		{Key: i18n.T("tui.col.latency"), Value: tuiLatency(res)},
		{Key: i18n.T("tui.col.iterations"), Value: fmt.Sprintf("%d", res.Iterations)},
	}
	if res.BytesProcessed > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.col.bytes"), Value: fmt.Sprintf("%d", res.BytesProcessed)})
	}
	if res.OpsProcessed > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.col.ops"), Value: fmt.Sprintf("%.0f", res.OpsProcessed)})
	}
	metricsCard := comp.Card{
		Title:  i18n.T("tui.detail.metrics"),
		Body:   comp.KVGrid(cardW-4, rows),
		Accent: t.CategoryColor(entry.Category),
		Width:  cardW,
	}
	parts = append(parts, metricsCard.Render())

	if strings.TrimSpace(res.Error) != "" {
		errCard := comp.Card{
			Title:  i18n.T("tui.detail.errorTitle"),
			Body:   clipDetailLines(res.Error, cardW),
			Accent: t.Danger,
			Width:  cardW,
		}
		parts = append(parts, "", errCard.Render())
	}

	if len(res.SamplesMS) > 0 {
		samples := make([]string, 0, len(res.SamplesMS))
		for i, s := range res.SamplesMS {
			samples = append(samples, fmt.Sprintf("#%d %.1fms", i+1, s))
		}
		samplesCard := comp.Card{
			Title:  i18n.T("tui.detail.samples"),
			Body:   strings.Join(samples, "  "),
			Accent: t.CategorySystem,
			Width:  cardW,
		}
		parts = append(parts, "", samplesCard.Render())
	}

	detail := strings.TrimSpace(res.Detail)
	if detail != "" {
		detailCard := comp.Card{
			Title:  i18n.T("tui.detail.detailTitle"),
			Body:   clipDetailLines(detail, cardW),
			Accent: t.Accent,
			Width:  cardW,
		}
		parts = append(parts, "", detailCard.Render())
	} else if strings.TrimSpace(res.Error) == "" {
		parts = append(parts, "", lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.detail.noDetail")))
	}

	return strings.Join(parts, "\n")
}

// clipDetailLines truncates each raw output line to the card width so long
// tool output (fio etc.) stays readable without breaking the layout.
func clipDetailLines(detail string, cardW int) string {
	max := cardW - 4
	if max < 20 {
		max = 20
	}
	lines := strings.Split(detail, "\n")
	for i, line := range lines {
		lines[i] = truncStr(line, max)
	}
	out := strings.Join(lines, "\n")
	if len(lines) > 40 {
		out = strings.Join(lines[:40], "\n"+"…")
	}
	return out
}
