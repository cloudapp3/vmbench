package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/checkupcompare"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// pickerLimit caps how many history records are listed: List() re-validates
// every record, so both listing and rendering stay bounded.
const pickerLimit = 50

type pickerMode int

const (
	pickerList pickerMode = iota
	pickerManual
)

type pickerState struct {
	mode       pickerMode
	cursor     int
	records    []history.Record
	loading    bool
	err        error
	a, b       int // selected record indices; -1 = none
	pathA      textinput.Model
	pathB      textinput.Model
	inputFocus byte // 0 none, 'a', 'b'
}

func newPickerState(pathA, pathB string) pickerState {
	a := textinput.New()
	a.Prompt = "A > "
	a.SetValue(pathA)
	b := textinput.New()
	b.Prompt = "B > "
	b.SetValue(pathB)
	return pickerState{a: -1, b: -1, pathA: a, pathB: b}
}

type historyListMsg struct {
	records []history.Record
	err     error
}

type checkupCompareMsg struct {
	text string
	err  error
}

type recordViewMsg struct {
	kind    history.Kind
	run     *gbreport.Document
	checkup *checkup.CheckupReport
	err     error
}

func loadHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		store, err := history.Open("")
		if err != nil {
			return historyListMsg{err: err}
		}
		records, err := store.List()
		if err != nil {
			return historyListMsg{err: err}
		}
		if len(records) > pickerLimit {
			records = records[:pickerLimit]
		}
		return historyListMsg{records: records}
	}
}

// compareRecordsCmd routes by record kind: run reports load into the delta
// table, checkup reports render through checkupcompare.
func compareRecordsCmd(a, b history.Record) tea.Cmd {
	return func() tea.Msg {
		if a.Kind != b.Kind {
			return checkupCompareMsg{err: fmt.Errorf("%s", i18n.T("tui.compare.mixedKinds"))}
		}
		if a.Kind == history.KindCheckup {
			var buf bytes.Buffer
			if err := checkupcompare.WriteCompare(&buf, [][]byte{a.Report, b.Report}); err != nil {
				return checkupCompareMsg{err: err}
			}
			return checkupCompareMsg{text: buf.String()}
		}
		docs, err := loadCompareDocsFiles(a.Report, b.Report)
		if err != nil {
			return checkupCompareMsg{err: err}
		}
		return compareLoadedMsg{docs: docs}
	}
}

// loadCompareDocsFiles parses compare documents from stored raw JSON.
func loadCompareDocsFiles(a, b []byte) ([]gbreport.Document, error) {
	docs := make([]gbreport.Document, 2)
	for i, data := range [][]byte{a, b} {
		if err := json.Unmarshal(data, &docs[i]); err != nil {
			return nil, err
		}
	}
	return docs, nil
}

// viewRecordCmd loads a single history record for browsing in the results
// pages.
func viewRecordCmd(rec history.Record) tea.Cmd {
	return func() tea.Msg {
		if rec.Kind == history.KindCheckup {
			var rep checkup.CheckupReport
			if err := json.Unmarshal(rec.Report, &rep); err != nil {
				return recordViewMsg{err: err}
			}
			return recordViewMsg{kind: rec.Kind, checkup: &rep}
		}
		var doc gbreport.Document
		if err := json.Unmarshal(rec.Report, &doc); err != nil {
			return recordViewMsg{err: err}
		}
		return recordViewMsg{kind: rec.Kind, run: &doc}
	}
}

func updateComparePicker(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.picker
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.inputFocus != 0 {
			return pickerTextInput(m, msg)
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
		case "enter", " ":
			if len(s.records) > 0 {
				s.togglePick()
			}
		case "c":
			if s.a < 0 || s.b < 0 {
				return pickerToast(m, i18n.T("tui.compare.needTwo"))
			}
			m.compareKind = string(s.records[s.a].Kind)
			m.compareLoading = true
			m.checkupCompareText = ""
			return m, compareRecordsCmd(s.records[s.a], s.records[s.b])
		case "v":
			if len(s.records) > 0 {
				return m, viewRecordCmd(s.records[s.cursor])
			}
		case "m":
			s.mode = pickerManual
			s.inputFocus = 'a'
			s.pathA.Focus()
			s.pathB.Blur()
		case "tab":
			if s.mode == pickerManual {
				return pickerCycleInput(m)
			}
		case "esc":
			if s.mode == pickerManual {
				s.mode = pickerList
				s.inputFocus = 0
				s.pathA.Blur()
				s.pathB.Blur()
			} else {
				m.page = pageDashboard
			}
		case "q":
			return m, tea.Quit
		}
		m.picker = s
		return followFocus(m), nil
	}
	return m, nil
}

// togglePick selects the cursor row as A or B; a second pick replaces the
// older selection.
func (s *pickerState) togglePick() {
	cur := s.cursor
	if s.a == cur {
		s.a = -1
		return
	}
	if s.b == cur {
		s.b = -1
		return
	}
	switch {
	case s.a < 0:
		s.a = cur
	case s.b < 0:
		s.b = cur
	default:
		s.a = s.b
		s.b = cur
	}
}

