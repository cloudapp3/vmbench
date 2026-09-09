package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/i18n"
)

// scrollState tracks the vertical scroll position of the current page body.
// page records which page the offset belongs to: the offset resets whenever
// navigation moves to a different page, without every transition site having
// to remember to do it.
type scrollState struct {
	page   page
	offset int // index of the first visible content line
}

// syncScrollPage resets the offset when the model has moved to another page.
func syncScrollPage(m Model) Model {
	if m.scroll.page != m.page {
		m.scroll.page = m.page
		m.scroll.offset = 0
	}
	return m
}

// clipLines returns lines [offset, offset+height) of s. Width is untouched:
// pages own per-line truncation, which assertRenderBounds enforces.
func clipLines(s string, offset, height int) string {
	if height <= 0 {
		return ""
	}
	if offset < 0 {
		offset = 0
	}
	lines := strings.Split(s, "\n")
	if offset >= len(lines) {
		return ""
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[offset:end], "\n")
}

// contentViewportHeight is the number of body lines that fit between the
// header, the footer, and the body style's vertical padding.
func contentViewportHeight(m Model) int {
	header := renderHeader(m)
	footer := renderFooter(m, scrollPos{})
	vh := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if vh < 5 {
		vh = 5
	}
	return vh
}

// pageContent is the pure per-page body switch factored out of View().
func pageContent(m Model) string {
	switch m.page {
	case pageDashboard:
		return viewDashboard(m)
	case pageRunning:
		return viewRunning(m)
	case pageResults:
		return viewResults(m)
	case pageCompare:
		return viewCompare(m)
	case pageConfig:
		return viewConfig(m)
	case pageSuiteResults:
		return viewSuiteResults(m)
	case pageHelp:
		return viewHelp(m)
	case pageComparePicker:
		return viewComparePicker(m)
	case pageResultDetail:
		return viewResultDetail(m)
	}
	return ""
}

// maxScrollOffset clamps an offset against the measured content height.
func maxScrollOffset(content string, vh int) int {
	max := lipgloss.Height(content) - vh
	if max < 0 {
		max = 0
	}
	return max
}

// scrollPos describes the visible window for the footer indicator.
type scrollPos struct {
	scrollable bool
	line       int // 1-based first visible line
	total      int // total content lines
}

// viewScrollPos renders the body with the display-time offset applied and
// reports the scroll window. View works on a value copy, so the defensive
// clamping here never mutates the stored model.
func viewScrollPos(m Model) (string, scrollPos) {
	m = syncScrollPage(m)
	content := pageContent(m)
	vh := contentViewportHeight(m)

	offset := m.scroll.offset
	if m.confirm {
		// The cancel modal overlays the tail of the content; keep it visible.
		offset = 0
	}
	if max := maxScrollOffset(content, vh); offset > max {
		offset = max
	}

	total := lipgloss.Height(content)
	pos := scrollPos{line: offset + 1, total: total}
	if maxScrollOffset(content, vh) > 0 {
		pos.scrollable = true
	}
	return widthGuard(clipLines(content, offset, vh), m.width-2), pos
}

// widthGuard truncates each line to the body content budget so lipgloss's
// block Width() never soft-wraps a long styled line into extra rows (which
// would push content past the viewport again). ANSI-aware.
func widthGuard(content string, budget int) string {
	if budget <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i18n.StyledWidth(line) > budget {
			lines[i] = i18n.TruncateStyled(line, budget)
		}
	}
	return strings.Join(lines, "\n")
}

// clampScroll clamps the stored offset after Update-side scroll changes.
func clampScroll(m Model) Model {
	m = syncScrollPage(m)
	content := pageContent(m)
	max := maxScrollOffset(content, contentViewportHeight(m))
	if m.scroll.offset > max {
		m.scroll.offset = max
	}
	if m.scroll.offset < 0 {
		m.scroll.offset = 0
	}
	return m
}

func scrollStep(m Model) int {
	step := contentViewportHeight(m) - 2
	if step < 1 {
		step = 1
	}
	return step
}

// handleScrollKeys applies global scroll keys (PgUp/PgDn/Home/End). It runs
// before per-page key routing; no page currently claims these keys.
func handleScrollKeys(m Model, msg tea.KeyMsg) (Model, bool) {
	m = syncScrollPage(m)
	var delta int
	switch msg.String() {
	case "pgup":
		delta = -scrollStep(m)
	case "pgdown":
		delta = scrollStep(m)
	case "home":
		m.scroll.offset = 0
		return clampScroll(m), true
	case "end":
		m.scroll.offset = 1 << 30
		return clampScroll(m), true
	default:
		return m, false
	}
	m.scroll.offset += delta
	return clampScroll(m), true
}

// handleMouseWheel maps wheel events to the same scroll deltas.
func handleMouseWheel(m Model, msg tea.MouseMsg) (Model, bool) {
	m = syncScrollPage(m)
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scroll.offset -= 3
	case tea.MouseButtonWheelDown:
		m.scroll.offset += 3
	default:
		return m, false
	}
	return clampScroll(m), true
}

// followFocus scrolls the minimum amount needed to keep the page's focused
// line (e.g. the results cursor row) inside the viewport. Called from Update
// after cursor moves so free scrolling via wheel is never fought.
func followFocus(m Model) Model {
	m = syncScrollPage(m)
	line, ok := focusedContentLine(m)
	if !ok {
		return m
	}
	content := pageContent(m)
	vh := contentViewportHeight(m)
	offset := m.scroll.offset
	if line < offset {
		offset = line
	} else if line > offset+vh-1 {
		offset = line - vh + 1
	}
	if offset < 0 {
		offset = 0
	}
	if max := maxScrollOffset(content, vh); offset > max {
		offset = max
	}
	m.scroll.offset = offset
	return m
}

// focusedContentLine reports the 0-based content line of the page's cursor,
// mirroring the layout math of the corresponding view. Tests pin the two
// together (TestResultsFocusedLine, TestConfigFocusedLineMatchesRender).
func focusedContentLine(m Model) (int, bool) {
	switch m.page {
	case pageResults:
		return resultsFocusedLine(m)
	case pageComparePicker:
		return pickerFocusedLine(m)
	case pageConfig:
		return configFocusedLine(m)
	}
	return 0, false
}
