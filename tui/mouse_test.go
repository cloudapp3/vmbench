package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func wheelMsg(down bool) tea.MouseMsg {
	button := tea.MouseButtonWheelUp
	if down {
		button = tea.MouseButtonWheelDown
	}
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: button, X: 10, Y: 10}
}

func clickMsg(x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}
}

func TestMouseWheelScrollsAndClamps(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = tallReport(40)
	})

	updated, _ := m.Update(wheelMsg(true))
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.scroll.offset != 3 {
		t.Fatalf("wheel down should add 3 lines, offset = %d", um.scroll.offset)
	}

	for i := 0; i < 100; i++ {
		updated, _ = um.Update(wheelMsg(true))
		um = updated.(Model)
	}
	max := maxScrollOffset(pageContent(um), contentViewportHeight(um))
	if um.scroll.offset != max {
		t.Fatalf("offset should clamp to %d, got %d", max, um.scroll.offset)
	}

	updated, _ = um.Update(wheelMsg(false))
	um = updated.(Model)
	if um.scroll.offset != max-3 {
		t.Fatalf("wheel up should subtract 3 lines, offset = %d", um.scroll.offset)
	}
}

// TestDashboardMenuRegionMatchesRender pins the hit-test geometry to the
// rendered menu: the row under each menu item must contain that item's label.
func TestDashboardMenuRegionMatchesRender(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := scrollTestModel(t, pageDashboard, nil)
		m.width = width

		top, count, xMax := dashboardMenuRegion(m)
		if count != len(menuItems()) {
			t.Fatalf("width %d: region count %d, want %d", width, count, len(menuItems()))
		}
		if xMax > width {
			t.Fatalf("width %d: xMax %d exceeds terminal", width, xMax)
		}

		rows := strings.Split(pageContent(m), "\n")
		for i, item := range menuItems() {
			line := top + i
			if line >= len(rows) {
				t.Fatalf("width %d: menu row %d beyond rendered content", width, i)
			}
			if !strings.Contains(rows[line], item.label) {
				t.Fatalf("width %d: menu row %d (%q) does not contain label %q", width, i, rows[line], item.label)
			}
		}
	}
}

func TestDashboardClickActivatesMenuItem(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)
	top, _, _ := dashboardMenuRegion(m)

	// Click "Run Suite (VPS Composite)" (row 1).
	updated, _ := m.Update(clickMsg(5, contentOriginY(m)+top+1))
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.cursor != 1 {
		t.Fatalf("click should set cursor to row 1, got %d", um.cursor)
	}
	if um.page != pageSuiteConfig {
		t.Fatalf("click should activate menu row 1 (suite config), page = %d", um.page)
	}
}

func TestDashboardClickOutsideMenuIgnored(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)

	// Far below the menu, on the sysinfo card side for wide layouts.
	updated, _ := m.Update(clickMsg(m.width-5, contentOriginY(m)+m.height))
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.page != pageDashboard || um.cursor != 0 {
		t.Fatalf("click outside menu should be ignored, page = %d cursor = %d", um.page, um.cursor)
	}
}

func TestMouseClickDuringConfirmIgnored(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)
	m.confirm = true
	top, _, _ := dashboardMenuRegion(m)

	updated, _ := m.Update(clickMsg(5, contentOriginY(m)+top+1))
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.cursor != 0 {
		t.Fatalf("click during confirm modal should be ignored, cursor = %d", um.cursor)
	}
}
