package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
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
		case "esc", "q":
			if m.reportFrom != pageDashboard {
				from := m.reportFrom
				m.reportFrom = pageDashboard
				m.page = from
				return m, nil
			}
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
			return followFocus(m), nil
		case "down", "j":
			maxScroll := maxResultsScroll(m)
			if m.resultsCur < maxScroll {
				m.resultsCur++
			}
			return followFocus(m), nil
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
		case "d":
			// Open the full evidence view for the flat-table cursor row.
			if m.resultsTab == 2 && m.report != nil &&
				m.resultsCur >= 0 && m.resultsCur < len(m.report.Results.Workloads) {
				m.resultsDetail = m.resultsCur
				m.page = pageResultDetail
				return m, nil
			}
		}
	}
	return m, nil
}

func maxResultsScroll(m Model) int {
	if m.report == nil {
		return 0
	}
	switch m.resultsTab {
	case 1: // grouped: cursor walks category rows
		return len(resultCategories(m.report.Results.Workloads)) - 1
	case 2: // flat: cursor walks workload rows
		return len(m.report.Results.Workloads) - 1
	default: // cards grid has no cursor
		return 0
	}
}

// resultsFocusedLine mirrors viewResults/viewResultsFlat/viewResultsGrouped
// layout math to report the 0-based content line of the cursor row:
// parts [headerTitle, stats, "", tabs, ""] occupy lines 0-4, the flat table
// adds a header + separator, grouped emits one category line plus workload
// lines of expanded categories.
func resultsFocusedLine(m Model) (int, bool) {
	if m.report == nil {
		return 0, false
	}
	switch m.resultsTab {
	case 1: // grouped: cursor walks category lines
		cats := resultCategories(m.report.Results.Workloads)
		if m.resultsCur >= len(cats) {
			return 0, false
		}
		bodyLine := 0
		for i, cat := range cats {
			if i == m.resultsCur {
				return resultsBodyStartLine + bodyLine, true
			}
			bodyLine++
			if m.expanded[cat] {
				for _, w := range m.report.Results.Workloads {
					if w.Category == cat {
						bodyLine++
					}
				}
			}
		}
		return 0, false
	default: // flat: header + separator + one row per workload
		if m.resultsCur >= len(m.report.Results.Workloads) {
			return 0, false
		}
		return resultsBodyStartLine + 2 + m.resultsCur, true
	}
}

const resultsBodyStartLine = 5

// reportFileName builds the timestamped save name so repeated saves never
// silently overwrite each other.
func reportFileName(now time.Time) string {
	return fmt.Sprintf("vmbench-report-%s.json", now.Format("20060102-150405"))
}

// saveReportFile writes the report to an absolute path, refusing to
// overwrite an existing file; the returned message carries the full path for
// the toast.
func saveReportFile(name string, report *vmbench.Report) saveDoneMsg {
	path, err := filepath.Abs(name)
	if err != nil {
		path = name
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return saveDoneMsg{path: path, err: fmt.Errorf("%s", i18n.Tf("tui.toast.saveExists", map[string]any{"Path": path}))}
		}
		return saveDoneMsg{path: path, err: err}
	}
	defer f.Close()
	if err := gbreport.WriteJSON(f, *report); err != nil {
		return saveDoneMsg{path: path, err: err}
	}
	return saveDoneMsg{path: path}
}

func saveReportCmd(report *vmbench.Report) tea.Cmd {
	return func() tea.Msg {
		return saveReportFile(reportFileName(time.Now()), report)
	}
}

