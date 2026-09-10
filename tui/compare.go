package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type compareLoadedMsg struct {
	docs []gbreport.Document
	err  error
}

func updateCompare(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.page = pageComparePicker
			return m, nil
		case "r":
			// Back to the picker to change the selection.
			m.page = pageComparePicker
			if m.picker.records == nil && !m.picker.loading && m.picker.err == nil {
				m.picker.loading = true
				return m, loadHistoryCmd()
			}
			return m, nil
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

// loadCompareCmd loads both compare reports off the render path so View()
// stays free of file IO.
func loadCompareCmd(a, b string) tea.Cmd {
	return func() tea.Msg {
		docs, err := loadCompareDocs(a, b)
		return compareLoadedMsg{docs: docs, err: err}
	}
}

func viewCompare(m Model) string {
	t := theme.Active

	if m.compareA == "" || m.compareB == "" {
		title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.compare.title"))
		body := lipgloss.NewStyle().Foreground(t.Fg).Render(i18n.T("tui.compare.usageBody"))
		card := comp.Card{
			Title:  i18n.T("tui.compare.howTo"),
			Body:   body,
			Accent: t.Accent,
			Width:  m.width - 4,
		}
		return title + "\n\n" + card.Render()
	}

	if m.compareLoading && len(m.compareDocs) == 0 {
		return lipgloss.NewStyle().Foreground(t.Muted).Render("  " + m.spinner.View() + " " + i18n.T("tui.compare.loading"))
	}
	if m.compareErr != nil {
		return lipgloss.NewStyle().Foreground(t.Danger).Render("  " + i18n.Tf("tui.compare.error", map[string]any{"Err": m.compareErr.Error()}))
	}
	if len(m.compareDocs) != 2 {
		return lipgloss.NewStyle().Foreground(t.Danger).Render("  " + i18n.Tf("tui.compare.error", map[string]any{"Err": "not loaded"}))
	}

	// Checkup comparisons arrive as pre-rendered textgrid output from
	// checkupcompare; the P0 clipping keeps the long table scrollable.
	if m.compareKind == string(history.KindCheckup) {
		if m.checkupCompareText == "" {
			return lipgloss.NewStyle().Foreground(t.Muted).Render("  " + i18n.T("tui.compare.loading"))
		}
		title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.compare.title"))
		return strings.Join([]string{title, "", m.checkupCompareText}, "\n")
	}

	docs := m.compareDocs

	dA, dB := docs[0], docs[1]
	width := m.width

	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.compare.title"))

	cardW := (width - 6) / 2
	if cardW < 30 {
		cardW = (width - 4)
	}

	cardA := compareSysCard("A", m.compareA, dA, cardW, t.CategoryInteger)
	cardB := compareSysCard("B", m.compareB, dB, cardW, t.CategoryDisk)

	var sysRow string
	if width >= 80 {
		sysRow = lipgloss.JoinHorizontal(lipgloss.Top, cardA, "  ", cardB)
	} else {
		sysRow = cardA + "\n" + cardB
	}

	mapA := workloadMap(append(dA.Results.Workloads, dA.Extensions.Workloads...))
	mapB := workloadMap(append(dB.Results.Workloads, dB.Extensions.Workloads...))

	cols := []comp.TableColumn{
		{Title: i18n.T("tui.col.workload"), Width: comp.ColWidth(i18n.T("tui.col.workload"), 22)},
		{Title: i18n.T("tui.col.metric"), Width: comp.ColWidth(i18n.T("tui.col.metric"), 11)},
		{Title: "A", Width: comp.ColWidth("A", 12), Align: lipgloss.Right},
		{Title: "B", Width: comp.ColWidth("B", 12), Align: lipgloss.Right},
		{Title: "Δ", Width: comp.ColWidth("Δ", 14), Align: lipgloss.Right},
	}
	var rows []comp.TableRow
	for _, name := range sortedWorkloadNames(mapA, mapB) {
		eA, okA := mapA[name]
		eB, okB := mapB[name]
		var rA, rB *gbreport.ResultEntry
		if okA {
			rA = eA.Result
		}
		if okB {
			rB = eB.Result
		}
		rows = appendDeltaRows(rows, name, rA, rB)
	}

	tableTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(i18n.T("tui.compare.workloadDelta"))
	table := comp.RenderTable(cols, rows)

	return strings.Join([]string{
		headerTitle,
		"",
		sysRow,
		"",
		tableTitle,
		table,
	}, "\n")
}

func compareSysCard(label, path string, doc gbreport.Document, width int, accent lipgloss.AdaptiveColor) string {
	t := theme.Active
	rows := []comp.KV{
		{Key: i18n.T("tui.compare.path"), Value: truncStr(path, width-12)},
		{Key: "CPU", Value: truncStr(doc.System.CPU.Model, width-12)},
		{Key: i18n.T("tui.compare.cores"), Value: fmt.Sprintf("%d/%d", doc.System.CPU.PhysicalCores, doc.System.CPU.LogicalCores)},
		{Key: i18n.T("tui.sys.memory"), Value: fmt.Sprintf("%.1f GB", float64(doc.System.Memory.TotalBytes)/(1024*1024*1024))},
		{Key: i18n.T("tui.sys.os"), Value: truncStr(doc.System.OS.Name, width-12)},
	}
	if nic := doc.System.Network.PrimaryNIC(); nic != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.nic"), Value: truncStr(nic, width-12)})
	}
	if oversell := doc.System.Platform.OversellSignalsText(); oversell != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.oversell"), Value: truncStr(oversell, width-12)})
	}
	body := comp.KVGrid(width-4, rows)
	_ = t
	return comp.Card{
		Title:    i18n.Tf("tui.compare.reportCard", map[string]any{"Label": label}),
		Subtitle: doc.Timestamp.Format("2006-01-02 15:04"),
		Body:     body,
		Accent:   accent,
		Width:    width,
	}.Render()
}

