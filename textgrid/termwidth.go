package textgrid

import (
	"io"
	"os"

	"golang.org/x/term"
)

// StdoutWidth returns the terminal width behind w when w wraps a terminal
// file (typically os.Stdout), else 0 (unknown). Console renderers use it to
// drop low-priority columns on narrow terminals; non-terminal writers such
// as test buffers keep the full layout.
func StdoutWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return width
}
