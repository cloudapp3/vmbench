package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/i18n"
)

func TestNewConfigStateDefaultsToFullChecklist(t *testing.T) {
	s := newConfigState()

	// Opening the page and pressing enter runs the full checkup; the cursor
	// starts on the start row so enter alone launches it.
	if s.sections != checkup.DefaultSections() {
		t.Fatalf("default sections = %+v, want all on", s.sections)
	}
	if row := s.currentRow(); row.kind != rowStart {
		t.Fatalf("cursor starts on row %+v, want start", row)
	}
	if s.iterations != 3 || s.ipVersion != "v4" {
		t.Fatalf("runtime defaults = %d/%s, want 3/v4", s.iterations, s.ipVersion)
	}

	// The UI no longer picks tools/providers/sets; normalization must fill
	// the documented defaults for them.
	norm, err := checkup.NormalizeOptions(s.buildCheckupOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(norm.HardwareTools) == 0 {
		t.Fatal("normalized checkup should default hardware tools")
	}
	if norm.MediaSet != checkup.DefaultMediaSet() {
		t.Fatalf("normalized MediaSet = %q, want %q", norm.MediaSet, checkup.DefaultMediaSet())
	}
	if strings.Join(norm.SpeedProviders, ",") == "" || strings.Join(norm.RoutePresets, ",") == "" {
		t.Fatalf("normalized providers/routes = %v/%v, want defaults", norm.SpeedProviders, norm.RoutePresets)
	}

	runNorm, err := vmbench.NormalizeOptions(s.buildRunOptions())
	if err != nil {
		t.Fatal(err)
	}
	if runNorm.Iterations != 3 || runNorm.Engine != "external" || runNorm.Filter != "" {
		t.Fatalf("normalized run options = %+v", runNorm)
	}
}

func TestConfigCursorNavigation(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	um := updated.(Model)
	if row := um.config.currentRow(); row.kind != rowSection || row.index != 0 {
		t.Fatalf("down from start = %+v, want first section", row)
	}

	// Up from start wraps around to the last visible row (advanced).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	um = updated.(Model)
	if row := um.config.currentRow(); row.kind != rowAdvanced {
		t.Fatalf("up from start = %+v, want advanced", row)
	}

	// Digits and toggles never move the cursor.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	um = updated.(Model)
	if row := um.config.currentRow(); row.kind != rowAdvanced {
		t.Fatalf("digit moved cursor: %+v", row)
	}
	if um.config.sections.Hardware {
		t.Fatal("digit 1 should untick hardware")
	}
}

func TestConfigSectionToggleBySpaceAndEnter(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.config.cursorAt(rowSection, 0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if updated.(Model).config.sections.Hardware {
		t.Fatal("space should untick the focused section")
	}
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !updated.(Model).config.sections.Hardware {
		t.Fatal("enter on a section row should toggle it back on")
	}
}

func TestConfigAdvancedToggleAndCycles(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.config.cursorAt(rowAdvanced, -1)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um := updated.(Model)
	if !um.config.advancedOpen {
		t.Fatal("enter on advanced should open it")
	}
	if got := len(um.config.visibleRows()); got != 1+len(um.config.sectionIDs)+1+advCount {
		t.Fatalf("open advanced rows = %d", got)
	}

	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyLeft})
	um = updated.(Model)
	if um.config.advancedOpen {
		t.Fatal("left on advanced should close it")
	}
	if row := um.config.currentRow(); row.kind != rowAdvanced {
		t.Fatalf("closing advanced left cursor on %+v, want advanced", row)
	}

	// Reopen, then cycle every setting in both directions.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRight})
	um = updated.(Model)
	if !um.config.advancedOpen {
		t.Fatal("right on advanced should open it")
	}

	um.config.cursorAt(rowAdvSetting, advIterations)
	um.config.iterations = 9
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).config.iterations; got != 1 {
		t.Fatalf("iterations 9 → right = %d, want wrap to 1", got)
	}
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := updated.(Model).config.iterations; got != 9 {
		t.Fatalf("iterations 1 → left = %d, want wrap to 9", got)
	}

	um = updated.(Model)
	um.config.cursorAt(rowAdvSetting, advIPVersion)
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(Model).config.ipVersion; got != "v6" {
		t.Fatalf("ipVersion v4 → enter = %q, want v6", got)
	}

	um = updated.(Model)
	um.config.cursorAt(rowAdvSetting, advTimeout)
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).config.timeoutValue(); got != 10*time.Minute {
		t.Fatalf("timeout 5m → right = %v, want 10m", got)
	}

	um = updated.(Model)
	um.config.cursorAt(rowAdvSetting, advCatalogSource)
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).config.catalogSource(); got != "auto" {
		t.Fatalf("catalog embedded → right = %q, want auto", got)
	}
}