func appendDeltaRows(rows []comp.TableRow, name string, rA, rB *gbreport.ResultEntry) []comp.TableRow {
	add := func(metric string, a, b float64, unit string, lowerIsBetter bool) {
		if a <= 0 && b <= 0 {
			return
		}
		rows = append(rows, comp.TableRow{
			Cells: []string{
				name,
				metric,
				fmtMeasured(a, unit),
				fmtMeasured(b, unit),
				formatMetricDelta(a, b, lowerIsBetter),
			},
		})
		name = ""
	}
	add("time", valueTime(rA), valueTime(rB), "ms", true)
	throughputUnit := firstThroughputUnit(rA, rB)
	if throughputUnitsCompatible(rA, rB) {
		add("throughput", valueThroughput(rA), valueThroughput(rB), throughputUnit, throughputLowerIsBetter(throughputUnit))
	} else {
		rows = append(rows, comp.TableRow{Cells: []string{
			name,
			"throughput",
			fmtMeasured(valueThroughput(rA), firstThroughputUnit(rA)),
			fmtMeasured(valueThroughput(rB), firstThroughputUnit(rB)),
			"incompatible units",
		}})
		name = ""
	}
	add("latency", valueLatency(rA), valueLatency(rB), "ns/op", true)
	add("latency p99", valueLatencyP99(rA), valueLatencyP99(rB), "ns", true)
	return rows
}

func loadCompareDocs(a, b string) ([]gbreport.Document, error) {
	docs := make([]gbreport.Document, 2)
	for i, path := range []string{a, b} {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		if err := json.Unmarshal(data, &docs[i]); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	return docs, nil
}

func valueTime(r *gbreport.ResultEntry) float64 {
	if r == nil || strings.TrimSpace(r.Error) != "" {
		return 0
	}
	return r.MedianMS
}

func valueThroughput(r *gbreport.ResultEntry) float64 {
	if r == nil || strings.TrimSpace(r.Error) != "" {
		return 0
	}
	return r.ThroughputPerSec
}

func valueLatency(r *gbreport.ResultEntry) float64 {
	if r == nil || strings.TrimSpace(r.Error) != "" {
		return 0
	}
	return r.AvgNSPerAccess
}

func valueLatencyP99(r *gbreport.ResultEntry) float64 {
	if r == nil || strings.TrimSpace(r.Error) != "" {
		return 0
	}
	return r.LatencyP99NS
}

func firstThroughputUnit(items ...*gbreport.ResultEntry) string {
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ThroughputUnit) != "" {
			return item.ThroughputUnit
		}
	}
	return ""
}

func throughputUnitsCompatible(items ...*gbreport.ResultEntry) bool {
	unit := ""
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.Error) != "" || item.ThroughputPerSec <= 0 {
			continue
		}
		current := strings.TrimSpace(item.ThroughputUnit)
		if unit == "" {
			unit = current
			continue
		}
		if current != unit {
			return false
		}
	}
	return true
}

func throughputLowerIsBetter(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "ms", "ms avg", "latency ms", "loss %", "% loss":
		return true
	default:
		return false
	}
}

func fmtMeasured(value float64, unit string) string {
	if value <= 0 {
		return "-"
	}
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return fmt.Sprintf("%.2f", value)
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f%s", value, unit)
	}
	return fmt.Sprintf("%.2f%s", value, unit)
}

func formatMetricDelta(base, target float64, lowerIsBetter bool) string {
	t := theme.Active
	if base <= 0 || target <= 0 {
		return lipgloss.NewStyle().Foreground(t.Muted).Render("—")
	}
	pct := (target - base) / base * 100
	if lowerIsBetter {
		pct = -pct
	}
	switch {
	case pct > 0.5:
		return lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(fmt.Sprintf("▲ %+.1f%%", pct))
	case pct < -0.5:
		return lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render(fmt.Sprintf("▼ %+.1f%%", pct))
	default:
		return lipgloss.NewStyle().Foreground(t.Muted).Render("=")
	}
}

func workloadMap(entries []gbreport.WorkloadEntry) map[string]gbreport.WorkloadEntry {
	m := make(map[string]gbreport.WorkloadEntry, len(entries))
	for _, e := range entries {
		m[e.Name] = e
	}
	return m
}

func sortedWorkloadNames(a, b map[string]gbreport.WorkloadEntry) []string {
	seen := map[string]struct{}{}
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
