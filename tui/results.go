package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type saveDoneMsg struct {
	path string
	err  error
}

func updateResults(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.page = pageDashboard
			m.report = nil
			return m, nil
		case "tab":
			m.resultsTab = (m.resultsTab + 1) % 3
			m.resultsCur = 0
			return m, nil
		case "up", "k":
			if m.resultsCur > 0 {
				m.resultsCur--
			}
			return m, nil
		case "down", "j":
			maxScroll := maxResultsScroll(m)
			if m.resultsCur < maxScroll {
				m.resultsCur++
			}
			return m, nil
		case "enter":
			if m.resultsTab == 1 && m.report != nil {
				cats := resultCategories(m.report.Results.Workloads)
				if m.resultsCur < len(cats) {
					cat := cats[m.resultsCur]
					m.expanded[cat] = !m.expanded[cat]
				}
			}
			return m, nil
		case "s":
			if m.report != nil {
				return m, saveReportCmd(m.report)
			}
		case "q":
			return m, tea.Quit
		}
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
	case comp.ToastExpireMsg:
		if msg.Stamp == m.toast.Until {
			m.toast = comp.Toast{}
		}
		return m, nil
	}
	return m, nil
}

func maxResultsScroll(m Model) int {
	if m.report == nil {
		return 0
	}
	switch m.resultsTab {
	case 0:
		return len(m.report.Results.Workloads) - 1
	case 1:
		return len(resultCategories(m.report.Results.Workloads)) - 1
	default:
		return 0
	}
}

func saveReportCmd(report *vmbench.Report) tea.Cmd {
	return func() tea.Msg {
		path := "vmbench-report.json"
		f, err := os.Create(path)
		if err != nil {
			return saveDoneMsg{path: path, err: err}
		}
		defer f.Close()
		if err := gbreport.WriteJSON(f, *report); err != nil {
			return saveDoneMsg{path: path, err: err}
		}
		return saveDoneMsg{path: path}
	}
}

func viewResults(m Model) string {
	if m.report == nil {
		return lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  No results available.")
	}

	t := theme.Active
	width := m.width
	doc := *m.report

	total, okCount, failCount := resultStats(doc.Results.Workloads, doc.Extensions.Workloads)

	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("◈ Benchmark Results")
	stats := lipgloss.NewStyle().Foreground(t.Muted).Render(
		fmt.Sprintf("  workloads %d  •  ", total),
	) +
		lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(fmt.Sprintf("✓ %d ok", okCount)) +
		lipgloss.NewStyle().Foreground(t.Muted).Render("  •  ") +
		lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render(fmt.Sprintf("✗ %d fail", failCount))

	tabs := comp.Tabs(width, []comp.Tab{
		{Label: "Cards"},
		{Label: "Grouped"},
		{Label: "Flat"},
	}, m.resultsTab)

	var body string
	switch m.resultsTab {
	case 0:
		body = viewResultsCards(m, width)
	case 1:
		body = viewResultsGrouped(m)
	default:
		body = viewResultsFlat(m, width)
	}

	parts := []string{headerTitle, stats, "", tabs, "", body}

	if len(doc.Extensions.Workloads) > 0 {
		extHeader := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render("◇ Extensions")
		parts = append(parts, "", extHeader, viewWorkloadRows(doc.Extensions.Workloads, -1, width))
	}

	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width))
	}

	return strings.Join(parts, "\n")
}

func viewResultsCards(m Model, width int) string {
	doc := *m.report
	groups := groupResults(doc.Results.Workloads)
	cardW := width - 4
	cols := comp.ColumnsFor(width)
	if cols > 1 {
		cardW = (width - (cols-1)*2 - 2) / cols
	}

	var cards []string
	for _, g := range groups {
		cards = append(cards, resultCard(g, cardW))
	}
	return gridCards(cards, cols)
}

