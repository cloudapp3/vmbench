package comp

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type HeaderInfo struct {
	Brand     string
	Version   string
	CPU       string
	Cores     string
	Now       time.Time
	ThemeName string
	Width     int
}

func Header(h HeaderInfo) string {
	t := theme.Active
	width := h.Width
	if width < 40 {
		width = 40
	}

	brandStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Bg).
		Background(t.Primary).
		Padding(0, 1)
	brand := brandStyle.Render(h.Brand)

	versionStyle := lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
	version := versionStyle.Render(h.Version)

	cpu := lipgloss.NewStyle().Foreground(t.Fg).Render(h.CPU)
	cores := lipgloss.NewStyle().Foreground(t.Muted).Render(h.Cores)

	right := lipgloss.NewStyle().Foreground(t.Accent).Render(h.Now.Format("15:04:05"))
	tn := lipgloss.NewStyle().Foreground(t.Secondary).Render("⬢ " + h.ThemeName)

	left := lipgloss.JoinHorizontal(lipgloss.Bottom, brand, " ", version)
	mid := lipgloss.JoinHorizontal(lipgloss.Bottom, cpu, " ", cores)

	rightCombined := tn + "  " + right
	maxMidWidth := width - lipgloss.Width(left) - lipgloss.Width(rightCombined) - 4
	if maxMidWidth <= 0 {
		mid = ""
	} else if lipgloss.Width(mid) > maxMidWidth {
		midText := truncate(strings.TrimSpace(h.CPU+" "+h.Cores), maxMidWidth)
		mid = lipgloss.NewStyle().Foreground(t.Fg).Render(midText)
	}
	combined := left
	if mid != "" {
		combined += "  " + mid
	}
	pad := width - lipgloss.Width(combined) - lipgloss.Width(rightCombined) - 2
	if pad < 1 {
		pad = 1
	}
	line := combined + strings.Repeat(" ", pad) + rightCombined

	border := lipgloss.NewStyle().
		Foreground(t.Border).
		Render(strings.Repeat("─", width))

	return line + "\n" + border
}