func pickerCycleInput(m Model) (tea.Model, tea.Cmd) {
	s := m.picker
	if s.inputFocus == 'a' {
		s.inputFocus = 'b'
		s.pathA.Blur()
		s.pathB.Focus()
	} else {
		s.inputFocus = 'a'
		s.pathB.Blur()
		s.pathA.Focus()
	}
	m.picker = s
	return m, nil
}

func pickerTextInput(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.picker
	switch msg.String() {
	case "enter":
		a, b := strings.TrimSpace(s.pathA.Value()), strings.TrimSpace(s.pathB.Value())
		if a == "" || b == "" {
			return pickerToast(m, i18n.T("tui.compare.needTwo"))
		}
		m.compareA, m.compareB = a, b
		m.compareKind = string(history.KindRun)
		m.compareLoading = true
		m.checkupCompareText = ""
		return m, loadCompareCmd(a, b)
	case "esc":
		s.inputFocus = 0
		s.pathA.Blur()
		s.pathB.Blur()
		m.picker = s
		return m, nil
	case "tab":
		return pickerCycleInput(m)
	}
	var cmd tea.Cmd
	if s.inputFocus == 'a' {
		s.pathA, cmd = s.pathA.Update(msg)
	} else {
		s.pathB, cmd = s.pathB.Update(msg)
	}
	m.picker = s
	return m, cmd
}

func pickerToast(m Model, text string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.toast, cmd = comp.ShowToast(text, comp.ToastWarn, 3*time.Second)
	return m, cmd
}

func viewComparePicker(m Model) string {
	t := theme.Active
	s := m.picker
	width := m.width

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.compare.pickerTitle"))
	desc := lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.compare.pickerDesc"))

	if s.mode == pickerManual {
		body := s.pathA.View() + "\n" + s.pathB.View() + "\n\n" +
			lipgloss.NewStyle().Foreground(t.Subtle).Render(i18n.T("tui.compare.manualHint"))
		card := comp.Card{Title: i18n.T("tui.compare.manual"), Body: body, Accent: t.Accent, Width: width - 4}.Render()
		parts := []string{title, desc, "", card}
		if m.toast.Active() {
			parts = append(parts, "", m.toast.Render(width-4))
		}
		return strings.Join(parts, "\n")
	}

	if s.loading {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Muted).Render("  " + i18n.T("tui.compare.loadingHistory")),
		}, "\n")
	}
	if s.err != nil {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Danger).Render("  " + i18n.Tf("tui.compare.historyError", map[string]any{"Err": s.err.Error()})),
		}, "\n")
	}
	if len(s.records) == 0 {
		return strings.Join([]string{title, desc, "",
			lipgloss.NewStyle().Foreground(t.Muted).Render("  " + i18n.T("tui.compare.noHistory")),
		}, "\n")
	}

	cols := []comp.TableColumn{
		{Title: "", Width: 3},
		{Title: i18n.T("tui.compare.col.time"), Width: comp.ColWidth(i18n.T("tui.compare.col.time"), 14)},
		{Title: i18n.T("tui.compare.col.kind"), Width: comp.ColWidth(i18n.T("tui.compare.col.kind"), 8)},
		{Title: i18n.T("tui.compare.col.tag"), Width: comp.ColWidth(i18n.T("tui.compare.col.tag"), 14)},
		{Title: i18n.T("tui.compare.col.id"), Width: comp.ColWidth(i18n.T("tui.compare.col.id"), 22)},
	}
	rows := make([]comp.TableRow, 0, len(s.records))
	for i, rec := range s.records {
		marker := "·"
		if i == s.a {
			marker = "A"
		} else if i == s.b {
			marker = "B"
		}
		rows = append(rows, comp.TableRow{
			Cells: []string{
				marker,
				rec.ReportTime.Format("01-02 15:04"),
				string(rec.Kind),
				truncStr(firstStr(rec.Tag, "—"), 14),
				truncStr(rec.ID, 22),
			},
			Highlight: i == s.cursor,
		})
	}
	table := comp.RenderTable(cols, rows)
	parts := []string{title, desc, "", table}

	var picked []string
	for label, idx := range map[string]int{"A": s.a, "B": s.b} {
		if idx >= 0 && idx < len(s.records) {
			picked = append(picked, fmt.Sprintf("%s: %s", label, truncStr(s.records[idx].ID, 24)))
		}
	}
	status := strings.Join(picked, "   ")
	if status == "" {
		status = i18n.T("tui.compare.pickerHint")
	}
	parts = append(parts, "", lipgloss.NewStyle().Foreground(t.Secondary).Render(truncStr(status, width-4)))
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

// pickerFocusedLine mirrors viewComparePicker's layout: title, desc, blank,
// then the table's header + separator rows.
func pickerFocusedLine(m Model) (int, bool) {
	if m.picker.mode != pickerList || len(m.picker.records) == 0 {
		return 0, false
	}
	if m.picker.cursor >= len(m.picker.records) {
		return 0, false
	}
	return pickerTableStartLine + 2 + m.picker.cursor, true
}

const pickerTableStartLine = 3