func resultCard(g resultGroup, width int) string {
	t := theme.Active
	var lines []string
	for _, w := range g.workloads {
		nameW := width - 30
		if nameW < 12 {
			nameW = 12
		}
		nameStyled := lipgloss.NewStyle().
			Foreground(t.Fg).
			Width(nameW).
			Render(truncStr(w.Name, nameW))

		var metric string
		if w.Result == nil {
			metric = lipgloss.NewStyle().Foreground(t.Muted).Render("—")
		} else if w.Result.Error != "" {
			metric = lipgloss.NewStyle().Foreground(t.Danger).Render("error")
		} else {
			thr := tuiThroughput(w.Result)
			if thr != "-" {
				metric = lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(thr)
			} else if w.Result.AvgNSPerAccess > 0 {
				metric = lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(tuiLatency(w.Result))
			} else {
				metric = lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(tuiTime(w.Result))
			}
		}
		lines = append(lines, nameStyled+" "+metric)
	}

	card := comp.Card{
		Title:    g.name,
		Subtitle: fmt.Sprintf("%d workloads", len(g.workloads)),
		Body:     strings.Join(lines, "\n"),
		Accent:   theme.Active.CategoryColor(g.category),
		Width:    width,
	}
	return card.Render()
}

func gridCards(cards []string, cols int) string {
	if cols <= 1 {
		return strings.Join(cards, "\n")
	}
	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		row := cards[i]
		for j := i + 1; j < end; j++ {
			row = lipgloss.JoinHorizontal(lipgloss.Top, row, "  ", cards[j])
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

type resultGroup struct {
	name      string
	category  string
	workloads []gbreport.WorkloadEntry
}

func groupResults(ws []gbreport.WorkloadEntry) []resultGroup {
	order := []string{}
	byCat := map[string][]gbreport.WorkloadEntry{}
	for _, w := range ws {
		cat := w.Category
		if cat == "" {
			cat = "Other"
		}
		if _, ok := byCat[cat]; !ok {
			order = append(order, cat)
		}
		byCat[cat] = append(byCat[cat], w)
	}
	out := make([]resultGroup, 0, len(order))
	for _, c := range order {
		out = append(out, resultGroup{name: c, category: c, workloads: byCat[c]})
	}
	return out
}

func viewResultsFlat(m Model, width int) string {
	cols := []comp.TableColumn{
		{Title: "Workload", Width: 22},
		{Title: "Category", Width: 12},
		{Title: "Time", Width: 10, Align: lipgloss.Right},
		{Title: "Throughput", Width: 18, Align: lipgloss.Right},
		{Title: "Latency", Width: 10, Align: lipgloss.Right},
		{Title: "Status", Width: 8},
	}
	if width < 100 {
		cols = []comp.TableColumn{
			{Title: "Workload", Width: 20},
			{Title: "Time", Width: 10, Align: lipgloss.Right},
			{Title: "Throughput", Width: 18, Align: lipgloss.Right},
			{Title: "Status", Width: 8},
		}
	}

	var rows []comp.TableRow
	for i, w := range m.report.Results.Workloads {
		var cells []string
		if width < 100 {
			cells = []string{
				w.Name,
				tuiTime(w.Result),
				tuiThroughput(w.Result),
				tuiStatusText(w.Result),
			}
		} else {
			cells = []string{
				w.Name,
				w.Category,
				tuiTime(w.Result),
				tuiThroughput(w.Result),
				tuiLatency(w.Result),
				tuiStatusText(w.Result),
			}
		}
		rows = append(rows, comp.TableRow{
			Cells:     cells,
			Highlight: i == m.resultsCur,
			Accent:    theme.Active.CategoryColor(w.Category),
		})
	}
	return comp.RenderTable(cols, rows)
}

func viewWorkloadRows(entries []gbreport.WorkloadEntry, cursor int, width int) string {
	cols := []comp.TableColumn{
		{Title: "Workload", Width: 22},
		{Title: "Time", Width: 10, Align: lipgloss.Right},
		{Title: "Throughput", Width: 18, Align: lipgloss.Right},
		{Title: "Status", Width: 8},
	}
	var rows []comp.TableRow
	for i, w := range entries {
		rows = append(rows, comp.TableRow{
			Cells: []string{
				w.Name,
				tuiTime(w.Result),
				tuiThroughput(w.Result),
				tuiStatusText(w.Result),
			},
			Highlight: i == cursor,
		})
	}
	return comp.RenderTable(cols, rows)
}

func viewResultsGrouped(m Model) string {
	t := theme.Active
	var b strings.Builder
	cats := resultCategories(m.report.Results.Workloads)
	line := 0

	for _, cat := range cats {
		icon := "▸"
		if m.expanded[cat] {
			icon = "▾"
		}
		cur := "  "
		if line == m.resultsCur {
			cur = lipgloss.NewStyle().Foreground(t.Primary).Render("▶ ")
		}
		catLine := lipgloss.NewStyle().Bold(true).Foreground(t.CategoryColor(cat)).Render(icon + " " + cat)
		b.WriteString(cur + catLine + "\n")

		if m.expanded[cat] {
			for _, w := range m.report.Results.Workloads {
				if w.Category != cat {
					continue
				}
				name := lipgloss.NewStyle().Width(22).Foreground(t.Fg).Render(truncStr(w.Name, 22))
				time := lipgloss.NewStyle().Width(10).Align(lipgloss.Right).Foreground(t.Fg).Render(tuiTime(w.Result))
				thr := lipgloss.NewStyle().Width(18).Align(lipgloss.Right).Foreground(t.Fg).Render(tuiThroughput(w.Result))
				lat := lipgloss.NewStyle().Width(10).Align(lipgloss.Right).Foreground(t.Fg).Render(tuiLatency(w.Result))
				status := tuiStatusText(w.Result)
				b.WriteString("    " + name + " " + time + " " + thr + " " + lat + " " + status + "\n")
			}
		}
		line++
	}
	return b.String()
}

func resultStats(groups ...[]gbreport.WorkloadEntry) (total int, okCount int, failCount int) {
	for _, entries := range groups {
		for _, item := range entries {
			total++
			if item.Result != nil && strings.TrimSpace(item.Result.Error) != "" {
				failCount++
				continue
			}
			okCount++
		}
	}
	return total, okCount, failCount
}

func resultCategories(entries []gbreport.WorkloadEntry) []string {
	seen := map[string]struct{}{}
	for _, item := range entries {
		seen[item.Category] = struct{}{}
	}
	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	return cats
}

func tuiTime(result *gbreport.ResultEntry) string {
	if result == nil {
		return "-"
	}
	if result.Error != "" && result.MedianMS == 0 {
		return "ERR"
	}
	return fmt.Sprintf("%.1fms", result.MedianMS)
}

func tuiThroughput(result *gbreport.ResultEntry) string {
	if result == nil || result.ThroughputPerSec <= 0 {
		return "-"
	}
	unit := strings.TrimSpace(result.ThroughputUnit)
	if unit == "" {
		unit = "ops/s"
	}
	if result.ThroughputPerSec >= 100 {
		return fmt.Sprintf("%.0f %s", result.ThroughputPerSec, unit)
	}
	return fmt.Sprintf("%.2f %s", result.ThroughputPerSec, unit)
}

func tuiLatency(result *gbreport.ResultEntry) string {
	if result == nil || result.AvgNSPerAccess <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fns", result.AvgNSPerAccess)
}

func tuiStatusText(result *gbreport.ResultEntry) string {
	t := theme.Active
	if result == nil {
		return lipgloss.NewStyle().Foreground(t.Muted).Render("—")
	}
	if strings.TrimSpace(result.Error) != "" {
		return lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render("✗ fail")
	}
	return lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render("✓ ok")
}
