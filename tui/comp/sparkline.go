package comp

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

var sparkRunes = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func Sparkline(values []float64) string {
	t := theme.Active
	if len(values) == 0 {
		return ""
	}
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	out := make([]rune, len(values))
	for i, v := range values {
		idx := 0
		if span > 0 {
			ratio := (v - min) / span
			idx = int(ratio * float64(len(sparkRunes)-1))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(sparkRunes) {
				idx = len(sparkRunes) - 1
			}
		}
		out[i] = sparkRunes[idx]
	}
	return lipgloss.NewStyle().Foreground(t.Accent).Render(string(out))
}
