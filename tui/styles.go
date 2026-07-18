package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

func titleStyle() lipgloss.Style {
	t := theme.Active
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Bg).
		Background(t.Primary).
		Padding(0, 2).
		MarginBottom(1)
}

func subtitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Muted)
}

func headerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Muted)
}

func footerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Muted).Padding(0, 2)
}

func menuItemStyle() lipgloss.Style {
	return lipgloss.NewStyle().Padding(0, 2).Foreground(theme.Active.Fg)
}

func selectedItemStyle() lipgloss.Style {
	t := theme.Active
	return lipgloss.NewStyle().
		Padding(0, 2).
		Bold(true).
		Foreground(t.Bg).
		Background(t.Primary)
}

func sectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		MarginTop(1).
		Foreground(theme.Active.Primary)
}

func statusDoneStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Success).Bold(true)
}

func statusRunStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Warning).Bold(true)
}

func statusFailStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Danger).Bold(true)
}

func statusWaitStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Muted)
}

func statusSkipStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Muted).Italic(true)
}

func deltaUpStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Success).Bold(true)
}

func deltaDownStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Danger).Bold(true)
}

func deltaSameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.Active.Muted)
}

func categoryColor(cat string) lipgloss.AdaptiveColor {
	return theme.Active.CategoryColor(cat)
}

var _ = categoryColor
