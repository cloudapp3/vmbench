package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/cloudapp3/vmbench/suite"
)

func TestNewConfigStateDefaultsMatchCLI(t *testing.T) {
	s := newConfigState()

	// The CLI default is hardware only, so the TUI must open the same way.
	if got := s.presetIDs[s.preset]; got != configPresetHardware {
		t.Fatalf("selected preset = %q, want %q", got, configPresetHardware)
	}
	if s.sections != (suite.SectionSelector{Hardware: true}) {
		t.Fatalf("sections = %+v, want hardware only", s.sections)
	}
	if s.iterations != 3 {
		t.Fatalf("default iterations = %d, want 3", s.iterations)
	}
	if s.filterExpr() != "" {
		t.Fatalf("default filter = %q, want none", s.filterExpr())
	}
	if strings.Join(s.selectedTools(), ",") != strings.Join(catalog.DefaultHardwareTools(), ",") {
		t.Fatalf("default tools = %v, want %v", s.selectedTools(), catalog.DefaultHardwareTools())
	}

	if norm, err := vmbench.NormalizeOptions(s.buildRunOptions()); err != nil {
		t.Fatalf("default run options must normalize: %v", err)
	} else if norm.Iterations != 3 || norm.Engine != "external" {
		t.Fatalf("normalized run options = %+v", norm)
	}
}

func TestConfigPresetApply(t *testing.T) {
	s := newConfigState()

	// Real presets apply their sections and IP version.
	s.preset = 2 // quick
	s.applyPreset()
	quick, ok := suite.LookupPreset("quick")
	if !ok {
		t.Fatal("quick preset not found")
	}
	if s.sections != quick.Sections || s.ipVersion != quick.IPVersion {
		t.Fatalf("sections = %+v ip = %s, want quick %+v/%s", s.sections, s.ipVersion, quick.Sections, quick.IPVersion)
	}

	// Custom keeps current picks.
	custom := suite.SectionSelector{Hardware: true, Mail: true}
	s.preset = 1
	s.sections = custom
	s.applyPreset()
	if s.sections != custom {
		t.Fatalf("custom preset changed sections: %+v", s.sections)
	}

	// Hardware resets to the bare hardware selection.
	s.preset = 0
	s.applyPreset()
	if s.sections != (suite.SectionSelector{Hardware: true}) {
		t.Fatalf("hardware preset sections = %+v", s.sections)
	}
	if s.suitePreset() != "" {
		t.Fatalf("suitePreset = %q, want empty for hardware", s.suitePreset())
	}
	s.preset = 2
	if s.suitePreset() != "quick" {
		t.Fatalf("suitePreset = %q, want quick", s.suitePreset())
	}
}

func TestConfigVisibleFields(t *testing.T) {
	base := []configField{fieldPreset, fieldSections, fieldRuntime}
	tail := []configField{fieldAdvanced, fieldStart}

	s := newConfigState()
	want := slices.Concat(base, []configField{fieldHardwareTools, fieldFilter}, tail)
	if got := s.visibleFields(); !slices.Equal(got, want) {
		t.Fatalf("hardware-only visible = %v, want %v", got, want)
	}

	s = newConfigState()
	s.sections = suite.DefaultSections()
	want = slices.Concat(base, []configField{fieldHardwareTools, fieldFilter, fieldSpeedProviders, fieldRoutePresets, fieldMediaSets, fieldIPSources}, tail)
	if got := s.visibleFields(); !slices.Equal(got, want) {
		t.Fatalf("all-sections visible = %v, want %v", got, want)
	}

	s = newConfigState()
	s.sections = suite.SectionSelector{Hardware: true, Route: true, Media: true}
	want = slices.Concat(base, []configField{fieldHardwareTools, fieldFilter, fieldRoutePresets, fieldMediaSets}, tail)
	if got := s.visibleFields(); !slices.Equal(got, want) {
		t.Fatalf("route+media visible = %v, want %v", got, want)
	}
}

