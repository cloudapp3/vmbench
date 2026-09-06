package textgrid

import (
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/i18n"
)

func TestRenderAlignsCJKHeadersWithASCIIData(t *testing.T) {
	out := Render(
		[]string{"工作负载", "状态"},
		[][]string{
			{"CPU Single-Core (sysbench)", "ok"},
			{"Disk 4K Random Read Q1 (fio)", "fail"},
		},
		2,
	)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3:\n%s", len(lines), out)
	}
	// Compare the display width of each line's prefix before its second
	// column: byte offsets differ for CJK, cell offsets must not.
	prefixWidth := func(line, marker string) int {
		idx := strings.Index(line, marker)
		if idx < 0 {
			return -1
		}
		return i18n.StringWidth(line[:idx])
	}
	dataOffset := prefixWidth(lines[1], "  ok")
	for _, line := range lines {
		marker := "  状态"
		if !strings.Contains(line, marker) {
			marker = "  fail"
		}
		if !strings.Contains(line, marker) {
			marker = "  ok"
		}
		if got := prefixWidth(line, marker); got != dataOffset {
			t.Fatalf("column start %d, want %d:\n%s", got, dataOffset, out)
		}
	}
}

func TestRenderPadsShortRowsAndTrimsTrailing(t *testing.T) {
	out := Render([]string{"A", "B"}, [][]string{{"x"}}, 2)
	if out != "A  B\nx\n" {
		t.Fatalf("short row output = %q", out)
	}
}
