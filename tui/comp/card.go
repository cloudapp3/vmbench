package comp

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type Card struct {
	Title    string
	Subtitle string
	Body     string
	Footer   string
	Accent   lipgloss.AdaptiveColor
	Width    int
	Focused  bool
}

func (c Card) Render() string {
	t := theme.Active
	accent := c.Accent
	if accent.Dark == "" && accent.Light == "" {
		accent = t.Primary
	}

	border := t.Border
	if c.Focused {
		border = t.BorderFocus
	}

	innerWidth := c.Width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	var headerLine string
	if c.Title != "" {
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(accent)
		titleText := titleStyle.Render(c.Title)
		if c.Subtitle != "" {
			sub := lipgloss.NewStyle().Foreground(t.Muted).Render(c.Subtitle)
			titleText = lipgloss.JoinHorizontal(lipgloss.Bottom, titleText, "  ", sub)
		}
		headerLine = titleText
	}

	bodyStyle := lipgloss.NewStyle().Foreground(t.Fg)
	body := bodyStyle.Render(c.Body)

	var parts []string
	if headerLine != "" {
		parts = append(parts, headerLine)
		band := lipgloss.NewStyle().
			Foreground(accent).
			Render(repeatRune('▔', innerWidth))
		parts = append(parts, band)
	}
	parts = append(parts, body)
	if c.Footer != "" {
		footerStyle := lipgloss.NewStyle().Foreground(t.Muted)
		parts = append(parts, footerStyle.Render(c.Footer))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(c.Width).
		Render(lipgloss.JoinVertical(lipgloss.Left, parts...))

	return box
}

func repeatRune(r rune, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