func TestConfigStartEmitsKindBasedOnSections(t *testing.T) {
	// Default full checklist → checkup report.
	m := scrollTestModel(t, pageConfig, nil)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a cmd")
	}
	start, ok := cmd().(checkupStartMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want checkupStartMsg", cmd())
	}
	norm, err := checkup.NormalizeOptions(start.opts)
	if err != nil {
		t.Fatal(err)
	}
	if !norm.Sections.Media || !norm.Sections.Hardware {
		t.Fatalf("checkup start sections = %+v, want full checklist", norm.Sections)
	}

	// Untick everything except hardware → bare benchmark run.
	um := updated.(Model)
	for digit := '2'; digit <= '9'; digit++ {
		updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{digit}})
		um = updated.(Model)
	}
	updated, cmd = um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a cmd")
	}
	hwStart, ok := cmd().(hardwareStartMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want hardwareStartMsg", cmd())
	}
	if hwStart.opts.Iterations != 3 || hwStart.opts.Engine != "external" {
		t.Fatalf("start opts = %+v", hwStart.opts)
	}
	if _, isModel := updated.(Model); !isModel {
		t.Fatalf("update returned %T", updated)
	}
}

func TestConfigStartDisabledWithoutSections(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	for digit := '1'; digit <= '9'; digit++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{digit}})
		m = updated.(Model)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter with no sections must not start anything")
	}
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("update returned %T", updated)
	}
	if view := um.View(); !strings.Contains(view, i18n.T("tui.config.startDisabled")) {
		t.Fatalf("start row should render disabled:\n%s", view)
	}
}

// TestConfigFocusedLineMatchesRender pins configFocusedLine's layout math to
// viewConfig so focus-follow scrolling tracks the real rows.
func TestConfigFocusedLineMatchesRender(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.height = 40 // tall enough that no clipping hides the focused row
	m.config.advancedOpen = true

	cases := []configRow{
		{kind: rowStart},
		{kind: rowSection, index: 0},
		{kind: rowSection, index: 4},
		{kind: rowSection, index: 8},
		{kind: rowAdvanced},
		{kind: rowAdvSetting, index: 0},
		{kind: rowAdvSetting, index: advCount - 1},
	}
	for _, row := range cases {
		if !m.config.cursorAt(row.kind, row.index) {
			t.Fatalf("row %+v not found", row)
		}
		line, ok := configFocusedLine(m)
		if !ok {
			t.Fatalf("focused line unknown for %+v", row)
		}
		viewLine := line + lipgloss.Height(renderHeader(m)) + 1 // body top padding
		lines := strings.Split(m.View(), "\n")
		if viewLine >= len(lines) {
			t.Fatalf("focused line %d beyond view for %+v", viewLine, row)
		}
		if !strings.Contains(lines[viewLine], "▎") {
			t.Fatalf("focused band not on line %d for %+v:\n%s", viewLine, row, m.View())
		}
	}
}

func TestConfigRowsFit80x24BothLocales(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		t.Run(lang, func(t *testing.T) {
			if !i18n.SetLang(lang) {
				t.Fatalf("SetLang(%q) failed", lang)
			}
			t.Cleanup(func() { i18n.SetLang("en") })

			m := scrollTestModel(t, pageConfig, nil)
			m.config.advancedOpen = true
			for cursor := 0; cursor < len(m.config.visibleRows()); cursor++ {
				mm := m
				mm.config.cursor = cursor
				assertRenderBounds(t, mm.View(), 80, 24)
			}
		})
	}
}

func TestCheckupDurationEstimates(t *testing.T) {
	s := newConfigState()

	rough := estimateCheckupDuration(s, historyStats{})
	if rough <= 0 {
		t.Fatalf("rough estimate should be positive, got %v", rough)
	}

	stats := historyStats{
		avg: map[checkup.SectionID]time.Duration{
			checkup.SectionHardware: 60 * time.Second,
			checkup.SectionSpeed:    20 * time.Second,
		},
		samples: 3,
	}
	if withHistory := estimateCheckupDuration(s, stats); withHistory <= 0 || withHistory >= rough {
		t.Fatalf("history estimate %v should beat rough %v on full checklist", withHistory, rough)
	}
}