func viewResults(m Model) string {
	if m.report == nil {
		return lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  " + i18n.T("tui.results.noResults"))
	}

	t := theme.Active
	width := m.width
	doc := *m.report

	total, okCount, failCount := resultStats(doc.Results.Workloads, doc.Extensions.Workloads)

	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.results.title"))
	stats := lipgloss.NewStyle().Foreground(t.Muted).Render(
		i18n.Tf("tui.results.statsPrefix", map[string]any{"Count": total}),
	) +
		lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(i18n.Tf("tui.results.okCount", map[string]any{"Count": okCount})) +
		lipgloss.NewStyle().Foreground(t.Muted).Render("  •  ") +
		lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render(i18n.Tf("tui.results.failCount", map[string]any{"Count": failCount}))

	tabs := comp.Tabs(width, []comp.Tab{
		{Label: i18n.T("tui.results.tabCards")},
		{Label: i18n.T("tui.results.tabGrouped")},
		{Label: i18n.T("tui.results.tabFlat")},
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
		extHeader := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(i18n.T("tui.results.extensions"))
		parts = append(parts, "", extHeader, viewWorkloadRows(doc.Extensions.Workloads, -1, width))
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
			metric = lipgloss.NewStyle().Foreground(t.Danger).Render(i18n.T("tui.results.error"))
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
		Subtitle: i18n.Tf("tui.results.workloadCount", map[string]any{"Count": len(g.workloads)}),
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
		{Title: i18n.T("tui.col.workload"), Width: comp.ColWidth(i18n.T("tui.col.workload"), 22)},
		{Title: i18n.T("tui.col.category"), Width: comp.ColWidth(i18n.T("tui.col.category"), 12)},
		{Title: i18n.T("tui.col.time"), Width: comp.ColWidth(i18n.T("tui.col.time"), 10), Align: lipgloss.Right},
		{Title: i18n.T("tui.col.throughput"), Width: comp.ColWidth(i18n.T("tui.col.throughput"), 18), Align: lipgloss.Right},
		{Title: i18n.T("tui.col.latency"), Width: comp.ColWidth(i18n.T("tui.col.latency"), 10), Align: lipgloss.Right},
		{Title: i18n.T("tui.col.status"), Width: comp.ColWidth(i18n.T("tui.col.status"), 8)},
	}
	if width < 100 {
		cols = []comp.TableColumn{
			{Title: i18n.T("tui.col.workload"), Width: comp.ColWidth(i18n.T("tui.col.workload"), 20)},
			{Title: i18n.T("tui.col.time"), Width: comp.ColWidth(i18n.T("tui.col.time"), 10), Align: lipgloss.Right},
			{Title: i18n.T("tui.col.throughput"), Width: comp.ColWidth(i18n.T("tui.col.throughput"), 18), Align: lipgloss.Right},
			{Title: i18n.T("tui.col.status"), Width: comp.ColWidth(i18n.T("tui.col.status"), 8)},
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
		{Title: i18n.T("tui.col.workload"), Width: comp.ColWidth(i18n.T("tui.col.workload"), 22)},
		{Title: i18n.T("tui.col.time"), Width: comp.ColWidth(i18n.T("tui.col.time"), 10), Align: lipgloss.Right},
		{Title: i18n.T("tui.col.throughput"), Width: comp.ColWidth(i18n.T("tui.col.throughput"), 18), Align: lipgloss.Right},
		{Title: i18n.T("tui.col.status"), Width: comp.ColWidth(i18n.T("tui.col.status"), 8)},
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

	// Narrow terminals drop the latency column (like the flat table) and let
	// the name column absorb the slack so the OK/FAIL status stays visible.
	nameW, timeW, thrW, latW := 22, 10, 18, 10
	if m.width < 100 {
		latW = 0
		if w := m.width - 2 - (10 + 18 + 8 + 3 + 4); w < nameW {
			nameW = w
		}
		if nameW < 12 {
			nameW = 12
		}
	}

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
				row := "    " +
					lipgloss.NewStyle().Width(nameW).Foreground(t.Fg).Render(truncStr(w.Name, nameW)) + " " +
					lipgloss.NewStyle().Width(timeW).Align(lipgloss.Right).Foreground(t.Fg).Render(tuiTime(w.Result)) + " " +
					lipgloss.NewStyle().Width(thrW).Align(lipgloss.Right).Foreground(t.Fg).Render(tuiThroughput(w.Result))
				if latW > 0 {
					row += " " + lipgloss.NewStyle().Width(latW).Align(lipgloss.Right).Foreground(t.Fg).Render(tuiLatency(w.Result))
				}
				b.WriteString(row + " " + tuiStatusText(w.Result) + "\n")
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

// formatLatencyP99NS renders a nanosecond latency with a human-readable unit.
func formatLatencyP99NS(ns float64) string {
	switch {
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2f µs", ns/1e3)
	default:
		return fmt.Sprintf("%.2f ns", ns)
	}
}

func tuiStatusText(result *gbreport.ResultEntry) string {
	t := theme.Active
	if result == nil {
		return lipgloss.NewStyle().Foreground(t.Muted).Render("—")
	}
	if strings.TrimSpace(result.Error) != "" {
		return lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render(i18n.T("tui.results.failLabel"))
	}
	return lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(i18n.T("tui.results.okLabel"))
}
