package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func TestClipLines(t *testing.T) {
	content := strings.Join([]string{"a", "b", "c", "d", "e"}, "\n")

	cases := []struct {
		name           string
		offset, height int
		want           string
	}{
		{"zero offset", 0, 3, "a\nb\nc"},
		{"middle", 1, 2, "b\nc"},
		{"past end", 10, 3, ""},
		{"negative offset clamps to zero", -2, 2, "a\nb"},
		{"short tail", 3, 10, "d\ne"},
		{"zero height", 0, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clipLines(content, tc.offset, tc.height)
			if got != tc.want {
				t.Fatalf("clipLines(%q, %d, %d) = %q, want %q", content, tc.offset, tc.height, got, tc.want)
			}
		})
	}
}

func TestClipLinesCJKWidth(t *testing.T) {
	content := strings.Join([]string{"系统信息", "CPU 型号", "c"}, "\n")
	got := clipLines(content, 1, 1)
	if got != "CPU 型号" {
		t.Fatalf("clipLines kept wrong line: %q", got)
	}
	if w := lipgloss.Width(got); w != 8 {
		t.Fatalf("visible width = %d, want 8", w)
	}
}

// tallReport builds a report with enough workloads to overflow a 24-line
// terminal in the flat view.
func tallReport(n int) *gbreport.Document {
	doc := &gbreport.Document{Timestamp: time.Now()}
	for i := 0; i < n; i++ {
		doc.Results.Workloads = append(doc.Results.Workloads, gbreport.WorkloadEntry{
			Name:     "Workload-Names-Long",
			Category: "CPU",
			Result: &gbreport.ResultEntry{
				Iterations:       3,
				MedianMS:         1010,
				ThroughputPerSec: 532,
				ThroughputUnit:   "events/sec",
			},
		})
	}
	return doc
}

func scrollTestModel(t *testing.T, page page, setup func(*Model)) Model {
	t.Helper()
	m := NewModel("", "")
	m.page = page
	m.width = 80
	m.height = 24
	m.sysInfo = sysinfo.SystemInfo{}
	if setup != nil {
		setup(&m)
	}
	return m
}

func TestResultsLongReportFitsViewport(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = tallReport(40)
	})
	// At 80 cols the footer may drop the position hint to stay in budget;
	// the content must still fit the terminal exactly.
	assertRenderBounds(t, m.View(), 80, 24)

	m.width = 120
	if view := m.View(); !strings.Contains(view, "1/") {
		t.Fatalf("expected scroll position indicator in footer at 120 cols:\n%s", view)
	}
}

func TestScrollKeysClampAndReset(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = tallReport(40)
	})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.scroll.offset <= 0 {
		t.Fatalf("End should scroll to bottom, offset = %d", um.scroll.offset)
	}

	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyHome})
	um, ok = updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.scroll.offset != 0 {
		t.Fatalf("Home should reset offset, got %d", um.scroll.offset)
	}

	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	um, ok = updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.scroll.offset <= 0 {
		t.Fatalf("PgDn should move offset down, got %d", um.scroll.offset)
	}
}

func TestScrollOffsetResetsOnPageChange(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = tallReport(40)
	})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	um := updated.(Model)
	if um.scroll.offset == 0 {
		t.Fatal("precondition: End should have scrolled")
	}

	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	um = updated.(Model)
	if um.page != pageDashboard {
		t.Fatalf("esc should return to dashboard, page = %d", um.page)
	}
	if um.scroll.page != pageDashboard || um.scroll.offset != 0 {
		t.Fatalf("scroll state should reset on page change, got page=%d offset=%d", um.scroll.page, um.scroll.offset)
	}
}

func TestFollowFocusKeepsCursorVisible(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = tallReport(40)
	})

	// Walk the cursor to the last row.
	var um Model = m
	for i := 0; i < 39; i++ {
		updated, _ := um.Update(tea.KeyMsg{Type: tea.KeyDown})
		um = updated.(Model)
	}

	content := pageContent(um)
	vh := contentViewportHeight(um)
	line, ok := resultsFocusedLine(um)
	if !ok {
		t.Fatal("expected focused line for flat results tab")
	}
	if lipgloss.Height(content) < vh {
		t.Fatalf("fixture should overflow viewport: content %d lines, viewport %d", lipgloss.Height(content), vh)
	}
	if line < um.scroll.offset || line > um.scroll.offset+vh-1 {
		t.Fatalf("focused line %d outside viewport [%d, %d]", line, um.scroll.offset, um.scroll.offset+vh-1)
	}

	// The highlighted row must actually be visible in the rendered view.
	view := um.View()
	rows := strings.Split(view, "\n")
	visible := false
	for _, row := range rows {
		if strings.Contains(row, "Workload-Names-Long") && strings.Contains(row, "532") {
			visible = true
		}
	}
	if !visible {
		t.Fatalf("highlighted row not visible after scrolling with cursor:\n%s", view)
	}
}

func TestResultsFocusedLineMatchesRender(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = tallReport(6)
	})
	m.resultsCur = 3
	line, ok := resultsFocusedLine(m)
	if !ok {
		t.Fatal("expected focused line")
	}
	rows := strings.Split(pageContent(m), "\n")
	if line >= len(rows) {
		t.Fatalf("focused line %d beyond content %d lines", line, len(rows))
	}
	// Row 3's name is uniform in the fixture; verify the separator/header
	// accounting by checking the line holds a workload row, not the header.
	if !strings.Contains(rows[line], "Workload-Names-Long") {
		t.Fatalf("focused line %d = %q, want a workload row", line, rows[line])
	}
}
