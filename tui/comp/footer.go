package comp

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type Hint struct {
	Key  string
	Desc string
}

func Footer(width int, hints []Hint) string {
	t := theme.Active
	if width < 20 {
		width = 20
	}
	keyStyle := lipgloss.NewStyle().
		Foreground(t.Bg).
		Background(t.Accent).
		Padding(0, 1).
		Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(t.Muted)

	var parts []string
	for _, h := range hints {
		parts = append(parts, keyStyle.Render(h.Key)+descStyle.Render(" "+h.Desc))
	}
	line := strings.Join(parts, "  ")
	border := lipgloss.NewStyle().
		Foreground(t.Border).
		Render(strings.Repeat("─", width))
	return border + "\n" + line
}