func TestConfigFocusSnapsAfterPresetSwitch(t *testing.T) {
	s := newConfigState()
	s.preset = 2 // quick: speed visible
	s.applyPreset()
	s.field = fieldSpeedProviders

	s.preset = 0 // back to hardware only: speed card disappears
	s.applyPreset()
	if got := s.field; slices.Contains(s.visibleFields(), fieldSpeedProviders) || got == fieldSpeedProviders {
		t.Fatalf("focus stayed on hidden field: %v", got)
	}
	if !slices.Contains(s.visibleFields(), s.field) {
		t.Fatalf("snapped focus %v is not visible", s.field)
	}
}

func TestConfigNavigationSkipsHiddenFields(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	// Hardware-only defaults: preset → sections → runtime → tools → filter →
	// advanced → start (speed/route/media/ip cards are hidden).
	m.config.field = fieldRuntime
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	um := updated.(Model)
	if um.config.field != fieldHardwareTools {
		t.Fatalf("tab from runtime = %v, want hardwareTools", um.config.field)
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyTab})
	um = updated.(Model)
	if um.config.field != fieldFilter {
		t.Fatalf("tab from tools = %v, want filter", um.config.field)
	}
	// Wrap from the last field back to the first.
	um.config.field = fieldStart
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyDown})
	um = updated.(Model)
	if um.config.field != fieldPreset {
		t.Fatalf("tab wrap from start = %v, want preset", um.config.field)
	}
}

func TestConfigDigitTogglesSectionAndForcesCustom(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	um := updated.(Model)
	if !um.config.sections.Speed || !um.config.sections.Hardware {
		t.Fatalf("sections after digit toggle = %+v, want hardware+speed", um.config.sections)
	}
	if got := um.config.presetIDs[um.config.preset]; got != configPresetCustom {
		t.Fatalf("digit toggle must force custom preset, got %q", got)
	}

	// Digit 1 toggles hardware off, leaving speed only.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	um = updated.(Model)
	if um.config.sections.Hardware || !um.config.sections.Speed {
		t.Fatalf("sections after second toggle = %+v, want speed only", um.config.sections)
	}

	// Digits land in text fields instead of toggling sections.
	um.config.field = fieldAdvanced
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	um = updated.(Model)
	if um.config.iperfHost != "3" {
		t.Fatalf("digit should type into advanced field, got %q", um.config.iperfHost)
	}
}

func TestConfigStartEmitsKindBasedOnSections(t *testing.T) {
	// Hardware-only default → hardwareStartMsg (run report).
	m := scrollTestModel(t, pageConfig, nil)
	m.config.field = fieldStart
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a cmd emitting hardwareStartMsg")
	}
	start, ok := cmd().(hardwareStartMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want hardwareStartMsg", cmd())
	}
	if start.opts.Iterations != 3 || start.opts.Engine != "external" {
		t.Fatalf("start opts = %+v", start.opts)
	}
	if _, isModel := updated.(Model); !isModel {
		t.Fatalf("update returned %T", updated)
	}

	// Adding a suite section → suiteStartMsg (suite report).
	m.config.preset = 1 // custom
	m.config.sections = suite.SectionSelector{Hardware: true, Speed: true}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a cmd emitting suiteStartMsg")
	}
	if _, ok := cmd().(suiteStartMsg); !ok {
		t.Fatalf("cmd() returned %T, want suiteStartMsg", cmd())
	}
}

func TestConfigStartDisabledWithoutSections(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.config.sections = suite.SectionSelector{}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter with no sections must not start anything")
	}
	if _, ok := updated.(Model); !ok {
		t.Fatalf("update returned %T", updated)
	}
	if view := updated.(Model).View(); !strings.Contains(view, i18n.T("tui.config.startDisabled")) {
		t.Fatalf("start button should render disabled:\n%s", view)
	}
}

func TestConfigInvalidRegexShowsToast(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.config.field = fieldStart
	m.config.filterChip = configFilterChipCustom
	m.config.filterText = "("

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um := updated.(Model)
	if cmd == nil {
		t.Fatal("invalid regex should surface a toast cmd")
	}
	if !um.toast.Active() {
		t.Fatal("invalid regex should show a toast")
	}
}

