package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// helpEntry is one keybinding row in the help registry. descKey is an i18n
// key resolved at render time; short entries also appear in the footer.
type helpEntry struct {
	keys    string
	descKey string
	short   bool
}

// globalHelpEntries apply to every page. They are appended to each page's
// footer and rendered as their own section on the help page.
func globalHelpEntries() []helpEntry {
	return []helpEntry{
		{keys: "?", descKey: "tui.hint.help", short: true},
		{keys: "PgUp/PgDn", descKey: "tui.hint.pageUp", short: false},
		{keys: "Home/End", descKey: "tui.hint.top", short: false},
		{keys: "Wheel", descKey: "tui.hint.wheel", short: false},
	}
}

// helpFor is the single source of truth for per-page keybindings, consumed
// by both the footer and the help page.
func helpFor(p page) []helpEntry {
	switch p {
	case pageDashboard:
		return []helpEntry{
			{keys: "↑↓/jk", descKey: "tui.hint.nav", short: true},
			{keys: "↵", descKey: "tui.hint.select", short: true},
			{keys: "t", descKey: "tui.hint.theme", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageRunning:
		return []helpEntry{
			{keys: "esc", descKey: "tui.hint.cancel", short: true},
			{keys: "tab", descKey: "tui.hint.log", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageResults:
		return []helpEntry{
			{keys: "tab", descKey: "tui.hint.view", short: true},
			{keys: "↑↓/jk", descKey: "tui.hint.nav", short: true},
			{keys: "↵", descKey: "tui.hint.expand", short: true},
			{keys: "d", descKey: "tui.hint.detail", short: true},
			{keys: "s", descKey: "tui.hint.save", short: true},
			{keys: "esc", descKey: "tui.hint.back", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageResultDetail:
		return []helpEntry{
			{keys: "esc", descKey: "tui.hint.back", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageCompare:
		return []helpEntry{
			{keys: "r", descKey: "tui.hint.repick", short: false},
			{keys: "esc", descKey: "tui.hint.back", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageConfig:
		return []helpEntry{
			{keys: "↑↓", descKey: "tui.hint.field", short: true},
			{keys: "←→", descKey: "tui.hint.choose", short: true},
			{keys: "spc/x", descKey: "tui.hint.toggle", short: true},
			{keys: "1-9", descKey: "tui.hint.digits", short: false},
			{keys: "↵", descKey: "tui.hint.start", short: true},
			{keys: "esc", descKey: "tui.hint.back", short: true},
		}
	case pageSuiteRunning:
		return []helpEntry{
			{keys: "esc", descKey: "tui.hint.cancel", short: true},
			{keys: "tab", descKey: "tui.hint.log", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageSuiteResults:
		return []helpEntry{
			{keys: "esc", descKey: "tui.hint.back", short: true},
			{keys: "q", descKey: "tui.hint.quit", short: true},
		}
	case pageComparePicker:
		return []helpEntry{
			{keys: "↑↓", descKey: "tui.hint.nav", short: true},
			{keys: "spc", descKey: "tui.hint.pick", short: true},
			{keys: "c", descKey: "tui.hint.compareSel", short: true},
			{keys: "v", descKey: "tui.hint.viewRecord", short: false},
			{keys: "m", descKey: "tui.hint.manual", short: false},
			{keys: "esc", descKey: "tui.hint.back", short: true},
		}
	}
	return nil
}

// textEntryActive reports whether a raw-text field currently has focus, so
// global single-key bindings (?, q) and rune routing stay out of its way.
func textEntryActive(m Model) bool {
	switch m.page {
	case pageConfig:
		return m.config.textEntryActive()
	case pageComparePicker:
		return m.picker.mode == pickerManual && m.picker.inputFocus != 0
	}
	return false
}

// toggleHelp reacts to the global "?" key.
func toggleHelp(m Model) (Model, bool) {
	if m.page == pageHelp {
		m.page = m.helpFrom
		return m, true
	}
	m.helpFrom = m.page
	m.page = pageHelp
	return m, true
}

func updateHelp(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.page = m.helpFrom
		return m, nil
	}
	return m, nil
}

func helpSectionTitle(p page) string {
	switch p {
	case pageDashboard:
		return i18n.T("tui.help.sec.dashboard")
	case pageConfig:
		return i18n.T("tui.help.sec.config")
	case pageComparePicker:
		return i18n.T("tui.help.sec.comparePicker")
	case pageResultDetail:
		return i18n.T("tui.help.sec.resultDetail")
	case pageRunning:
		return i18n.T("tui.help.sec.running")
	case pageResults:
		return i18n.T("tui.help.sec.results")
	case pageCompare:
		return i18n.T("tui.help.sec.compare")
	case pageSuiteRunning:
		return i18n.T("tui.help.sec.suiteRunning")
	case pageSuiteResults:
		return i18n.T("tui.help.sec.suiteResults")
	}
	return ""
}

var helpPageOrder = []page{
	pageDashboard,
	pageConfig,
	pageRunning,
	pageResults,
	pageResultDetail,
	pageComparePicker,
	pageCompare,
	pageSuiteRunning,
	pageSuiteResults,
}

func viewHelp(m Model) string {
	t := theme.Active

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.help.title"))

	var sections []string
	sections = append(sections, title, "")

	current := helpSectionTitle(m.helpFrom)
	sections = append(sections,
		lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(i18n.T("tui.help.currentSection")+": "+current),
		helpRows(helpFor(m.helpFrom)),
		"",
	)

	allTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(i18n.T("tui.help.allPages"))
	var allRows []string
	for _, p := range helpPageOrder {
		if p == m.helpFrom {
			continue
		}
		allRows = append(allRows, lipgloss.NewStyle().Bold(true).Foreground(t.CategoryColor("system")).Render("◆ "+helpSectionTitle(p)))
		allRows = append(allRows, helpRows(helpFor(p)), "")
	}
	sections = append(sections, allTitle, strings.Join(allRows, "\n"))

	globalTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Render(i18n.T("tui.help.globalSection"))
	sections = append(sections, "", globalTitle, helpRows(globalHelpEntries()))

	return strings.Join(sections, "\n")
}

// helpRows renders entries as "  key  description" lines, aligned with the
// widest key label in the group.
func helpRows(entries []helpEntry) string {
	t := theme.Active
	keyW := 0
	for _, e := range entries {
		if w := i18n.StringWidth(e.keys); w > keyW {
			keyW = w
		}
	}
	if keyW > 16 {
		keyW = 16
	}
	var lines []string
	for _, e := range entries {
		key := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render(i18n.PadCells(e.keys, keyW))
		lines = append(lines, "  "+key+"  "+i18n.T(e.descKey))
	}
	return strings.Join(lines, "\n")
}

func helpFooterHints(p page) []comp.Hint {
	var hints []comp.Hint
	for _, e := range helpFor(p) {
		if e.short {
			hints = append(hints, comp.Hint{Key: e.keys, Desc: i18n.T(e.descKey)})
		}
	}
	for _, e := range globalHelpEntries() {
		if e.short {
			hints = append(hints, comp.Hint{Key: e.keys, Desc: i18n.T(e.descKey)})
		}
	}
	return hints
}
