package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type configField int

const (
	fieldPreset configField = iota
	fieldSections
	fieldRuntime
	fieldHardwareTools
	fieldFilter
	fieldSpeedProviders
	fieldRoutePresets
	fieldMediaSets
	fieldIPSources
	fieldAdvanced
	fieldStart
)

// Pseudo-preset IDs the config page prepends to suite.PresetIDs().
const (
	configPresetHardware = "hardware"
	configPresetCustom   = "custom"
)

// configFilterChips maps quick filter presets to the regex the runner applies
// to workload Name/Category ("" = no filter).
var configFilterChips = []struct {
	id       int
	labelKey string
	expr     string
}{
	{0, "tui.config.filterAll", ""},
	{1, "tui.config.filterCPU", "CPU"},
	{2, "tui.config.filterDisk", "Disk"},
	{3, "tui.config.filterMemory", "Memory"},
	{4, "tui.config.filterCustom", ""},
}

const configFilterChipCustom = 4

type configState struct {
	field           configField
	preset          int
	presetIDs       []string
	sections        suite.SectionSelector
	sectionCursor   int
	sectionIDs      []suite.SectionID
	speedProviders  map[string]bool
	speedCursor     int
	speedIDs        []string
	routePresets    map[string]bool
	routeCursor     int
	routeIDs        []string
	mediaSets       map[string]bool
	mediaCursor     int
	mediaIDs        []string
	ipSources       map[string]bool
	ipSourceCursor  int
	ipSourceIDs     []string
	runtimeCursor   int
	iterations      int
	ipVersion       string
	timeoutIndex    int
	timeouts        []time.Duration
	hardwareTools   map[string]bool
	hardwareCursor  int
	hardwareIDs     []string
	advancedCursor  int
	iperfHost       string
	catalogSource   string
	catalogRevision string
	// Hardware-only extras: workload filter chips plus the tool preflight
	// result. Only consulted while the hardware section is enabled.
	filterChip int
	filterText string
	missing    []string
	missingOK  bool // preflight result present
}

func newConfigState() configState {
	presetIDs := append([]string{configPresetHardware, configPresetCustom}, suite.PresetIDs()...)
	speedIDs := suite.SpeedProviderIDs()
	speed := map[string]bool{}
	for _, id := range suite.DefaultSpeedProviders() {
		speed[id] = true
	}
	routeSpecs := suite.RoutePresets()
	routeIDs := make([]string, 0, len(routeSpecs))
	for _, spec := range routeSpecs {
		routeIDs = append(routeIDs, spec.ID)
	}
	route := map[string]bool{}
	for _, id := range suite.DefaultRoutePresets() {
		route[id] = true
	}
	media := map[string]bool{}
	for _, id := range suite.MediaSets() {
		media[id] = id == suite.DefaultMediaSet()
	}
	ipSources := map[string]bool{}
	for _, id := range suite.IPSourceIDs() {
		ipSources[id] = id == suite.IPSourceBuiltin
	}
	hardwareIDs := catalog.HardwareToolIDs()
	hardware := map[string]bool{}
	for _, id := range catalog.DefaultHardwareTools() {
		hardware[id] = true
	}
	return configState{
		// Hardware only, matching the CLI default: no flags means the bare
		// hardware benchmark.
		preset:    0,
		presetIDs: presetIDs,
		sections:  suite.SectionSelector{Hardware: true},
		sectionIDs: []suite.SectionID{
			suite.SectionHardware, suite.SectionNetworkInfo, suite.SectionRoute, suite.SectionPing,
			suite.SectionSpeed, suite.SectionIPQuality, suite.SectionReachability, suite.SectionMail, suite.SectionMedia,
		},
		speedProviders: speed,
		speedIDs:       speedIDs,
		routePresets:   route,
		routeIDs:       routeIDs,
		mediaSets:      media,
		mediaIDs:       suite.MediaSets(),
		ipSources:      ipSources,
		ipSourceIDs:    suite.IPSourceIDs(),
		iterations:     3,
		ipVersion:      "v4",
		timeoutIndex:   1,
		timeouts:       []time.Duration{time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute},
		hardwareTools:  hardware,
		hardwareIDs:    hardwareIDs,
		catalogSource:  nodecatalog.SourceEmbedded,
	}
}