func TestConfigCustomFilterTextEntry(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.config.field = fieldFilter
	m.config.filterChip = configFilterChipCustom

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fio")})
	um := updated.(Model)
	if um.config.filterText != "fio" {
		t.Fatalf("filterText = %q, want fio", um.config.filterText)
	}

	// "q" must be typed, not quit.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	um = updated.(Model)
	if um.config.filterText != "fioq" {
		t.Fatalf("filterText = %q, want fioq", um.config.filterText)
	}

	// "?" must not open help while typing; it is typed into the field.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	um = updated.(Model)
	if um.page != pageConfig {
		t.Fatalf("? leaked during text entry, page = %d", um.page)
	}
	if um.config.filterText != "fioq?" {
		t.Fatalf("filterText = %q, want fioq?", um.config.filterText)
	}

	// Backspace removes one rune.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	um = updated.(Model)
	if um.config.filterText != "fioq" {
		t.Fatalf("filterText = %q after backspace, want fioq", um.config.filterText)
	}

	// plannedWorkloads honors the regex.
	um.config.filterText = "fio"
	planned := um.config.plannedWorkloads()
	if len(planned) == 0 {
		t.Fatal("fio filter should plan fio workloads")
	}
	for _, d := range planned {
		if !strings.Contains(d.Name, "fio") {
			t.Fatalf("fio filter planned non-fio workload %q", d.Name)
		}
	}
}

func TestConfigChipFilterPlansCategory(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)
	m.config.field = fieldFilter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	um := updated.(Model)
	if um.config.filterChip != 1 || um.config.buildRunOptions().Filter != "CPU" {
		t.Fatalf("filter chip = %d expr = %q, want 1/CPU", um.config.filterChip, um.config.buildRunOptions().Filter)
	}
	for _, d := range um.config.plannedWorkloads() {
		if d.Category != "CPU" {
			t.Fatalf("CPU filter planned non-CPU workload %q (%s)", d.Name, d.Category)
		}
	}
}

func TestConfigBuildsCanonicalSuiteOptions(t *testing.T) {
	state := newConfigState()
	state.preset = 1
	state.sections = suite.SectionSelector{Hardware: true, Ping: true, Reachability: true}
	state.iterations = 5
	state.ipVersion = "dual"
	state.timeoutIndex = 2
	state.iperfHost = "iperf.example:5201"
	state.catalogSource = nodecatalog.SourceEmbedded
	state.catalogRevision = ""
	for id := range state.hardwareTools {
		state.hardwareTools[id] = false
	}
	state.hardwareTools[catalog.HardwareToolOpenSSL] = true

	raw := state.buildSuiteOptions()
	if raw.Iterations != 5 || raw.IPVersion != "dual" || raw.Timeout != 10*time.Minute {
		t.Fatalf("runtime options = %+v", raw)
	}
	if !slices.Equal(raw.HardwareTools, []string{catalog.HardwareToolOpenSSL}) || !slices.Equal(raw.IperfHosts, []string{"iperf.example:5201"}) {
		t.Fatalf("tool options = %+v", raw)
	}
	norm, err := suite.NormalizeOptions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if norm.CatalogRevision == "" || norm.ResolvedCatalog == nil || len(norm.NodeIDs) == 0 {
		t.Fatalf("normalized TUI provenance = %+v", norm)
	}
}

func TestConfigDefaultsForOptionalSections(t *testing.T) {
	state := newConfigState()
	if !state.mediaSets[suite.DefaultMediaSet()] {
		t.Errorf("default media set %s should be selected", suite.DefaultMediaSet())
	}
	if !state.ipSources[suite.IPSourceBuiltin] {
		t.Error("builtin IP source should be selected by default")
	}
	if state.ipSources[suite.IPSourceSecurityCheck] {
		t.Error("securitycheck should be opt-in only")
	}
	for _, id := range []string{suite.SpeedProviderChinaISP, suite.SpeedProviderSpeedtestISP} {
		if !slices.Contains(state.speedIDs, id) {
			t.Errorf("speed provider %s missing from TUI list", id)
		}
	}
	if !state.speedProviders[suite.SpeedProviderCloudflare] {
		t.Error("Cloudflare should be selected by default")
	}
	if state.speedProviders[suite.SpeedProviderIperf3] {
		t.Error("iperf3 should not be selected without a host")
	}
}

