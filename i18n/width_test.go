package i18n

import "testing"

func TestStringWidth(t *testing.T) {
	cases := map[string]int{
		"":      0,
		"abc":   3,
		"运行中":   6,
		"ok 运行": 7,
		"a…":    2,
	}
	for s, want := range cases {
		if got := StringWidth(s); got != want {
			t.Errorf("StringWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestTruncateCells(t *testing.T) {
	cases := []struct {
		s    string
		max  int
		want string
	}{
		{"anything", 0, ""},
		{"", 5, ""},
		{"short", 10, "short"},     // fits → unchanged
		{"benchmark", 6, "bench…"}, // ASCII cut leaves room for ellipsis
		{"运行中状态良好", 8, "运行中…"},     // 10 cells → 8: 3 CJK + …
		{"运行中状态良好", 7, "运行中…"},     // 6 CJK cells + … = 7 exactly
		{"运行中状态良好", 6, "运行…"},      // cannot fit 3 CJK + … → 2 CJK + …
		{"ok 运行", 4, "ok …"},       // mixed
		{"ab", 2, "ab"},            // exact fit
		{"运行", 4, "运行"},            // exact fit, double width
		{"运行", 3, "运…"},            // 4 cells > 3 → single CJK + …
	}
	for _, c := range cases {
		if got := TruncateCells(c.s, c.max); got != c.want {
			t.Errorf("TruncateCells(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
		}
	}
}

// The bug being fixed: the old rune-count truncation let 6 CJK runes pass a
// max of 10 cells (12 actual) and cut 9 runes (18 cells) on the other path.
func TestTruncateCellsBoundsDisplayWidth(t *testing.T) {
	s := "运行中状态良好" // 6 runes, 12 cells
	for max := 1; max <= 12; max++ {
		got := TruncateCells(s, max)
		if w := StringWidth(got); w > max {
			t.Fatalf("TruncateCells(s, %d) = %q, width %d exceeds max", max, got, w)
		}
	}
}

func TestPadCells(t *testing.T) {
	cases := []struct {
		s     string
		width int
		want  string
	}{
		{"ab", 5, "ab   "},
		{"abcde", 5, "abcde"},  // exact
		{"abcdef", 5, "abcde"}, // wider → hard truncate, no ellipsis (padding keeps tables aligned)
		{"运行", 6, "运行  "},      // 4 cells + 2 spaces
		{"运行", 4, "运行"},        // exact fit, double width
		{"运行状态", 3, "运 "},      // hard cut at boundary then re-pad: exact cell count
		{"x", 0, ""},
	}
	for _, c := range cases {
		if got := PadCells(c.s, c.width); got != c.want {
			t.Errorf("PadCells(%q, %d) = %q, want %q", c.s, c.width, got, c.want)
		}
	}
}
