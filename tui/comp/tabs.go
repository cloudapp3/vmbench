package comp

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type Tab struct {
	Label string
	Key   string
}

func Tabs(width int, tabs []Tab, active int) string {
	t := theme.Active
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Bg).
		Background(t.Primary).
		Padding(0, 2)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 2)

	var parts []string
	for i, tab := range tabs {
		label := tab.Label
		if tab.Key != "" {
			label = tab.Label + " (" + tab.Key + ")"
		}
		if i == active {
			parts = append(parts, activeStyle.Render(label))
		} else {
			parts = append(parts, inactiveStyle.Render(label))
		}
	}
	row := strings.Join(parts, "")
	used := lipgloss.Width(row)
	if used < width {
		row += lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat(" ", width-used))
	}
	underline := lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", width))
	return row + "\n" + underline
}
