package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// updateMouse routes mouse events: the wheel scrolls every page; left clicks
// activate Dashboard menu rows and the theme line. Other pages gain their own
// click regions as they grow hit-testable geometry.
func updateMouse(m Model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		scrolled, _ := handleMouseWheel(m, msg)
		return scrolled, nil
	}

	if m.confirm {
		return m, nil
	}
	if m.page != pageDashboard {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	return dashboardClick(m, msg.X, msg.Y)
}

// contentOriginY is the screen row where content line 0 renders: header
// height plus the body style's top padding.
func contentOriginY(m Model) int {
	return lipgloss.Height(renderHeader(m)) + 1
}

// dashboardMenuRegion reports the menu's position in content coordinates.
// It mirrors viewDashboard's layout math; the render tests pin the two
// together (TestDashboardMenuRegionMatchesRender).
func dashboardMenuRegion(m Model) (top int, count int, xMax int) {
	top = 0
	bp := comp.BreakpointFor(m.width)
	if bp >= comp.BreakpointCompact {
		top = lipgloss.Height(comp.Banner(m.width)) + 2 // tagline + blank
	}
	top += 2 // menu header + blank line
	if bp >= comp.BreakpointNormal {
		xMax = m.width / 2
	} else {
		xMax = m.width
	}
	return top, len(menuItems()), xMax
}

func dashboardClick(m Model, x, y int) (tea.Model, tea.Cmd) {
	top, count, xMax := dashboardMenuRegion(m)
	line := y - contentOriginY(m) + m.scroll.offset
	if x >= xMax || line < top {
		return m, nil
	}
	switch row := line - top; {
	case row >= 0 && row < count:
		m.cursor = row
		return updateDashboard(m, tea.KeyMsg{Type: tea.KeyEnter})
	case row == count+1: // blank line then the theme line under the menu
		theme.CycleTheme()
		return m, nil
	}
	return m, nil
}
