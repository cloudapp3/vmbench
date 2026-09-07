package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
)

// fixtureStore writes a temp history dir with one run and one suite record
// and points VMBENCH_HISTORY_DIR at it.
func fixtureStore(t *testing.T, runCount, suiteCount int) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("VMBENCH_HISTORY_DIR", dir)

	store, err := history.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < runCount; i++ {
		doc := gbreport.Document{
			SchemaVersion: 2,
			Timestamp:     time.Now().Add(-time.Duration(i+1) * time.Hour),
		}
		doc.Results.Workloads = []gbreport.WorkloadEntry{{
			Name:     "CPU Single-Core (sysbench)",
			Category: "CPU",
			Result:   &gbreport.ResultEntry{Iterations: 3, MedianMS: 1010, ThroughputPerSec: 532, ThroughputUnit: "events/sec"},
		}}
		data, _ := json.Marshal(doc)
		if _, err := store.Add(data, ""); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < suiteCount; i++ {
		rep := suite.SuiteReport{SchemaVersion: 2, ReportKind: "suite"}
		rep.Config.Preset = "quick"
		data, _ := json.Marshal(rep)
		if _, err := store.Add(data, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func pickerTestModel(t *testing.T, records int) Model {
	t.Helper()
	fixtureStore(t, records, 1)
	m := scrollTestModel(t, pageComparePicker, nil)
	msg := loadHistoryCmd()()
	list, ok := msg.(historyListMsg)
	if !ok || list.err != nil {
		t.Fatalf("loadHistoryCmd returned %#v", msg)
	}
	m.picker.records = list.records
	m.picker.loading = false
	return m
}

func TestPickerListsRecords(t *testing.T) {
	m := pickerTestModel(t, 2)
	if len(m.picker.records) != 3 {
		t.Fatalf("records = %d, want 3 (2 run + 1 suite)", len(m.picker.records))
	}
	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	if !strings.Contains(view, "run") || !strings.Contains(view, "suite") {
		t.Fatalf("picker should show kind column:\n%s", view)
	}
}

func TestPickerPickTwoAndCompareRuns(t *testing.T) {
	m := pickerTestModel(t, 2)
	// records are newest-first: [0] suite, [1..2] runs.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if m.picker.a != 1 || m.picker.b != 2 {
		t.Fatalf("picks = %d/%d, want 1/2", m.picker.a, m.picker.b)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("c should return a compare cmd")
	}
	msg := cmd()
	loaded, ok := msg.(compareLoadedMsg)
	if !ok {
		t.Fatalf("two run records should load delta docs, got %T", msg)
	}
	if len(loaded.docs) != 2 || loaded.err != nil {
		t.Fatalf("loaded = %+v err = %v", loaded.docs, loaded.err)
	}
}

func TestPickerRejectsMixedKinds(t *testing.T) {
	m := pickerTestModel(t, 1)
	// records: newest first → suite(0), run(1)
	if m.picker.records[0].Kind != history.KindSuite || m.picker.records[1].Kind != history.KindRun {
		t.Fatalf("fixture kinds = %s/%s", m.picker.records[0].Kind, m.picker.records[1].Kind)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	msg := cmd()
	got, ok := msg.(suiteCompareMsg)
	if !ok || got.err == nil {
		t.Fatalf("mixed kinds must be rejected, got %#v", msg)
	}
	if !strings.Contains(got.err.Error(), i18n.T("tui.compare.mixedKinds")) {
		t.Fatalf("rejection message = %q", got.err.Error())
	}
}

func TestPickerViewRecordRoundTrip(t *testing.T) {
	m := pickerTestModel(t, 2)
	// Move to the suite record (newest first → index 0 is suite).
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(Model)
	msg := cmd()
	view, ok := msg.(recordViewMsg)
	if !ok {
		t.Fatalf("v should return recordViewMsg, got %T", msg)
	}
	if view.kind != history.KindSuite || view.suite == nil {
		t.Fatalf("view msg = kind %s suite %v", view.kind, view.suite)
	}

	// Feed it through Update as the program would.
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.page != pageSuiteResults || m.suiteReport == nil {
		t.Fatalf("suite record should open suite results, page = %d", m.page)
	}
	if !m.reportCameFromPicker {
		t.Fatal("reportCameFromPicker should be set")
	}

	// esc returns to the picker, not the dashboard.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.page != pageComparePicker || m.reportCameFromPicker {
		t.Fatalf("esc should return to picker, page = %d fromPicker = %v", m.page, m.reportCameFromPicker)
	}
}

func TestPickerManualPaths(t *testing.T) {
	dir := t.TempDir()
	doc := gbreport.Document{SchemaVersion: 2, Timestamp: time.Now()}
	doc.Results.Workloads = []gbreport.WorkloadEntry{{
		Name: "CPU Single-Core (sysbench)", Category: "CPU",
		Result: &gbreport.ResultEntry{MedianMS: 1010, ThroughputPerSec: 532, ThroughputUnit: "events/sec"},
	}}
	data, _ := json.Marshal(doc)
	path := filepath.Join(dir, "a.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	m := scrollTestModel(t, pageComparePicker, nil)
	m.picker.mode = pickerManual
	m.picker.inputFocus = 'a'
	m.picker.pathA.Focus()

	// '?' must be typed, not open help.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)
	if m.page != pageComparePicker {
		t.Fatalf("? leaked into help from picker input, page = %d", m.page)
	}
	if !strings.Contains(m.picker.pathA.Value(), "?") {
		t.Fatalf("rune should reach path input, got %q", m.picker.pathA.Value())
	}

	m.picker.pathA.SetValue(path)
	m.picker.pathB.SetValue(path)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	msg := cmd()
	loaded, ok := msg.(compareLoadedMsg)
	if !ok || loaded.err != nil || len(loaded.docs) != 2 {
		t.Fatalf("manual compare failed: %#v", msg)
	}
}

func TestPickerFocusedLineMatchesRender(t *testing.T) {
	m := pickerTestModel(t, 20)
	m.picker.cursor = 4
	line, ok := pickerFocusedLine(m)
	if !ok {
		t.Fatal("expected focused line")
	}
	rows := strings.Split(pageContent(m), "\n")
	if line >= len(rows) {
		t.Fatalf("focused line %d beyond content %d", line, len(rows))
	}
	if !strings.Contains(rows[line], m.picker.records[4].ID[:10]) {
		t.Fatalf("focused line %d = %q, want record row %q", line, rows[line], m.picker.records[4].ID)
	}
}
