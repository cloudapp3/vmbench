package comp

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type ToastKind int

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastWarn
	ToastError
)

type Toast struct {
	Kind    ToastKind
	Message string
	Until   time.Time
}

type ToastExpireMsg struct{ Stamp time.Time }

func ShowToast(msg string, kind ToastKind, dur time.Duration) (Toast, tea.Cmd) {
	t := Toast{Kind: kind, Message: msg, Until: time.Now().Add(dur)}
	stamp := t.Until
	cmd := tea.Tick(dur, func(time.Time) tea.Msg {
		return ToastExpireMsg{Stamp: stamp}
	})
	return t, cmd
}

func (t Toast) Active() bool {
	return t.Message != "" && time.Now().Before(t.Until)
}

func (t Toast) Render(width int) string {
	if !t.Active() {
		return ""
	}
	th := theme.Active
	var color lipgloss.AdaptiveColor
	var icon string
	switch t.Kind {
	case ToastSuccess:
		color = th.Success
		icon = "✓"
	case ToastWarn:
		color = th.Warning
		icon = "⚠"
	case ToastError:
		color = th.Danger
		icon = "✗"
	default:
		color = th.Info
		icon = "ℹ"
	}
	style := lipgloss.NewStyle().
		Foreground(th.Bg).
		Background(color).
		Padding(0, 2).
		Bold(true)
	return style.Render(icon + "  " + t.Message)
}