func TestToggleMediaSetMutualExclusion(t *testing.T) {
	state := newConfigState()
	state.toggleMediaSet("jp")
	if state.mediaSets[suite.DefaultMediaSet()] {
		t.Error("selecting a region must clear the all-platform set")
	}
	if !state.mediaSets["jp"] {
		t.Fatal("jp should stay selected")
	}
	state.toggleMediaSet("kr")
	if !state.mediaSets["jp"] || !state.mediaSets["kr"] {
		t.Error("region sets must combine")
	}
	state.toggleMediaSet(suite.DefaultMediaSet())
	for _, id := range state.mediaIDs {
		if id != suite.DefaultMediaSet() && state.mediaSets[id] {
			t.Errorf("selecting all must clear %s", id)
		}
	}
}

func TestBuildSuiteOptionsCarriesMediaSetAndIPSources(t *testing.T) {
	state := newConfigState()
	state.sections = suite.SectionSelector{Media: true, IPQuality: true}
	state.toggleMediaSet("jp")
	state.toggleMediaSet("kr")
	state.ipSources[suite.IPSourceSecurityCheck] = true

	norm, err := suite.NormalizeOptions(state.buildSuiteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if norm.MediaSet != "jp,kr" {
		t.Fatalf("normalized MediaSet = %q, want jp,kr", norm.MediaSet)
	}
	if strings.Join(norm.IPSources, ",") != "builtin,securitycheck" {
		t.Fatalf("normalized IPSources = %v", norm.IPSources)
	}
}

func TestConfigPreflightAndRender(t *testing.T) {
	m := scrollTestModel(t, pageConfig, nil)

	updated, _ := m.Update(missingToolsMsg{missing: []string{"mbw"}})
	um := updated.(Model)
	if !um.config.missingOK || len(um.config.missing) != 1 {
		t.Fatalf("preflight state = %+v", um.config)
	}
	// The preflight card lives in the full grid; render tall enough for it.
	um.width, um.height = 100, 50
	view := um.View()
	if !strings.Contains(view, "mbw") {
		t.Fatalf("preflight card should list missing tool:\n%s", view)
	}

	// Every visible field focus must fit 80x24 in both locales.
	for _, lang := range []string{"en", "zh-CN"} {
		if !i18n.SetLang(lang) {
			t.Fatalf("SetLang(%q) failed", lang)
		}
		t.Cleanup(func() { i18n.SetLang("en") })
		for _, f := range newConfigState().visibleFields() {
			mm := scrollTestModel(t, pageConfig, nil)
			mm.config.field = f
			mm.config.missingOK = true
			assertRenderBounds(t, mm.View(), 80, 24)
		}
	}
}

func TestSummaryCardEstimates(t *testing.T) {
	s := newConfigState()

	rough := estimateSuiteDuration(s, historyStats{})
	if rough <= 0 {
		t.Fatalf("rough estimate should be positive, got %v", rough)
	}

	stats := historyStats{
		avg: map[suite.SectionID]time.Duration{
			suite.SectionHardware: 60 * time.Second,
			suite.SectionSpeed:    20 * time.Second,
		},
		samples: 3,
	}
	s.preset = 2 // quick: hardware + network evidence + speed
	s.applyPreset()
	withHistory := estimateSuiteDuration(s, stats)
	if withHistory <= 0 || withHistory >= rough {
		t.Fatalf("history estimate %v should beat rough %v on quick defaults", withHistory, rough)
	}

	// Card renders within 80 cells in both locales.
	card := suiteSummaryCard(s, stats, catalogStats{loaded: true, download: 15, route: 25, ping: 25, isp: 12}, 76)
	for i, line := range strings.Split(card, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("summary card line %d width %d > 80: %q", i, w, line)
		}
	}
}
