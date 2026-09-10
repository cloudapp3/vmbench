package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// historyState is the report-history browsing page. It shares the picker's
// loading path (loadHistoryCmd/historyListMsg) and record viewer, minus the
// A/B pick and manual-path machinery.
type historyState struct {
	cursor  int
	records []history.Record
	loading bool
	err     error
}

func updateHistory(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.history
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(s.records)-1 {
				s.cursor++
			}
		case "enter", "v":
			if len(s.records) > 0 {
				return m, viewRecordCmd(s.records[s.cursor])
			}
		case "esc":
			m.page = pageDashboard
		case "q":
			return m, tea.Quit
		}
		m.history = s
		return followFocus(m), nil
	}
	return m, nil
}

func viewHistory(m Model) string {
	t := theme.Active
	s := m.history
	width := m.width

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.history.title"))
	desc := lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.history.desc"))

	if s.loading {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Muted).Render("  " + i18n.T("tui.history.loading")),
		}, "\n")
	}
	if s.err != nil {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Danger).Render("  " + i18n.Tf("tui.history.error", map[string]any{"Err": s.err.Error()})),
		}, "\n")
	}
	if len(s.records) == 0 {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Muted).Render("  " + i18n.T("tui.history.empty")),
			lipgloss.NewStyle().Foreground(t.Subtle).Render("  " + i18n.T("tui.history.emptyHint")),
		}, "\n")
	}

	cols := []comp.TableColumn{
		{Title: i18n.T("tui.history.col.time"), Width: comp.ColWidth(i18n.T("tui.history.col.time"), 14)},
		{Title: i18n.T("tui.history.col.kind"), Width: comp.ColWidth(i18n.T("tui.history.col.kind"), 8)},
		{Title: i18n.T("tui.history.col.tag"), Width: comp.ColWidth(i18n.T("tui.history.col.tag"), 14)},
		{Title: i18n.T("tui.history.col.id"), Width: comp.ColWidth(i18n.T("tui.history.col.id"), 22)},
	}
	rows := make([]comp.TableRow, 0, len(s.records))
	for i, rec := range s.records {
		rows = append(rows, comp.TableRow{
			Cells: []string{
				rec.ReportTime.Format("01-02 15:04"),
				string(rec.Kind),
				truncStr(firstStr(rec.Tag, "—"), 14),
				truncStr(rec.ID, 22),
			},
			Highlight: i == s.cursor,
		})
	}
	table := comp.RenderTable(cols, rows)
	count := lipgloss.NewStyle().Foreground(t.Secondary).Render(
		truncStr(i18n.Tf("tui.history.count", map[string]any{"Count": len(s.records)}), width-4))
	parts := []string{title, desc, "", table, "", count}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

// historyFocusedLine mirrors viewHistory's layout: title, desc, blank, then
// the table's header + separator rows.
func historyFocusedLine(m Model) (int, bool) {
	if len(m.history.records) == 0 || m.history.cursor >= len(m.history.records) {
		return 0, false
	}
	return historyTableStartLine + 2 + m.history.cursor, true
}

const historyTableStartLine = 3