func (s *configState) sectionGet(i int) bool {
	switch s.sectionIDs[i] {
	case suite.SectionHardware:
		return s.sections.Hardware
	case suite.SectionNetworkInfo:
		return s.sections.NetworkInfo
	case suite.SectionRoute:
		return s.sections.Route
	case suite.SectionPing:
		return s.sections.Ping
	case suite.SectionSpeed:
		return s.sections.Speed
	case suite.SectionIPQuality:
		return s.sections.IPQuality
	case suite.SectionReachability:
		return s.sections.Reachability
	case suite.SectionMail:
		return s.sections.Mail
	case suite.SectionMedia:
		return s.sections.Media
	}
	return false
}

func (s *configState) sectionToggle(i int) {
	switch s.sectionIDs[i] {
	case suite.SectionHardware:
		s.sections.Hardware = !s.sections.Hardware
	case suite.SectionNetworkInfo:
		s.sections.NetworkInfo = !s.sections.NetworkInfo
	case suite.SectionRoute:
		s.sections.Route = !s.sections.Route
	case suite.SectionPing:
		s.sections.Ping = !s.sections.Ping
	case suite.SectionSpeed:
		s.sections.Speed = !s.sections.Speed
	case suite.SectionIPQuality:
		s.sections.IPQuality = !s.sections.IPQuality
	case suite.SectionReachability:
		s.sections.Reachability = !s.sections.Reachability
	case suite.SectionMail:
		s.sections.Mail = !s.sections.Mail
	case suite.SectionMedia:
		s.sections.Media = !s.sections.Media
	}
}

// toggleMediaSet flips one media set selection. Selecting "all" clears the
// region picks; picking any region clears "all" so the value stays meaningful.
func (s *configState) toggleMediaSet(id string) {
	s.mediaSets[id] = !s.mediaSets[id]
	if !s.mediaSets[id] {
		return
	}
	if id == suite.DefaultMediaSet() {
		for _, other := range s.mediaIDs {
			if other != id {
				s.mediaSets[other] = false
			}
		}
		return
	}
	s.mediaSets[suite.DefaultMediaSet()] = false
}

// textEntryActive reports whether a raw-text field currently has focus (the
// advanced provenance inputs, or the custom filter chip).
func (s configState) textEntryActive() bool {
	return s.field == fieldAdvanced ||
		(s.field == fieldFilter && s.filterChip == configFilterChipCustom)
}

// visibleFields is the tab order pruned to cards that currently render. Field
// navigation must never land on a hidden card, so every step goes through
// this list.
func (s configState) visibleFields() []configField {
	fields := []configField{fieldPreset, fieldSections, fieldRuntime}
	if s.sections.Hardware {
		fields = append(fields, fieldHardwareTools, fieldFilter)
	}
	if s.sections.Speed {
		fields = append(fields, fieldSpeedProviders)
	}
	if s.sections.Route || s.sections.Ping {
		fields = append(fields, fieldRoutePresets)
	}
	if s.sections.Media {
		fields = append(fields, fieldMediaSets)
	}
	if s.sections.IPQuality {
		fields = append(fields, fieldIPSources)
	}
	return append(fields, fieldAdvanced, fieldStart)
}

// stepField moves focus by delta through the visible fields, wrapping.
func (s *configState) stepField(delta int) {
	fields := s.visibleFields()
	idx := 0
	for i, f := range fields {
		if f == s.field {
			idx = i
			break
		}
	}
	s.field = fields[(idx+delta+len(fields))%len(fields)]
}

// snapFocus pulls focus back to a visible field after a visibility change
// (section toggle, preset switch). It is a no-op while focus is already on a
// visible card.
func (s *configState) snapFocus() {
	for _, f := range s.visibleFields() {
		if f == s.field {
			return
		}
	}
	s.field = fieldPreset
}

// applyPreset rewrites the section selection for the selected preset. The
// hardware pseudo-preset matches the CLI default; custom keeps current picks.
func (s *configState) applyPreset() {
	switch s.presetIDs[s.preset] {
	case configPresetHardware:
		s.sections = suite.SectionSelector{Hardware: true}
	case configPresetCustom:
		// keep current selections
	default:
		if spec, ok := suite.LookupPreset(s.presetIDs[s.preset]); ok {
			s.sections = spec.Sections
			if strings.TrimSpace(spec.IPVersion) != "" {
				s.ipVersion = spec.IPVersion
			}
		}
	}
	s.missingOK = false
	s.snapFocus()
}

func (s configState) presetDisplayName() string {
	id := s.presetIDs[s.preset]
	switch id {
	case configPresetHardware:
		return i18n.T("tui.config.hardwarePreset")
	case configPresetCustom:
		return i18n.T("tui.config.customPreset")
	}
	if spec, ok := suite.LookupPreset(id); ok {
		return spec.LocalizedName()
	}
	return id
}

