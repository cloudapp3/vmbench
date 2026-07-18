package comp

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

func Bar(label string, value, max float64, width int, unit string, accent lipgloss.AdaptiveColor) string {
	t := theme.Active
	if max <= 0 {
		max = 1
	}
	ratio := value / max
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	if accent.Dark == "" {
		accent = t.Primary
	}
	bar := ProgressBar(width, ratio, accent)
	val := fmt.Sprintf("%.1f %s", value, unit)
	valStyle := lipgloss.NewStyle().Foreground(t.Fg)
	labelStyle := lipgloss.NewStyle().Foreground(t.Muted)
	return labelStyle.Render(label) + " " + bar + " " + valStyle.Render(val)
}
