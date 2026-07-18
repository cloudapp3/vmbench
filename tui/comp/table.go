package comp

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/theme"
)

type TableColumn struct {
	Title string
	Width int
	Align lipgloss.Position
}

type TableRow struct {
	Cells     []string
	Highlight bool
	Accent    lipgloss.AdaptiveColor
}

func RenderTable(cols []TableColumn, rows []TableRow) string {
	t := theme.Active
	if len(cols) == 0 {
		return ""
	}
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Muted)

	var headerCells []string
	for _, c := range cols {
		s := headerStyle.Width(c.Width)
		if c.Align != 0 {
			s = s.Align(c.Align)
		}
		headerCells = append(headerCells, s.Render(c.Title))
	}
	header := strings.Join(headerCells, " ")

	totalW := 0
	for _, c := range cols {
		totalW += c.Width
	}
	totalW += len(cols) - 1
	separator := lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("─", totalW))

	var lines []string
	lines = append(lines, header, separator)
	for _, r := range rows {
		var cells []string
		for i, c := range cols {
			if i >= len(r.Cells) {
				cells = append(cells, lipgloss.NewStyle().Width(c.Width).Render(""))
				continue
			}
			s := lipgloss.NewStyle().Width(c.Width).Foreground(t.Fg)
			if c.Align != 0 {
				s = s.Align(c.Align)
			}
			if r.Highlight {
				s = s.Background(t.Surface).Bold(true)
			}
			cells = append(cells, s.Render(truncate(r.Cells[i], c.Width)))
		}
		row := strings.Join(cells, " ")
		if r.Highlight && r.Accent.Dark != "" {
			row = lipgloss.NewStyle().Foreground(r.Accent).Render("▎") + row[len("▎"):]
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}
