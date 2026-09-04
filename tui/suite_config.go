package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type suiteConfigField int

const (
	fieldPreset suiteConfigField = iota
	fieldSections
	fieldRuntime
	fieldHardwareTools
	fieldSpeedProviders
	fieldRoutePresets
	fieldMediaSets
	fieldIPSources
	fieldAdvanced
	fieldStart
)

const suiteConfigFieldCount = int(fieldStart) + 1

type suiteConfigState struct {
	field           suiteConfigField
	preset          int
	presetIDs       []string
	sections        suite.SectionSelector
	sectionCursor   int
	sectionKeys     []string
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
}

func newSuiteConfigState() suiteConfigState {
	presetIDs := append([]string{"custom"}, suite.PresetIDs()...)
	sections := suite.DefaultSections()
	if quick, ok := suite.LookupPreset("quick"); ok {
		sections = quick.Sections
	}
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
	return suiteConfigState{
		preset:         1,
		presetIDs:      presetIDs,
		sections:       sections,
		sectionKeys:    []string{"Hardware", "NetworkInfo", "Route", "Ping", "Speed", "IPQuality", "Reachability", "Mail", "Media"},
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

func (s *suiteConfigState) sectionGet(i int) bool {
	switch s.sectionKeys[i] {
	case "Hardware":
		return s.sections.Hardware
	case "NetworkInfo":
		return s.sections.NetworkInfo
	case "Route":
		return s.sections.Route
	case "Ping":
		return s.sections.Ping
	case "Speed":
		return s.sections.Speed
	case "IPQuality":
		return s.sections.IPQuality
	case "Reachability":
		return s.sections.Reachability
	case "Mail":
		return s.sections.Mail
	case "Media":
		return s.sections.Media
	}
	return false
}

func (s *suiteConfigState) sectionToggle(i int) {
	switch s.sectionKeys[i] {
	case "Hardware":
		s.sections.Hardware = !s.sections.Hardware
	case "NetworkInfo":
		s.sections.NetworkInfo = !s.sections.NetworkInfo
	case "Route":
		s.sections.Route = !s.sections.Route
	case "Ping":
		s.sections.Ping = !s.sections.Ping
	case "Speed":
		s.sections.Speed = !s.sections.Speed
	case "IPQuality":
		s.sections.IPQuality = !s.sections.IPQuality
	case "Reachability":
		s.sections.Reachability = !s.sections.Reachability
	case "Mail":
		s.sections.Mail = !s.sections.Mail
	case "Media":
		s.sections.Media = !s.sections.Media
	}
}

// toggleMediaSet flips one media set selection. Selecting "all" clears the
// region picks; picking any region clears "all" so the value stays meaningful.
func (s *suiteConfigState) toggleMediaSet(id string) {
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

func (s *suiteConfigState) applyPreset() {
	if s.preset == 0 {
		return
	}
	id := s.presetIDs[s.preset]
	if spec, ok := suite.LookupPreset(id); ok {
		s.sections = spec.Sections
		if strings.TrimSpace(spec.IPVersion) != "" {
			s.ipVersion = spec.IPVersion
		}
	}
}

func (s suiteConfigState) buildOptions(iperfHost string) suite.Options {
	preset := ""
	if s.preset > 0 {
		preset = s.presetIDs[s.preset]
	}
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
		Preset:          preset,
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
	selectedIperfHost := strings.TrimSpace(s.iperfHost)
	if strings.TrimSpace(iperfHost) != "" {
		selectedIperfHost = strings.TrimSpace(iperfHost)
	}
	if selectedIperfHost != "" {
		opts.IperfHosts = []string{selectedIperfHost}
	}
	return opts
}

type suiteStartMsg struct {
	opts suite.Options
}

func updateSuiteConfig(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.suiteConfig
	switch msg.String() {
	case "esc":
		m.page = pageDashboard
		return m, nil
	case "q":
		return m, tea.Quit
	case "tab", "down":
		s.field = suiteConfigField((int(s.field) + 1) % suiteConfigFieldCount)
		return m, nil
	case "shift+tab", "up":
		s.field = suiteConfigField((int(s.field) + suiteConfigFieldCount - 1) % suiteConfigFieldCount)
		return m, nil
	case "left", "h":
		switch s.field {
		case fieldPreset:
			if s.preset > 0 {
				s.preset--
				s.applyPreset()
			}
		case fieldSections:
			if s.sectionCursor > 0 {
				s.sectionCursor--
			}
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
			}
		case fieldSections:
			if s.sectionCursor < len(s.sectionKeys)-1 {
				s.sectionCursor++
			}
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
		switch s.field {
		case fieldSections:
			s.sectionToggle(s.sectionCursor)
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
		case fieldAdvanced:
			if s.advancedCursor == 1 {
				if s.catalogSource == nodecatalog.SourceAuto {
					s.catalogSource = nodecatalog.SourceEmbedded
				} else {
					s.catalogSource = nodecatalog.SourceAuto
				}
			} else if msg.String() == "x" {
				s.appendAdvanced("x")
			}
		}
		return m, nil
	case "backspace", "ctrl+h":
		if s.field == fieldAdvanced {
			s.backspaceAdvanced()
		}
		return m, nil
	case "ctrl+u":
		if s.field == fieldAdvanced {
			s.clearAdvanced()
		}
		return m, nil
	case "enter":
		if s.field == fieldStart || !s.sections.AnyEnabled() {
			if !s.sections.AnyEnabled() {
				return m, nil
			}
			norm, err := suite.NormalizeOptions(s.buildOptions(""))
			if err != nil {
				var cmd tea.Cmd
				m.toast, cmd = comp.ShowToast(err.Error(), comp.ToastError, 4*time.Second)
				return m, cmd
			}
			return m, func() tea.Msg { return suiteStartMsg{opts: norm} }
		}
		s.field = suiteConfigField((int(s.field) + 1) % suiteConfigFieldCount)
		return m, nil
	}
	if s.field == fieldAdvanced && msg.Type == tea.KeyRunes {
		s.appendAdvanced(string(msg.Runes))
	}
	return m, nil
}

func (s *suiteConfigState) cycleRuntimeValue() {
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

func (s *suiteConfigState) appendAdvanced(value string) {
	switch s.advancedCursor {
	case 0:
		s.iperfHost += value
	case 1:
		s.catalogSource += value
	case 2:
		s.catalogRevision += value
	}
}

func (s *suiteConfigState) backspaceAdvanced() {
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

func (s *suiteConfigState) clearAdvanced() {
	switch s.advancedCursor {
	case 0:
		s.iperfHost = ""
	case 1:
		s.catalogSource = ""
	case 2:
		s.catalogRevision = ""
	}
}

func viewSuiteConfig(m Model) string {
	t := theme.Active
	s := m.suiteConfig
	width := m.width

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("◈ Suite Configuration")
	if m.height < 40 {
		return viewSuiteConfigCompact(m, title)
	}
	desc := lipgloss.NewStyle().Foreground(t.Muted).Render(
		"VPS evidence suite — host, identity, route, latency, speed, reachability, IP, mail, media",
	)

	cardWidth := width - 4
	if cardWidth < 32 {
		cardWidth = 32
	}
	if width >= 100 {
		cardWidth = (width - 8) / 2
	}
	presetCard := suiteFieldPreset(s, cardWidth, s.field == fieldPreset)
	sectionsCard := suiteFieldSections(s, cardWidth, s.field == fieldSections)
	runtimeCard := suiteFieldRuntime(s, cardWidth, s.field == fieldRuntime)
	hardwareCard := suiteFieldHardware(s, cardWidth, s.field == fieldHardwareTools)
	speedCard := suiteFieldSpeed(s, cardWidth, s.field == fieldSpeedProviders)
	routeCard := suiteFieldRoute(s, cardWidth, s.field == fieldRoutePresets)
	mediaCard := suiteFieldMediaSets(s, cardWidth, s.field == fieldMediaSets)
	ipSourceCard := suiteFieldIPSources(s, cardWidth, s.field == fieldIPSources)
	advancedCard := suiteFieldAdvanced(s, cardWidth, s.field == fieldAdvanced)
	startBtn := suiteStartButton(s, width-4, s.field == fieldStart)

	help := lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(
		"  ↑↓ field   ←→ choose   space toggle   enter start   esc back",
	)

	var fields string
	if width >= 100 {
		rows := []string{
			lipgloss.JoinHorizontal(lipgloss.Top, presetCard, "  ", runtimeCard),
			lipgloss.JoinHorizontal(lipgloss.Top, sectionsCard, "  ", hardwareCard),
			lipgloss.JoinHorizontal(lipgloss.Top, speedCard, "  ", routeCard),
			lipgloss.JoinHorizontal(lipgloss.Top, mediaCard, "  ", ipSourceCard),
			advancedCard,
		}
		fields = strings.Join(rows, "\n")
	} else {
		fields = strings.Join([]string{presetCard, runtimeCard, sectionsCard, hardwareCard, speedCard, routeCard, mediaCard, ipSourceCard, advancedCard}, "\n")
	}
	parts := []string{title, desc, "", fields, "", startBtn, "", help}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

func viewSuiteConfigCompact(m Model, title string) string {
	t := theme.Active
	s := m.suiteConfig
	width := m.width
	preset := s.presetIDs[s.preset]
	if spec, ok := suite.LookupPreset(preset); ok {
		preset = spec.Name
	}
	summary := fmt.Sprintf("%s  |  %d sections  |  IP %s  |  %d iterations",
		preset, len(s.sections.Names()), s.ipVersion, s.iterations)
	summary = lipgloss.NewStyle().Foreground(t.Muted).Render(truncStr(summary, width-4))

	fieldWidth := width - 4
	if fieldWidth < 32 {
		fieldWidth = 32
	}
	var field string
	switch s.field {
	case fieldPreset:
		field = suiteFieldPreset(s, fieldWidth, true)
	case fieldSections:
		field = suiteFieldSections(s, fieldWidth, true)
	case fieldRuntime:
		field = suiteFieldRuntime(s, fieldWidth, true)
	case fieldHardwareTools:
		field = suiteFieldHardware(s, fieldWidth, true)
	case fieldSpeedProviders:
		field = suiteFieldSpeed(s, fieldWidth, true)
	case fieldRoutePresets:
		field = suiteFieldRoute(s, fieldWidth, true)
	case fieldMediaSets:
		field = suiteFieldMediaSets(s, fieldWidth, true)
	case fieldIPSources:
		field = suiteFieldIPSources(s, fieldWidth, true)
	case fieldAdvanced:
		field = suiteFieldAdvanced(s, fieldWidth, true)
	default:
		field = comp.Card{
			Title:   "Ready",
			Body:    fmt.Sprintf("%d sections selected  ·  press enter to run", len(s.sections.Names())),
			Accent:  t.Success,
			Width:   fieldWidth,
			Focused: true,
		}.Render()
	}

	startBtn := suiteStartButton(s, width-4, s.field == fieldStart)
	help := lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(
		"↑↓ field  ←→ choose  space toggle  enter next/start  esc back",
	)
	parts := []string{title, summary, "", field, "", startBtn, help}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

func suiteFieldPreset(s suiteConfigState, width int, focus bool) string {
	t := theme.Active

	var pills []string
	for i, id := range s.presetIDs {
		label := id
		if id == "custom" {
			label = "Custom"
		} else if spec, ok := suite.LookupPreset(id); ok {
			label = spec.Name
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
		Title:   "Preset",
		Body:    body,
		Accent:  accent,
		Width:   width,
		Focused: focus,
	}.Render()
}

func suiteFieldSections(s suiteConfigState, width int, focus bool) string {
	t := theme.Active
	var pills []string
	for i, name := range s.sectionKeys {
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
		pills = append(pills, st.Render(icon+" "+name))
	}
	body := strings.Join(pills, " ")
	if !s.sections.AnyEnabled() {
		body += "\n" + lipgloss.NewStyle().Foreground(t.Danger).Italic(true).Render("at least one section required")
	}
	return comp.Card{
		Title:   "Sections",
		Body:    body,
		Accent:  t.Accent,
		Width:   width,
		Focused: focus,
	}.Render()
}

func suiteFieldRuntime(s suiteConfigState, width int, focus bool) string {
	t := theme.Active
	timeout := 5 * time.Minute
	if s.timeoutIndex >= 0 && s.timeoutIndex < len(s.timeouts) {
		timeout = s.timeouts[s.timeoutIndex]
	}
	values := []string{
		fmt.Sprintf("Iterations %d", s.iterations),
		"IP " + s.ipVersion,
		"Timeout " + timeout.String(),
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
	return comp.Card{Title: "Runtime", Body: strings.Join(pills, " "), Accent: t.Primary, Width: width, Focused: focus}.Render()
}

func suiteFieldHardware(s suiteConfigState, width int, focus bool) string {
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
	return comp.Card{Title: "Hardware Tools", Body: strings.Join(pills, " "), Accent: t.Warning, Width: width, Focused: focus}.Render()
}

func suiteFieldSpeed(s suiteConfigState, width int, focus bool) string {
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
		Title:   "Speed Providers",
		Body:    body,
		Accent:  t.Secondary,
		Width:   width,
		Focused: focus,
	}.Render()
}

func suiteFieldRoute(s suiteConfigState, width int, focus bool) string {
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
		Title:   "China Route Presets",
		Body:    body,
		Accent:  t.Info,
		Width:   width,
		Focused: focus,
	}.Render()
}

func suiteFieldMediaSets(s suiteConfigState, width int, focus bool) string {
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
		Title:   "Media Sets",
		Body:    body,
		Accent:  t.Primary,
		Width:   width,
		Focused: focus,
	}.Render()
}

func suiteFieldIPSources(s suiteConfigState, width int, focus bool) string {
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
		Title:   "IP Quality Sources",
		Body:    body,
		Accent:  t.Warning,
		Width:   width,
		Focused: focus,
	}.Render()
}

func suiteFieldAdvanced(s suiteConfigState, width int, focus bool) string {
	t := theme.Active
	values := []string{
		"iperf: " + firstStr(strings.TrimSpace(s.iperfHost), "-"),
		"catalog: " + firstStr(strings.TrimSpace(s.catalogSource), nodecatalog.SourceEmbedded),
		"revision: " + firstStr(strings.TrimSpace(s.catalogRevision), "latest selected"),
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
	return comp.Card{Title: "Network Provenance", Body: strings.Join(lines, "\n"), Accent: t.Info, Width: width, Focused: focus}.Render()
}

func suiteStartButton(s suiteConfigState, width int, focus bool) string {
	t := theme.Active
	enabled := s.sections.AnyEnabled()
	label := "▶ Start Suite"
	var btn lipgloss.Style
	switch {
	case !enabled:
		btn = lipgloss.NewStyle().Foreground(t.Muted).Background(t.Subtle).Padding(0, 4).Bold(true)
		label = "▶ Start (disabled)"
	case focus:
		btn = lipgloss.NewStyle().Foreground(t.Bg).Background(t.Success).Padding(0, 4).Bold(true)
	default:
		btn = lipgloss.NewStyle().Foreground(t.Success).Padding(0, 4).Bold(true)
	}
	return btn.Render(label)
}