// suitePreset reports the preset ID to carry into suite.Options: only real
// suite presets are passed through; hardware/custom selections are expressed
// by their section selector instead.
func (s configState) suitePreset() string {
	id := s.presetIDs[s.preset]
	if _, ok := suite.LookupPreset(id); ok {
		return id
	}
	return ""
}

func (s configState) selectedTools() []string {
	var out []string
	for _, id := range s.hardwareIDs {
		if s.hardwareTools[id] {
			out = append(out, id)
		}
	}
	return out
}

func (s configState) filterExpr() string {
	if s.filterChip == configFilterChipCustom {
		return strings.TrimSpace(s.filterText)
	}
	return configFilterChips[s.filterChip].expr
}

func (s configState) buildRunOptions() vmbench.Options {
	return vmbench.Options{
		Engine:        "external",
		Iterations:    s.iterations,
		Filter:        s.filterExpr(),
		HardwareTools: s.selectedTools(),
	}
}

func (s configState) buildSuiteOptions() suite.Options {
	var providers []string
	for _, id := range s.speedIDs {
		if s.speedProviders[id] {
			providers = append(providers, id)
		}
	}
	var routes []string
	for _, id := range s.routeIDs {
		if s.routePresets[id] {
			routes = append(routes, id)
		}
	}
	var mediaSets []string
	for _, id := range s.mediaIDs {
		if s.mediaSets[id] {
			mediaSets = append(mediaSets, id)
		}
	}
	var ipSources []string
	for _, id := range s.ipSourceIDs {
		if s.ipSources[id] {
			ipSources = append(ipSources, id)
		}
	}
	var hardwareTools []string
	for _, id := range s.hardwareIDs {
		if s.hardwareTools[id] {
			hardwareTools = append(hardwareTools, id)
		}
	}
	timeout := 5 * time.Minute
	if s.timeoutIndex >= 0 && s.timeoutIndex < len(s.timeouts) {
		timeout = s.timeouts[s.timeoutIndex]
	}
	opts := suite.Options{
		Iterations:      s.iterations,
		Sections:        s.sections,
		Preset:          s.suitePreset(),
		SpeedProviders:  providers,
		RoutePresets:    routes,
		HardwareTools:   hardwareTools,
		IPVersion:       s.ipVersion,
		MediaSet:        strings.Join(mediaSets, ","),
		IPSources:       ipSources,
		Timeout:         timeout,
		CatalogSource:   strings.TrimSpace(s.catalogSource),
		CatalogRevision: strings.TrimSpace(s.catalogRevision),
	}
	if host := strings.TrimSpace(s.iperfHost); host != "" {
		opts.IperfHosts = []string{host}
	}
	return opts
}

// plannedWorkloads lists the workload rows the hardware run will actually
// execute, mirroring the runner's filter so the running page never shows
// ghost rows.
func (s configState) plannedWorkloads() []catalog.Definition {
	defs := catalog.ExternalHardwareDefinitionsForTools("", s.selectedTools())
	expr := s.filterExpr()
	if expr == "" {
		return defs
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return defs
	}
	var out []catalog.Definition
	for _, d := range defs {
		if re.MatchString(d.Name) || re.MatchString(d.Category) {
			out = append(out, d)
		}
	}
	return out
}

type hardwareStartMsg struct{ opts vmbench.Options }
type missingToolsMsg struct{ missing []string }

// configMissingToolsCmd re-runs the hardware tool preflight in the
// background; missingToolsMsg carries the result back to the card.
func configMissingToolsCmd(s configState) tea.Cmd {
	return func() tea.Msg {
		expr := s.filterExpr()
		var re *regexp.Regexp
		if expr != "" {
			if compiled, err := regexp.Compile(expr); err == nil {
				re = compiled
			}
		}
		return missingToolsMsg{missing: catalog.MissingHardwareToolsForFilter(s.selectedTools(), re)}
	}
}

