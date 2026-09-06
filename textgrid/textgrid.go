// Package textgrid renders fixed-width text tables using display-cell
// alignment. text/tabwriter counts runes, so double-width CJK headers
// misalign columns; these helpers pad by terminal cells instead.
package textgrid

import (
	"strings"

	"github.com/cloudapp3/vmbench/i18n"
)

// Render writes a left-aligned table: one header row, then rows of cells.
// Column width is the maximum display width of the header and all cells,
// plus gap. Cells wider than the column are not truncated (data wins).
func Render(headers []string, rows [][]string, gap int) string {
	if len(headers) == 0 {
		return ""
	}
	if gap < 1 {
		gap = 2
	}
	cols := len(headers)
	widths := make([]int, cols)
	for i, h := range headers {
		widths[i] = i18n.StringWidth(h)
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			if w := i18n.StringWidth(row[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}

	renderRow := func(cells []string) string {
		parts := make([]string, 0, cols)
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			parts = append(parts, i18n.PadCells(cell, widths[i]))
		}
		return strings.TrimRight(strings.Join(parts, strings.Repeat(" ", gap)), " ")
	}

	var b strings.Builder
	b.WriteString(renderRow(headers))
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString(renderRow(row))
		b.WriteByte('\n')
	}
	return b.String()
}
