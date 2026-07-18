package comp

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type Status string

const (
	StatusWaiting Status = "waiting"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFail    Status = "fail"
	StatusSkip    Status = "skip"
	StatusOK      Status = "ok"
)

func StatusPill(s Status, text string) string {
	t := theme.Active
	var icon string
	var color lipgloss.AdaptiveColor
	switch s {
	case StatusDone, StatusOK:
		icon = "●"
		color = t.Success
	case StatusRunning:
		icon = "◐"
		color = t.Warning
	case StatusFail:
		icon = "✗"
		color = t.Danger
	case StatusSkip:
		icon = "⊘"
		color = t.Muted
	default:
		icon = "○"
		color = t.Muted
	}
	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	if text == "" {
		return style.Render(icon)
	}
	return style.Render(icon+" ") + lipgloss.NewStyle().Foreground(t.Fg).Render(text)
}

func StatusIcon(s Status) string {
	return StatusPill(s, "")
}

func StatusFromString(s string) Status {
	switch s {
	case "done", "ok":
		return StatusDone
	case "running":
		return StatusRunning
	case "fail", "error":
		return StatusFail
	case "skip", "skipped":
		return StatusSkip
	default:
		return StatusWaiting
	}
}
