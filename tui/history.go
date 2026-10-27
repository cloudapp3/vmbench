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
	cursor        int
	records       []history.Record
	total         int // records in the store; > len(records) when the list is truncated
	loading       bool
	err           error
	confirmDelete bool
}

// historyNeedsLoad reports whether the page should (re)fetch records: no
// successful load yet, or the previous one failed so re-entry retries.
func historyNeedsLoad(s historyState) bool {
	return !s.loading && (s.records == nil || s.err != nil)
}

type historyDeletedMsg struct {
	id  string
	err error
}

// deleteHistoryCmd removes one record from the store off the render path.
func deleteHistoryCmd(id string) tea.Cmd {
	return func() tea.Msg {
		store, err := history.Open("")
		if err != nil {
			return historyDeletedMsg{id: id, err: err}
		}
		if err := store.Delete(id); err != nil {
			return historyDeletedMsg{id: id, err: err}
		}
		return historyDeletedMsg{id: id}
	}
}

func updateHistory(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.history
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.confirmDelete {
			// While the delete confirmation is up, only y/n/esc act.
			switch strings.ToLower(msg.String()) {
			case "y":
				if len(s.records) > 0 {
					m.history = s
					return m, deleteHistoryCmd(s.records[s.cursor].ID)
				}
				s.confirmDelete = false
			case "n", "esc":
				s.confirmDelete = false
			default:
				m.history = s
				return m, nil
			}
			m.history = s
			return followFocus(m), nil
		}
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
		case "r":
			s.loading = true
			s.err = nil
			m.history = s
			return m, loadHistoryCmd()
		case "d":
			if len(s.records) > 0 {
				s.confirmDelete = true
			}
		case "esc", "q":
			m.page = pageDashboard
		}
		m.history = s
		return followFocus(m), nil
	}
	return m, nil
}

// historyTableColumns builds the page's columns. The full time/kind/tag/id
// set needs 61 cells; narrower terminals drop tag and let the ID column
// absorb the slack, keeping time and ID (the load-bearing columns) on screen.
func historyTableColumns(width int) (cols []comp.TableColumn, idWidth int, withTag bool) {
	const timeW, kindW, tagW = 14, 8, 14
	budget := width - 4
	idWidth = 22
	withTag = budget >= timeW+kindW+tagW+idWidth+3
	if !withTag {
		idWidth = budget - timeW - kindW - 2
		if idWidth < 12 {
			idWidth = 12
		}
		if idWidth > 30 {
			idWidth = 30
		}
	}
	cols = []comp.TableColumn{
		{Title: i18n.T("tui.history.col.time"), Width: comp.ColWidth(i18n.T("tui.history.col.time"), timeW)},
		{Title: i18n.T("tui.history.col.kind"), Width: comp.ColWidth(i18n.T("tui.history.col.kind"), kindW)},
	}
	if withTag {
		cols = append(cols, comp.TableColumn{Title: i18n.T("tui.history.col.tag"), Width: comp.ColWidth(i18n.T("tui.history.col.tag"), tagW)})
	}
	cols = append(cols, comp.TableColumn{Title: i18n.T("tui.history.col.id"), Width: comp.ColWidth(i18n.T("tui.history.col.id"), idWidth)})
	return cols, idWidth, withTag
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
			lipgloss.NewStyle().Foreground(t.Subtle).Render("  " + i18n.T("tui.hint.retryHint")),
		}, "\n")
	}
	if len(s.records) == 0 {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Muted).Render("  " + i18n.T("tui.history.empty")),
			lipgloss.NewStyle().Foreground(t.Subtle).Render("  " + i18n.T("tui.history.emptyHint")),
		}, "\n")
	}

	cols, idWidth, withTag := historyTableColumns(width)
	rows := make([]comp.TableRow, 0, len(s.records))
	for i, rec := range s.records {
		cells := []string{
			rec.ReportTime.Format("01-02 15:04"),
			string(rec.Kind),
		}
		if withTag {
			cells = append(cells, truncStr(firstStr(rec.Tag, "—"), 14))
		}
		cells = append(cells, truncStr(rec.ID, idWidth))
		rows = append(rows, comp.TableRow{
			Cells:     cells,
			Highlight: i == s.cursor,
		})
	}
	table := comp.RenderTable(cols, rows)

	count := lipgloss.NewStyle().Foreground(t.Secondary).Render(
		truncStr(historyCountText(s), width-4))

	parts := []string{title, desc}
	if s.confirmDelete {
		// The delete prompt rides above the table so it never scrolls out
		// of view; historyTableStartLine accounts for the extra lines.
		prompt := lipgloss.NewStyle().Bold(true).Foreground(t.Danger).Render(
			i18n.Tf("tui.history.deletePrompt", map[string]any{"ID": truncStr(s.records[s.cursor].ID, 32)}))
		parts = append(parts, "", prompt)
	}
	parts = append(parts, "", table, "", count)
	return strings.Join(parts, "\n")
}

// historyCountText reports "N reports", or "latest N of M" when the list was
// truncated to the picker limit.
func historyCountText(s historyState) string {
	if s.total > len(s.records) {
		return i18n.Tf("tui.history.countTruncated", map[string]any{"Shown": len(s.records), "Total": s.total})
	}
	return i18n.Tf("tui.history.count", map[string]any{"Count": len(s.records)})
}

// historyFocusedLine mirrors viewHistory's layout: title, desc, (delete
// prompt + blank), blank, then the table's header + separator rows.
func historyFocusedLine(m Model) (int, bool) {
	if len(m.history.records) == 0 || m.history.cursor >= len(m.history.records) {
		return 0, false
	}
	return historyTableStartLine(m) + 2 + m.history.cursor, true
}

func historyTableStartLine(m Model) int {
	if m.history.confirmDelete {
		return 5 // title, desc, blank, prompt, blank
	}
	return 3 // title, desc, blank
}
