package comp

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

const bannerArt = `
██╗   ██╗███╗   ███╗██████╗ ███████╗███╗   ██╗ ██████╗██╗  ██╗
██║   ██║████╗ ████║██╔══██╗██╔════╝████╗  ██║██╔════╝██║  ██║
██║   ██║██╔████╔██║██████╔╝█████╗  ██╔██╗ ██║██║     ███████║
╚██╗ ██╔╝██║╚██╔╝██║██╔══██╗██╔══╝  ██║╚██╗██║██║     ██╔══██║
 ╚████╔╝ ██║ ╚═╝ ██║██████╔╝███████╗██║ ╚████║╚██████╗██║  ██║
  ╚═══╝  ╚═╝     ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═══╝ ╚═════╝╚═╝  ╚═╝`

const bannerCompact = ` ██╗  ██╗███╗   ███╗  ████████╗
 ██║  ██║████╗ ████║██╔════╝
 ██║  ██║██╔████╔██║█████████╗
  ╚═══╝ ╚═╝     ╚═╝╚════════╝`

const bannerMini = `▌ ▌▛▚▀▖▛▀▖▛▀▘▙ ▌▞▀▖▌ ▌
▐▐ ▌▌▐ ▌▙▟ ▙▄ ▌▙▌▌  ▙▄▌
 ▘ ▘▘ ▘▘▝ ▘▝▀▘▘ ▘▝▀ ▘ ▘`

func Banner(width int) string {
	t := theme.Active
	var art string
	switch {
	case width >= 80:
		art = bannerArt
	case width >= 40:
		art = bannerCompact
	default:
		art = bannerMini
	}
	style := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)
	return style.Render(strings.TrimLeft(art, "\n"))
}

func TaglineRow(width int, tagline string) string {
	t := theme.Active
	style := lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
	return style.Render(tagline)
}
