package comp

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

func ProgressBar(width int, ratio float64, accent lipgloss.AdaptiveColor) string {
	t := theme.Active
	if width < 4 {
		width = 4
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(float64(width) * ratio)
	if filled > width {
		filled = width
	}

	on := lipgloss.NewStyle().Foreground(accent).Render(repeatRune('▰', filled))
	off := lipgloss.NewStyle().Foreground(t.Subtle).Render(repeatRune('▱', width-filled))
	return on + off
}

func ProgressLine(width int, ratio float64, label string, accent lipgloss.AdaptiveColor) string {
	t := theme.Active
	if accent.Dark == "" {
		accent = t.Primary
	}
	pct := fmt.Sprintf(" %3.0f%%", ratio*100)
	pctStyled := lipgloss.NewStyle().Foreground(t.Fg).Bold(true).Render(pct)
	bar := ProgressBar(width, ratio, accent)
	if label == "" {
		return bar + " " + pctStyled
	}
	labelStyled := lipgloss.NewStyle().Foreground(t.Muted).Render(label)
	return labelStyled + " " + bar + " " + pctStyled
}
