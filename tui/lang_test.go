package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/i18n"
)

func keyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestCycleLangStateRingStartsAtAuto(t *testing.T) {
	// Supported() is sorted: auto, en, zh-CN, back to auto.
	if got := cycleLangState(""); got != "en" {
		t.Fatalf(`cycleLangState("") = %q, want "en"`, got)
	}
	if got := cycleLangState("en"); got != "zh-CN" {
		t.Fatalf(`cycleLangState("en") = %q, want "zh-CN"`, got)
	}
	if got := cycleLangState("zh-CN"); got != "" {
		t.Fatalf(`cycleLangState("zh-CN") = %q, want ""`, got)
	}
	if got := cycleLangState("bogus"); got != "" {
		t.Fatalf(`cycleLangState("bogus") = %q, want "" (restart at auto)`, got)
	}
}

func TestApplyLangState(t *testing.T) {
	t.Setenv("VMBENCH_LANG", "zh-CN")
	defer i18n.SetLang("en")

	// Auto re-resolves through VMBENCH_LANG / OS locale / default.
	i18n.SetLang("en")
	applyLangState("")
	if got := i18n.Lang(); got != "zh-CN" {
		t.Fatalf("applyLangState(\"\") → Lang() = %q, want zh-CN from VMBENCH_LANG", got)
	}

	// Explicit canonical codes switch immediately.
	applyLangState("en")
	if got := i18n.Lang(); got != "en" {
		t.Fatalf(`applyLangState("en") → Lang() = %q, want en`, got)
	}
}

func TestLangLineLabelMarksAuto(t *testing.T) {
	defer i18n.SetLang("en")
	i18n.SetLang("en")

	if got := langLineLabel(""); !strings.Contains(got, "English") || !strings.Contains(got, i18n.T("tui.dashboard.langAuto")) {
		t.Fatalf("auto label %q must show active language and auto hint", got)
	}
	if got := langLineLabel("en"); strings.Contains(got, i18n.T("tui.dashboard.langAuto")) {
		t.Fatalf("explicit label %q must not carry the auto hint", got)
	}
}

func TestDashboardLangKeyCycles(t *testing.T) {
	t.Setenv("VMBENCH_LANG", "en") // pin the auto resolution for the wrap-around step
	defer i18n.SetLang("en")
	m := scrollTestModel(t, pageDashboard, nil)

	updated, _ := m.Update(keyMsg('l'))
	um := updated.(Model)
	if um.langExplicit != "en" || i18n.Lang() != "en" {
		t.Fatalf("first l: langExplicit = %q, active = %q, want en/en", um.langExplicit, i18n.Lang())
	}

	updated, _ = um.Update(keyMsg('l'))
	um = updated.(Model)
	if um.langExplicit != "zh-CN" || i18n.Lang() != "zh-CN" {
		t.Fatalf("second l: langExplicit = %q, active = %q, want zh-CN/zh-CN", um.langExplicit, i18n.Lang())
	}

	updated, _ = um.Update(keyMsg('l'))
	um = updated.(Model)
	if um.langExplicit != "" || i18n.Lang() != "en" {
		t.Fatalf("third l: langExplicit = %q, active = %q, want auto/en (test env default)", um.langExplicit, i18n.Lang())
	}
}

func TestDashboardLangLineRendersUnderTheme(t *testing.T) {
	defer i18n.SetLang("en")
	i18n.SetLang("en")
	m := scrollTestModel(t, pageDashboard, nil)
	m.width = 120

	rows := strings.Split(viewDashboard(m), "\n")
	themeRow, langRow := -1, -1
	for i, row := range rows {
		if strings.Contains(row, i18n.T("tui.dashboard.theme")+":") {
			themeRow = i
		}
		if strings.Contains(row, i18n.T("tui.dashboard.lang")+":") {
			langRow = i
		}
	}
	if themeRow < 0 || langRow != themeRow+1 {
		t.Fatalf("language line must render directly under the theme line: themeRow = %d, langRow = %d", themeRow, langRow)
	}
	if !strings.Contains(rows[langRow], "English") {
		t.Fatalf("language line %q must show the active language name", rows[langRow])
	}
}
