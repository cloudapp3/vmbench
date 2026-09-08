package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/i18n"
)

func TestFooterMatchesRegistry(t *testing.T) {
	for _, p := range helpPageOrder {
		view := scrollTestModel(t, p, nil).View()
		for _, e := range helpFor(p) {
			if !e.short {
				continue
			}
			if !strings.Contains(view, i18n.T(e.descKey)) {
				t.Fatalf("page %d footer missing short hint %q (%s)", p, e.keys, e.descKey)
			}
		}
	}
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
	m := scrollTestModel(t, pageConfig, nil)
	m.config.field = fieldAdvanced

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.page != pageConfig {
		t.Fatalf("? must not toggle help while text entry is focused, page = %d", um.page)
	}
	if got := um.config.iperfHost; got != "?" {
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