func updateConfig(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.config
	switch msg.String() {
	case "esc":
		m.page = pageDashboard
		return m, nil
	case "q":
		if !s.textEntryActive() {
			return m, tea.Quit
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Quick section toggle by digit; text fields keep runes.
		if !s.textEntryActive() {
			if digit := int(msg.String()[0] - '1'); digit < len(s.sectionIDs) {
				s.sectionToggle(digit)
				s.preset = 1 // custom
				s.missingOK = false
				s.snapFocus()
				if s.sections.Hardware {
					return m, configMissingToolsCmd(*s)
				}
			}
			return m, nil
		}
	case "tab", "down":
		s.stepField(1)
		return m, nil
	case "shift+tab", "up":
		s.stepField(-1)
		return m, nil
	case "left", "h":
		switch s.field {
		case fieldPreset:
			if s.preset > 0 {
				s.preset--
				s.applyPreset()
				if s.sections.Hardware {
					return m, configMissingToolsCmd(*s)
				}
			}
		case fieldSections:
			if s.sectionCursor > 0 {
				s.sectionCursor--
			}
		case fieldFilter:
			s.filterChip = (s.filterChip + len(configFilterChips) - 1) % len(configFilterChips)
			s.missingOK = false
			return m, configMissingToolsCmd(*s)
		case fieldSpeedProviders:
			if s.speedCursor > 0 {
				s.speedCursor--
			}
		case fieldRoutePresets:
			if s.routeCursor > 0 {
				s.routeCursor--
			}
		case fieldMediaSets:
			if s.mediaCursor > 0 {
				s.mediaCursor--
			}
		case fieldIPSources:
			if s.ipSourceCursor > 0 {
				s.ipSourceCursor--
			}
		case fieldRuntime:
			if s.runtimeCursor > 0 {
				s.runtimeCursor--
			}
		case fieldHardwareTools:
			if s.hardwareCursor > 0 {
				s.hardwareCursor--
			}
		case fieldAdvanced:
			if s.advancedCursor > 0 {
				s.advancedCursor--
			}
		}
		return m, nil
	case "right", "l":
		switch s.field {
		case fieldPreset:
			if s.preset < len(s.presetIDs)-1 {
				s.preset++
				s.applyPreset()
				if s.sections.Hardware {
					return m, configMissingToolsCmd(*s)
				}
			}
		case fieldSections:
			if s.sectionCursor < len(s.sectionIDs)-1 {
				s.sectionCursor++
			}
		case fieldFilter:
			s.filterChip = (s.filterChip + 1) % len(configFilterChips)
			s.missingOK = false
			return m, configMissingToolsCmd(*s)
		case fieldSpeedProviders:
			if s.speedCursor < len(s.speedIDs)-1 {
				s.speedCursor++
			}
		case fieldRoutePresets:
			if s.routeCursor < len(s.routeIDs)-1 {
				s.routeCursor++
			}
		case fieldMediaSets:
			if s.mediaCursor < len(s.mediaIDs)-1 {
				s.mediaCursor++
			}
		case fieldIPSources:
			if s.ipSourceCursor < len(s.ipSourceIDs)-1 {
				s.ipSourceCursor++
			}
		case fieldRuntime:
			if s.runtimeCursor < 2 {
				s.runtimeCursor++
			}
		case fieldHardwareTools:
			if s.hardwareCursor < len(s.hardwareIDs)-1 {
				s.hardwareCursor++
			}
		case fieldAdvanced:
			if s.advancedCursor < 2 {
				s.advancedCursor++
			}
		}
		return m, nil
	case " ", "x":
		if !s.textEntryActive() {
			switch s.field {
			case fieldSections:
				s.sectionToggle(s.sectionCursor)
				s.preset = 1 // custom
				s.missingOK = false
				s.snapFocus()
				if s.sections.Hardware {
					return m, configMissingToolsCmd(*s)
				}
			case fieldSpeedProviders:
				id := s.speedIDs[s.speedCursor]
				s.speedProviders[id] = !s.speedProviders[id]
			case fieldRoutePresets:
				id := s.routeIDs[s.routeCursor]
				s.routePresets[id] = !s.routePresets[id]
			case fieldMediaSets:
				id := s.mediaIDs[s.mediaCursor]
				s.toggleMediaSet(id)
			case fieldIPSources:
				id := s.ipSourceIDs[s.ipSourceCursor]
				s.ipSources[id] = !s.ipSources[id]
			case fieldRuntime:
				s.cycleRuntimeValue()
			case fieldHardwareTools:
				id := s.hardwareIDs[s.hardwareCursor]
				s.hardwareTools[id] = !s.hardwareTools[id]
				s.missingOK = false
				return m, configMissingToolsCmd(*s)
			}
			return m, nil
		}
	case "backspace", "ctrl+h":
		if s.field == fieldFilter && s.filterChip == configFilterChipCustom {
			if len(s.filterText) > 0 {
				s.filterText = s.filterText[:len(s.filterText)-1]
				s.missingOK = false
			}
			return m, nil
		}
		if s.field == fieldAdvanced {
			s.backspaceAdvanced()
		}
		return m, nil
	case "ctrl+u":
		if s.field == fieldFilter && s.filterChip == configFilterChipCustom {
			s.filterText = ""
			s.missingOK = false
			return m, configMissingToolsCmd(*s)
		}
		if s.field == fieldAdvanced {
			s.clearAdvanced()
		}
		return m, nil
	case "enter":
		if s.field != fieldStart && s.sections.AnyEnabled() {
			s.stepField(1)
			return m, nil
		}
		if !s.sections.AnyEnabled() {
			return m, nil
		}
		// One report-kind rule, shared with the CLI and MCP: exactly the
		// hardware section runs the bare benchmark (run report), anything
		// else runs the suite.
		if s.sections.HardwareOnly() {
			opts, err := vmbench.NormalizeOptions(s.buildRunOptions())
			if err != nil {
				var cmd tea.Cmd
				m.toast, cmd = comp.ShowToast(err.Error(), comp.ToastError, 4*time.Second)
				return m, cmd
			}
			return m, func() tea.Msg { return hardwareStartMsg{opts: opts} }
		}
		norm, err := suite.NormalizeOptions(s.buildSuiteOptions())
		if err != nil {
			var cmd tea.Cmd
			m.toast, cmd = comp.ShowToast(err.Error(), comp.ToastError, 4*time.Second)
			return m, cmd
		}
		return m, func() tea.Msg { return suiteStartMsg{opts: norm} }
	}
	if s.textEntryActive() && (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) {
		if s.field == fieldFilter {
			s.filterText += string(msg.Runes)
			s.missingOK = false
			return m, configMissingToolsCmd(*s)
		}
		s.appendAdvanced(string(msg.Runes))
	}
	return m, nil
}

func (s *configState) cycleRuntimeValue() {
	switch s.runtimeCursor {
	case 0:
		s.iterations++
		if s.iterations > 9 {
			s.iterations = 1
		}
	case 1:
		switch s.ipVersion {
		case "v4":
			s.ipVersion = "v6"
		case "v6":
			s.ipVersion = "dual"
		default:
			s.ipVersion = "v4"
		}
	case 2:
		s.timeoutIndex = (s.timeoutIndex + 1) % len(s.timeouts)
	}
}

func (s *configState) appendAdvanced(value string) {
	switch s.advancedCursor {
	case 0:
		s.iperfHost += value
	case 1:
		s.catalogSource += value
	case 2:
		s.catalogRevision += value
	}
}

func (s *configState) backspaceAdvanced() {
	var value *string
	switch s.advancedCursor {
	case 0:
		value = &s.iperfHost
	case 1:
		value = &s.catalogSource
	case 2:
		value = &s.catalogRevision
	}
	if value == nil {
		return
	}
	runes := []rune(*value)
	if len(runes) > 0 {
		*value = string(runes[:len(runes)-1])
	}
}

func (s *configState) clearAdvanced() {
	switch s.advancedCursor {
	case 0:
		s.iperfHost = ""
	case 1:
		s.catalogSource = ""
	case 2:
		s.catalogRevision = ""
	}
}

func viewConfig(m Model) string {
	t := theme.Active
	s := m.config
	width := m.width

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.config.title"))
	if m.height < 40 {
		return viewConfigCompact(m, title)
	}
	desc := lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.config.description"))

	cardWidth := width - 4
	if cardWidth < 32 {
		cardWidth = 32
	}
	if width >= 100 {
		cardWidth = (width - 8) / 2
	}

	cards := []string{
		configCardPreset(s, cardWidth, s.field == fieldPreset),
		configCardRuntime(s, cardWidth, s.field == fieldRuntime),
		configCardSections(s, cardWidth, s.field == fieldSections),
	}
	if s.sections.Hardware {
		cards = append(cards,
			configCardHardware(s, cardWidth, s.field == fieldHardwareTools),
			configCardFilter(s, cardWidth, s.field == fieldFilter),
			configCardPreflight(s, cardWidth),
		)
	}
	if s.sections.Speed {
		cards = append(cards, configCardSpeed(s, cardWidth, s.field == fieldSpeedProviders))
	}
	if s.sections.Route || s.sections.Ping {
		cards = append(cards, configCardRoute(s, cardWidth, s.field == fieldRoutePresets))
	}
	if s.sections.Media {
		cards = append(cards, configCardMediaSets(s, cardWidth, s.field == fieldMediaSets))
	}
	if s.sections.IPQuality {
		cards = append(cards, configCardIPSources(s, cardWidth, s.field == fieldIPSources))
	}
	cards = append(cards, configCardAdvanced(s, cardWidth, s.field == fieldAdvanced))

	var fields string
	if width >= 100 {
		fields = pairCards(cards, width)
	} else {
		fields = strings.Join(cards, "\n")
	}
	startBtn := configStartButton(s, width-4, s.field == fieldStart)
	summaryCard := suiteSummaryCard(s, m.historyStats, m.catalogStats, width-4)

	help := lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(i18n.T("tui.config.helpFull"))

	parts := []string{title, desc, "", fields, "", summaryCard, "", startBtn, "", help}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

func viewConfigCompact(m Model, title string) string {
	t := theme.Active
	s := m.config
	width := m.width

	summary := i18n.Tf("tui.config.summary", map[string]any{
		"Preset": s.presetDisplayName(), "Sections": len(s.sections.Names()), "IP": s.ipVersion, "Iterations": s.iterations,
	})
	summary = lipgloss.NewStyle().Foreground(t.Muted).Render(truncStr(summary, width-4))

	fieldWidth := width - 4
	if fieldWidth < 32 {
		fieldWidth = 32
	}
	var field string
	switch s.field {
	case fieldPreset:
		field = configCardPreset(s, fieldWidth, true)
	case fieldSections:
		field = configCardSections(s, fieldWidth, true)
	case fieldRuntime:
		field = configCardRuntime(s, fieldWidth, true)
	case fieldHardwareTools:
		field = configCardHardware(s, fieldWidth, true)
	case fieldFilter:
		field = configCardFilter(s, fieldWidth, true)
	case fieldSpeedProviders:
		field = configCardSpeed(s, fieldWidth, true)
	case fieldRoutePresets:
		field = configCardRoute(s, fieldWidth, true)
	case fieldMediaSets:
		field = configCardMediaSets(s, fieldWidth, true)
	case fieldIPSources:
		field = configCardIPSources(s, fieldWidth, true)
	case fieldAdvanced:
		field = configCardAdvanced(s, fieldWidth, true)
	default:
		field = comp.Card{
			Title:   i18n.T("tui.config.ready"),
			Body:    i18n.Tf("tui.config.readyBody", map[string]any{"Count": len(s.sections.Names())}),
			Accent:  t.Success,
			Width:   fieldWidth,
			Focused: true,
		}.Render()
	}

	startBtn := configStartButton(s, width-4, s.field == fieldStart)
	help := lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(i18n.T("tui.config.helpCompact"))
	etaLine := lipgloss.NewStyle().Foreground(t.Secondary).Render(
		truncStr(i18n.Tf("tui.suiteSummary.etaLine", map[string]any{"Duration": formatDuration(estimateSuiteDuration(s, m.historyStats))}), width-4))
	parts := []string{title, summary, etaLine, "", field, "", startBtn, help}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

func configCardPreset(s configState, width int, focus bool) string {
	t := theme.Active

	var pills []string
	for i, id := range s.presetIDs {
		label := id
		switch id {
		case configPresetHardware:
			label = i18n.T("tui.config.hardwarePreset")
		case configPresetCustom:
			label = i18n.T("tui.config.customPreset")
		default:
			if spec, ok := suite.LookupPreset(id); ok {
				label = spec.LocalizedName()
			}
		}
		var st lipgloss.Style
		switch {
		case i == s.preset && focus:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Bg).Background(t.Primary).Padding(0, 2)
		case i == s.preset:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Padding(0, 2)
		default:
			st = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 2)
		}
		pills = append(pills, st.Render(label))
	}
	body := strings.Join(pills, " ")

	accent := t.Primary
	if focus {
		accent = t.BorderFocus
	}
	return comp.Card{
		Title:   i18n.T("tui.config.preset"),
		Body:    body,
		Accent:  accent,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardSections(s configState, width int, focus bool) string {
	t := theme.Active
	var pills []string
	for i, id := range s.sectionIDs {
		on := s.sectionGet(i)
		icon := "☐"
		if on {
			icon = "☑"
		}
		var st lipgloss.Style
		switch {
		case i == s.sectionCursor && focus:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Bg).Background(t.Accent).Padding(0, 1)
		case on:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Success).Padding(0, 1)
		default:
			st = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		}
		pills = append(pills, st.Render(icon+" "+fmt.Sprintf("%d", i+1)+" "+i18n.SectionLabel(string(id))))
	}
	body := strings.Join(pills, " ")
	if !s.sections.AnyEnabled() {
		body += "\n" + lipgloss.NewStyle().Foreground(t.Danger).Italic(true).Render(i18n.T("tui.config.needOneSection"))
	}
	return comp.Card{
		Title:   i18n.T("tui.config.sections"),
		Body:    body,
		Accent:  t.Accent,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardRuntime(s configState, width int, focus bool) string {
	t := theme.Active
	timeout := 5 * time.Minute
	if s.timeoutIndex >= 0 && s.timeoutIndex < len(s.timeouts) {
		timeout = s.timeouts[s.timeoutIndex]
	}
	values := []string{
		i18n.Tf("tui.config.iterations", map[string]any{"Count": s.iterations}),
		i18n.Tf("tui.config.ipVersion", map[string]any{"Version": s.ipVersion}),
		i18n.Tf("tui.config.timeout", map[string]any{"Timeout": timeout.String()}),
	}
	pills := make([]string, 0, len(values))
	for i, value := range values {
		style := lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		if i == s.runtimeCursor && focus {
			style = style.Bold(true).Foreground(t.Bg).Background(t.Primary)
		} else if i == s.runtimeCursor {
			style = style.Bold(true).Foreground(t.Primary)
		}
		pills = append(pills, style.Render(value))
	}
	return comp.Card{Title: i18n.T("tui.config.runtime"), Body: strings.Join(pills, " "), Accent: t.Primary, Width: width, Focused: focus}.Render()
}

func configCardHardware(s configState, width int, focus bool) string {
	t := theme.Active
	pills := make([]string, 0, len(s.hardwareIDs))
	for i, id := range s.hardwareIDs {
		on := s.hardwareTools[id]
		icon := "☐"
		if on {
			icon = "☑"
		}
		style := lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		if i == s.hardwareCursor && focus {
			style = style.Bold(true).Foreground(t.Bg).Background(t.Warning)
		} else if on {
			style = style.Bold(true).Foreground(t.Warning)
		}
		pills = append(pills, style.Render(icon+" "+id))
	}
	return comp.Card{Title: i18n.T("tui.config.hardwareTools"), Body: strings.Join(pills, " "), Accent: t.Warning, Width: width, Focused: focus}.Render()
}

func configCardFilter(s configState, width int, focus bool) string {
	t := theme.Active
	var cells []string
	for i, chip := range configFilterChips {
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(t.Subtle)
		if i == s.filterChip {
			style = style.Foreground(t.Fg).Bold(true).Background(t.Surface)
		}
		cells = append(cells, style.Render(i18n.T(chip.labelKey)))
	}
	body := strings.Join(cells, " ") + "\n"
	if s.filterChip == configFilterChipCustom {
		text := s.filterText
		if text == "" {
			text = i18n.T("tui.config.filterCustomHint")
		}
		style := lipgloss.NewStyle().Foreground(t.Fg)
		if text == "" {
			style = lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
		}
		body += "\n" + style.Render(text) + "▏"
	}
	return comp.Card{
		Title:   i18n.T("tui.config.filter"),
		Body:    body,
		Accent:  t.CategorySystem,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardPreflight(s configState, width int) string {
	t := theme.Active
	title := i18n.T("tui.config.preflight")
	accent := t.CategorySystem
	if !s.missingOK {
		return comp.Card{
			Title:  title,
			Body:   lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(i18n.T("tui.config.preflightPending")),
			Accent: accent,
			Width:  width,
		}.Render()
	}
	if len(s.missing) == 0 {
		return comp.Card{
			Title:  title,
			Body:   lipgloss.NewStyle().Foreground(t.Success).Render("✓ " + i18n.T("tui.config.missingNone")),
			Accent: accent,
			Width:  width,
		}.Render()
	}
	missing := append([]string(nil), s.missing...)
	sort.Strings(missing)
	body := lipgloss.NewStyle().Foreground(t.Warning).Render("! "+i18n.T("tui.config.missingTools")) + "\n" +
		strings.Join(missing, ", ")
	return comp.Card{
		Title:  title,
		Body:   body,
		Accent: t.Warning,
		Width:  width,
	}.Render()
}

func configCardSpeed(s configState, width int, focus bool) string {
	t := theme.Active
	var pills []string
	for i, id := range s.speedIDs {
		on := s.speedProviders[id]
		icon := "☐"
		if on {
			icon = "☑"
		}
		var st lipgloss.Style
		switch {
		case i == s.speedCursor && focus:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Bg).Background(t.Secondary).Padding(0, 1)
		case on:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Secondary).Padding(0, 1)
		default:
			st = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		}
		pills = append(pills, st.Render(icon+" "+id))
	}
	body := strings.Join(pills, " ")
	return comp.Card{
		Title:   i18n.T("tui.config.speedProviders"),
		Body:    body,
		Accent:  t.Secondary,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardRoute(s configState, width int, focus bool) string {
	t := theme.Active
	var pills []string
	for i, id := range s.routeIDs {
		on := s.routePresets[id]
		icon := "☐"
		if on {
			icon = "☑"
		}
		var st lipgloss.Style
		switch {
		case i == s.routeCursor && focus:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Bg).Background(t.Info).Padding(0, 1)
		case on:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Info).Padding(0, 1)
		default:
			st = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		}
		pills = append(pills, st.Render(icon+" "+strings.ToUpper(id)))
	}
	body := strings.Join(pills, " ")
	return comp.Card{
		Title:   i18n.T("tui.config.routePresets"),
		Body:    body,
		Accent:  t.Info,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardMediaSets(s configState, width int, focus bool) string {
	t := theme.Active
	var pills []string
	for i, id := range s.mediaIDs {
		on := s.mediaSets[id]
		icon := "☐"
		if on {
			icon = "☑"
		}
		var st lipgloss.Style
		switch {
		case i == s.mediaCursor && focus:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Bg).Background(t.Primary).Padding(0, 1)
		case on:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Padding(0, 1)
		default:
			st = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		}
		pills = append(pills, st.Render(icon+" "+id))
	}
	body := strings.Join(pills, " ")
	return comp.Card{
		Title:   i18n.T("tui.config.mediaSets"),
		Body:    body,
		Accent:  t.Primary,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardIPSources(s configState, width int, focus bool) string {
	t := theme.Active
	var pills []string
	for i, id := range s.ipSourceIDs {
		on := s.ipSources[id]
		icon := "☐"
		if on {
			icon = "☑"
		}
		var st lipgloss.Style
		switch {
		case i == s.ipSourceCursor && focus:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Bg).Background(t.Warning).Padding(0, 1)
		case on:
			st = lipgloss.NewStyle().Bold(true).Foreground(t.Warning).Padding(0, 1)
		default:
			st = lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1)
		}
		pills = append(pills, st.Render(icon+" "+id))
	}
	body := strings.Join(pills, " ")
	return comp.Card{
		Title:   i18n.T("tui.config.ipSources"),
		Body:    body,
		Accent:  t.Warning,
		Width:   width,
		Focused: focus,
	}.Render()
}

func configCardAdvanced(s configState, width int, focus bool) string {
	t := theme.Active
	values := []string{
		i18n.Tf("tui.config.iperf", map[string]any{"Value": firstStr(strings.TrimSpace(s.iperfHost), "-")}),
		i18n.Tf("tui.config.catalog", map[string]any{"Value": firstStr(strings.TrimSpace(s.catalogSource), nodecatalog.SourceEmbedded)}),
		i18n.Tf("tui.config.revision", map[string]any{"Value": firstStr(strings.TrimSpace(s.catalogRevision), i18n.T("tui.config.latestSelected"))}),
	}
	lines := make([]string, 0, len(values))
	for i, value := range values {
		value = truncStr(value, width-8)
		style := lipgloss.NewStyle().Foreground(t.Muted)
		if i == s.advancedCursor && focus {
			style = style.Bold(true).Foreground(t.Bg).Background(t.Info).Padding(0, 1)
		} else if i == s.advancedCursor {
			style = style.Bold(true).Foreground(t.Info)
		}
		lines = append(lines, style.Render(value))
	}
	return comp.Card{Title: i18n.T("tui.config.provenance"), Body: strings.Join(lines, "\n"), Accent: t.Info, Width: width, Focused: focus}.Render()
}

func configStartButton(s configState, width int, focus bool) string {
	t := theme.Active
	enabled := s.sections.AnyEnabled()
	label := i18n.T("tui.config.start")
	var btn lipgloss.Style
	switch {
	case !enabled:
		btn = lipgloss.NewStyle().Foreground(t.Muted).Background(t.Subtle).Padding(0, 4).Bold(true)
		label = i18n.T("tui.config.startDisabled")
	case focus:
		btn = lipgloss.NewStyle().Foreground(t.Bg).Background(t.Success).Padding(0, 4).Bold(true)
	default:
		btn = lipgloss.NewStyle().Foreground(t.Success).Padding(0, 4).Bold(true)
	}
	return btn.Render(label)
}
