package i18n

import "github.com/mattn/go-runewidth"

// Display-width-aware helpers for mixed ASCII/CJK output. Plain fmt padding
// ("%-20s") and rune-count truncation count every rune as one cell, so
// translated text (Chinese labels are two cells per rune) misaligns columns
// and can overflow fixed-width layouts.

// StringWidth returns the display width of s in terminal cells.
func StringWidth(s string) int {
	return runewidth.StringWidth(s)
}

// TruncateCells truncates s to at most max display cells, appending "…" when
// truncation occurs. Never splits a double-width rune across the boundary.
func TruncateCells(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= max {
		return s
	}
	return runewidth.Truncate(s, max, "…")
}

// PadCells pads s with spaces on the right to exactly width display cells,
// truncating first when s is wider (re-padding after a boundary-straddling
// cut keeps the promise of an exact cell count). Drop-in replacement for
// fmt's "%-Ns".
func PadCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) > width {
		s = runewidth.Truncate(s, width, "")
	}
	return runewidth.FillRight(s, width)
}
