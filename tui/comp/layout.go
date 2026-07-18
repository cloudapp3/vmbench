package comp

type Breakpoint int

const (
	BreakpointTiny    Breakpoint = iota // <80
	BreakpointCompact                   // 80..119
	BreakpointNormal                    // 120..159
	BreakpointWide                      // 160..199
	BreakpointUltra                     // >=200
)

func BreakpointFor(width int) Breakpoint {
	switch {
	case width < 80:
		return BreakpointTiny
	case width < 120:
		return BreakpointCompact
	case width < 160:
		return BreakpointNormal
	case width < 200:
		return BreakpointWide
	default:
		return BreakpointUltra
	}
}

func ColumnsFor(width int) int {
	switch BreakpointFor(width) {
	case BreakpointTiny, BreakpointCompact:
		return 1
	case BreakpointNormal:
		return 2
	case BreakpointWide:
		return 3
	default:
		return 4
	}
}

func AllocateColumns(total int, weights []float64) []int {
	if len(weights) == 0 {
		return nil
	}
	sum := 0.0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum <= 0 {
		base := total / len(weights)
		out := make([]int, len(weights))
		for i := range out {
			out[i] = base
		}
		return out
	}
	out := make([]int, len(weights))
	used := 0
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		c := int(float64(total) * w / sum)
		out[i] = c
		used += c
	}
	rem := total - used
	for i := 0; i < rem && i < len(out); i++ {
		out[i]++
	}
	return out
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
