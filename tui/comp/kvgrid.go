package comp

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type KV struct {
	Key   string
	Value string
}

func KVGrid(width int, rows []KV) string {
	t := theme.Active
	if len(rows) == 0 {
		return ""
	}
	keyW := 0
	for _, r := range rows {
		if l := lipgloss.Width(r.Key); l > keyW {
			keyW = l
		}
	}
	if keyW > width/2 {
		keyW = width / 2
	}
	keyStyle := lipgloss.NewStyle().Foreground(t.Muted).Width(keyW)
	valStyle := lipgloss.NewStyle().Foreground(t.Fg)

	var lines []string
	for _, r := range rows {
		k := keyStyle.Render(r.Key)
		valWidth := width - keyW - 2
		v := r.Value
		if valWidth > 0 && lipgloss.Width(v) > valWidth {
			v = truncate(v, valWidth)
		}
		lines = append(lines, k+" "+valStyle.Render(v))
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxRunes {
		return s
	}
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	return string(rs[:maxRunes-1]) + "…"
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

var _ = formatBytes
