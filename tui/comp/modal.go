package comp

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type Modal struct {
	Title   string
	Body    string
	Actions []ModalAction
	Width   int
}

type ModalAction struct {
	Key      string
	Label    string
	Selected bool
	Danger   bool
}

func (m Modal) Render() string {
	t := theme.Active
	w := m.Width
	if w < 30 {
		w = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	bodyStyle := lipgloss.NewStyle().Foreground(t.Fg)

	var actionParts []string
	for _, a := range m.Actions {
		var s lipgloss.Style
		switch {
		case a.Selected && a.Danger:
			s = lipgloss.NewStyle().Foreground(t.Bg).Background(t.Danger).Padding(0, 2).Bold(true)
		case a.Selected:
			s = lipgloss.NewStyle().Foreground(t.Bg).Background(t.Primary).Padding(0, 2).Bold(true)
		case a.Danger:
			s = lipgloss.NewStyle().Foreground(t.Danger).Padding(0, 2)
		default:
			s = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 2)
		}
		label := a.Label
		if a.Key != "" {
			label = a.Label + " [" + a.Key + "]"
		}
		actionParts = append(actionParts, s.Render(label))
	}
	actions := strings.Join(actionParts, "  ")

	parts := []string{
		titleStyle.Render(m.Title),
		"",
		bodyStyle.Render(m.Body),
		"",
		actions,
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus).
		Padding(1, 3).
		Width(w).
		Align(lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Center, parts...))

	return box
}
