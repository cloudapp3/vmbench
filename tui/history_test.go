package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/i18n"
)

// historyTestModel loads a fixture store and routes the load result through
// Update on the history page, exercising the historyListMsg routing.
func historyTestModel(t *testing.T, runs, checkups int) Model {
	t.Helper()
	fixtureStore(t, runs, checkups)
	m := scrollTestModel(t, pageHistory, nil)
	m.history.loading = true
	updated, _ := m.Update(loadHistoryCmd()())
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.history.loading || um.history.err != nil {
		t.Fatalf("history load failed: loading=%v err=%v", um.history.loading, um.history.err)
	}
	return um
}

func TestHistoryListsRecords(t *testing.T) {
	m := historyTestModel(t, 2, 1)
	if len(m.history.records) != 3 {
		t.Fatalf("records = %d, want 3 (2 run + 1 checkup)", len(m.history.records))
	}
	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	if !strings.Contains(view, "run") || !strings.Contains(view, "checkup") {
		t.Fatalf("history page should show kind column:\n%s", view)
	}
	if !strings.Contains(view, i18n.T("tui.history.title")) {
		t.Fatalf("history page missing title:\n%s", view)
	}
}

func TestHistoryRenderZhCN(t *testing.T) {
	i18n.SetLang("zh-CN")
	t.Cleanup(func() { i18n.SetLang("en") })
	m := historyTestModel(t, 1, 1)
	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	if !strings.Contains(view, i18n.T("tui.history.title")) || !strings.Contains(view, "时间") {
		t.Fatalf("zh-CN history page missing translated strings:\n%s", view)
	}
}

func TestHistoryEmptyState(t *testing.T) {
	m := historyTestModel(t, 0, 0)
	if len(m.history.records) != 0 {
		t.Fatalf("records = %d, want 0", len(m.history.records))
	}
	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	if !strings.Contains(view, i18n.T("tui.history.empty")) || !strings.Contains(view, i18n.T("tui.history.emptyHint")) {
		t.Fatalf("empty history page should show empty + hint:\n%s", view)
	}
}

func TestHistoryErrorState(t *testing.T) {
	errBoom := errors.New("boom")
	m := scrollTestModel(t, pageHistory, nil)
	m.history.loading = true
	updated, _ := m.Update(historyListMsg{err: errBoom})
	um := updated.(Model)
	if !errors.Is(um.history.err, errBoom) || um.history.loading {
		t.Fatalf("error not stored on history state: %+v", um.history)
	}
	view := um.View()
	assertRenderBounds(t, view, 80, 24)
	if !strings.Contains(view, "boom") {
		t.Fatalf("error state should render the error:\n%s", view)
	}
}

func TestHistoryOpensRunReportAndReturns(t *testing.T) {
	m := historyTestModel(t, 2, 0)
	// records are newest-first runs; move off row 0 to be safe against
	// ordering ties, then open with enter.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	msg := cmd()
	view, ok := msg.(recordViewMsg)
	if !ok || view.run == nil {
		t.Fatalf("enter should return a run recordViewMsg, got %#v", msg)
	}

	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.page != pageResults || m.report == nil {
		t.Fatalf("run record should open results page, page = %d", m.page)
	}
	if m.reportFrom != pageHistory {
		t.Fatalf("reportFrom = %d, want pageHistory", m.reportFrom)
	}

	// esc returns to the history page (records kept), then to the dashboard.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.page != pageHistory || m.reportFrom != pageDashboard {
		t.Fatalf("esc should return to history page, page = %d reportFrom = %d", m.page, m.reportFrom)
	}
	if len(m.history.records) != 2 {
		t.Fatalf("history records should survive a view round trip, got %d", len(m.history.records))
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.page != pageDashboard {
		t.Fatalf("esc on history should return to dashboard, page = %d", m.page)
	}
}

func TestHistoryOpensCheckupReport(t *testing.T) {
	// Fixture adds runs first, checkups last; newest-first listing puts the
	// checkup on row 0.
	m := historyTestModel(t, 1, 1)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	msg := cmd()
	if view, ok := msg.(recordViewMsg); !ok || view.checkup == nil {
		t.Fatalf("enter should return a checkup recordViewMsg, got %#v", msg)
	}

	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.page != pageCheckupResults || m.checkupReport == nil {
		t.Fatalf("checkup record should open checkup results, page = %d", m.page)
	}
	if m.reportFrom != pageHistory {
		t.Fatalf("reportFrom = %d, want pageHistory", m.reportFrom)
	}
}

func TestHistoryFocusedLineMatchesRender(t *testing.T) {
	m := historyTestModel(t, 20, 1)
	m.history.cursor = 4
	line, ok := historyFocusedLine(m)
	if !ok {
		t.Fatal("expected focused line")
	}
	rows := strings.Split(pageContent(m), "\n")
	if line >= len(rows) {
		t.Fatalf("focused line %d beyond content %d", line, len(rows))
	}
	if !strings.Contains(rows[line], m.history.records[4].ID[:10]) {
		t.Fatalf("focused line %d = %q, want record row %q", line, rows[line], m.history.records[4].ID)
	}
}

func TestHistoryDashboardEntry(t *testing.T) {
	fixtureStore(t, 1, 1)
	m := scrollTestModel(t, pageDashboard, nil)

	row := 0
	for i, item := range menuItems() {
		if item.mode == "history" {
			row = i
		}
	}
	m.cursor = row
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.page != pageHistory || !um.history.loading {
		t.Fatalf("menu enter should open loading history page, page = %d loading = %v", um.page, um.history.loading)
	}
	if cmd == nil {
		t.Fatal("menu enter should issue loadHistoryCmd")
	}

	updated, _ = um.Update(cmd())
	um = updated.(Model)
	if len(um.history.records) != 2 || um.history.loading {
		t.Fatalf("load should fill records, got %d loading=%v", len(um.history.records), um.history.loading)
	}
}
