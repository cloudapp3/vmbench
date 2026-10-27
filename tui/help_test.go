package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
)

func TestFooterMatchesRegistry(t *testing.T) {
	for _, p := range helpPageOrder {
		// The results page filters its tab-scoped hints, so walk all tabs.
		tabs := []int{0}
		if p == pageResults {
			tabs = []int{0, 1, 2}
		}
		for _, tab := range tabs {
			m := scrollTestModel(t, p, nil)
			m.resultsTab = tab
			view := m.View()
			for _, e := range helpFor(p) {
				if !e.short || !footerHintApplies(m, e) {
					continue
				}
				if !strings.Contains(view, i18n.T(e.descKey)) {
					t.Fatalf("page %d tab %d footer missing short hint %q (%s)", p, tab, e.keys, e.descKey)
				}
			}
		}
	}
}

// TestResultsFooterHintsFilterByTab pins the tab-scoped footer filtering:
// expand only shows on the grouped tab, detail only on the flat tab.
func TestResultsFooterHintsFilterByTab(t *testing.T) {
	m := scrollTestModel(t, pageResults, nil)

	m.resultsTab = 0
	hints := helpFooterHints(m)
	if hintTouched(hints, i18n.T("tui.hint.expand")) || hintTouched(hints, i18n.T("tui.hint.detail")) {
		t.Fatalf("cards tab should hide expand/detail hints: %v", hints)
	}

	m.resultsTab = 1
	hints = helpFooterHints(m)
	if !hintTouched(hints, i18n.T("tui.hint.expand")) {
		t.Fatalf("grouped tab should show the expand hint: %v", hints)
	}
	if hintTouched(hints, i18n.T("tui.hint.detail")) {
		t.Fatalf("grouped tab should hide the detail hint: %v", hints)
	}

	m.resultsTab = 2
	hints = helpFooterHints(m)
	if !hintTouched(hints, i18n.T("tui.hint.detail")) {
		t.Fatalf("flat tab should show the detail hint: %v", hints)
	}
	if hintTouched(hints, i18n.T("tui.hint.expand")) {
		t.Fatalf("flat tab should hide the expand hint: %v", hints)
	}
}

func hintTouched(hints []comp.Hint, desc string) bool {
	for _, h := range hints {
		if h.Desc == desc {
			return true
		}
	}
	return false
}

func TestHelpToggleRoundTrip(t *testing.T) {
	for _, from := range helpPageOrder {
		m := scrollTestModel(t, from, nil)

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		um, ok := updated.(Model)
		if !ok {
			t.Fatalf("Update returned %T, want Model", updated)
		}
		if um.page != pageHelp || um.helpFrom != from {
			t.Fatalf("? from page %d: got page=%d helpFrom=%d, want page=%d helpFrom=%d", from, um.page, um.helpFrom, pageHelp, from)
		}
		if !strings.Contains(um.View(), i18n.T("tui.help.currentSection")) {
			t.Fatalf("help page missing current-section header from page %d:\n%s", from, um.View())
		}

		updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
		um = updated.(Model)
		if um.page != from {
			t.Fatalf("esc should return to page %d, got %d", from, um.page)
		}
	}
}

func TestHelpShowsEveryPageSection(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	um := updated.(Model)
	um.width = 200
	um.height = 120
	view := um.View()
	for _, p := range helpPageOrder {
		if !strings.Contains(view, helpSectionTitle(p)) {
			t.Fatalf("help page missing section title for page %d: %q", p, helpSectionTitle(p))
		}
	}
	for _, e := range globalHelpEntries() {
		if !strings.Contains(view, i18n.T(e.descKey)) {
			t.Fatalf("help page missing global entry %q", e.descKey)
		}
	}
}

func TestHelpSuppressedDuringTextEntry(t *testing.T) {
	m := scrollTestModel(t, pageComparePicker, nil)

	// "m" switches the picker to manual entry with the path input focused.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	um := updated.(Model)
	if um.picker.mode != pickerManual || um.picker.inputFocus != 'a' {
		t.Fatalf("manual entry not focused: mode=%d focus=%d", um.picker.mode, um.picker.inputFocus)
	}

	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.page != pageComparePicker {
		t.Fatalf("? must not toggle help while text entry is focused, page = %d", um.page)
	}
	if got := um.picker.pathA.Value(); got != "?" {
		t.Fatalf("rune should reach the text field, got %q", got)
	}
}

func TestHelpPageFitsCompactTerminal(t *testing.T) {
	m := scrollTestModel(t, pageHelp, nil)
	m.helpFrom = pageConfig
	assertRenderBounds(t, m.View(), 80, 24)

	i18n.SetLang("zh-CN")
	t.Cleanup(func() { i18n.SetLang("en") })
	mZh := scrollTestModel(t, pageHelp, nil)
	mZh.helpFrom = pageConfig
	assertRenderBounds(t, mZh.View(), 80, 24)
	if !strings.Contains(mZh.View(), i18n.T("tui.help.title")) {
		t.Fatalf("zh-CN help page missing translated title")
	}
}
