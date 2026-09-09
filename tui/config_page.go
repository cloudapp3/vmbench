package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// The config page is a flat, ECS-style checklist: a start row on top, one row
// per suite section, and one collapsible advanced row. Every setting ships a
// default (all sections on, matching a full VPS checkup), so a fresh user can
// open the page and press enter. ↑↓ moves the cursor, space or enter toggles
// a row, enter on the start row runs.

// configRowKind identifies which row of the checklist the cursor is on.
type configRowKind int

const (
	rowStart configRowKind = iota
	rowSection
	rowAdvanced
	rowAdvSetting
)

// configRow is one selectable checklist row; index selects within the kind
// (section index, or advanced-setting index).
type configRow struct {
	kind  configRowKind
	index int
}

// Advanced settings, in display order. Each cycles through a fixed value
// list with ←→ or enter.
const (
	advIterations = iota
	advIPVersion
	advTimeout
	advCatalogSource
	advCount
)

var (
	advIPVersions     = []string{"v4", "v6", "dual"}
	advCatalogSources = []string{nodecatalog.SourceEmbedded, nodecatalog.SourceAuto}
)

type configState struct {
	cursor       int
	sections     suite.SectionSelector
	sectionIDs   []suite.SectionID
	advancedOpen bool

	iterations   int
	ipVersion    string
	timeoutIndex int
	timeouts     []time.Duration
	catalogIndex int
}

func newConfigState() configState {
	return configState{
		// All sections on by default: opening the page and pressing enter
		// runs the full checkup, and users untick what they do not want.
		sections: suite.DefaultSections(),
		sectionIDs: []suite.SectionID{
			suite.SectionHardware, suite.SectionNetworkInfo, suite.SectionRoute, suite.SectionPing,
			suite.SectionSpeed, suite.SectionIPQuality, suite.SectionReachability, suite.SectionMail, suite.SectionMedia,
		},
		iterations:   3,
		ipVersion:    "v4",
		timeoutIndex: 1,
		timeouts:     []time.Duration{time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute},
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

// visibleRows lists the checklist rows that currently render, in order.
// Cursor movement must never land on a hidden row, so every step goes
// through this list.
func (s configState) visibleRows() []configRow {
	rows := []configRow{{kind: rowStart}}
	for i := range s.sectionIDs {
		rows = append(rows, configRow{kind: rowSection, index: i})
	}
	rows = append(rows, configRow{kind: rowAdvanced})
	if s.advancedOpen {
		for j := 0; j < advCount; j++ {
			rows = append(rows, configRow{kind: rowAdvSetting, index: j})
		}
	}
	return rows
}

func (s configState) currentRow() configRow {
	rows := s.visibleRows()
	if s.cursor < 0 || s.cursor >= len(rows) {
		return rows[0]
	}
	return rows[s.cursor]
}

// cursorAt moves the cursor onto the first row matching kind (any index when
// index < 0); it reports whether such a row exists.
func (s *configState) cursorAt(kind configRowKind, index int) bool {
	for i, row := range s.visibleRows() {
		if row.kind == kind && (index < 0 || row.index == index) {
			s.cursor = i
			return true
		}
	}
	return false
}

// stepCursor moves by delta through the visible rows, wrapping.
func (s *configState) stepCursor(delta int) {
	rows := s.visibleRows()
	s.cursor = (s.cursor + delta + len(rows)) % len(rows)
}

// toggleAdvanced flips the settings block and pulls the cursor back onto the
// advanced row when collapsing swallowed it.
func (s *configState) toggleAdvanced() {
	s.advancedOpen = !s.advancedOpen
	if rows := s.visibleRows(); s.cursor >= len(rows) {
		s.cursor = len(rows) - 1
	}
}

// cycleSetting advances the focused advanced setting by delta, wrapping.
func (s *configState) cycleSetting(delta int) {
	switch s.currentRow().index {
	case advIterations:
		s.iterations += delta
		if s.iterations > 9 {
			s.iterations = 1
		}
		if s.iterations < 1 {
			s.iterations = 9
		}
	case advIPVersion:
		s.ipVersion = cycleChoice(advIPVersions, s.ipVersion, delta)
	case advTimeout:
		s.timeoutIndex = (s.timeoutIndex + delta + len(s.timeouts)) % len(s.timeouts)
	case advCatalogSource:
		s.catalogIndex = (s.catalogIndex + delta + len(advCatalogSources)) % len(advCatalogSources)
	}
}

func cycleChoice(values []string, current string, delta int) string {
	for i, v := range values {
		if v == current {
			return values[(i+delta+len(values))%len(values)]
		}
	}
	return values[0]
}

func (s configState) timeoutValue() time.Duration {
	if s.timeoutIndex >= 0 && s.timeoutIndex < len(s.timeouts) {
		return s.timeouts[s.timeoutIndex]
	}
	return 5 * time.Minute
}

func (s configState) catalogSource() string {
	if s.catalogIndex >= 0 && s.catalogIndex < len(advCatalogSources) {
		return advCatalogSources[s.catalogIndex]
	}
	return nodecatalog.SourceEmbedded
}

func (s configState) enabledCount() int {
	n := 0
	for i := range s.sectionIDs {
		if s.sectionGet(i) {
			n++
		}
	}
	return n
}

// buildRunOptions assembles the bare hardware-run options. Tool and filter
// picks are gone from the UI; NormalizeOptions applies the documented
// defaults for both.
func (s configState) buildRunOptions() vmbench.Options {
	return vmbench.Options{
		Engine:     "external",
		Iterations: s.iterations,
	}
}

// buildSuiteOptions assembles suite options. Providers, route presets, media
// sets, and IP sources are not surfaced any more; NormalizeOptions fills
// each with its default when the section is enabled.
func (s configState) buildSuiteOptions() suite.Options {
	return suite.Options{
		Iterations:    s.iterations,
		Sections:      s.sections,
		IPVersion:     s.ipVersion,
		Timeout:       s.timeoutValue(),
		CatalogSource: s.catalogSource(),
	}
}

type hardwareStartMsg struct{ opts vmbench.Options }

// configToast surfaces a launch failure without leaving the page.
func configToast(m Model, text string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.toast, cmd = comp.ShowToast(text, comp.ToastError, 4*time.Second)
	return m, cmd
}

func updateConfig(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.config
	switch msg.String() {
	case "esc":
		m.page = pageDashboard
		return m, nil
	case "q":
		return m, tea.Quit
	case "up", "k", "shift+tab":
		s.stepCursor(-1)
		return followFocus(m), nil
	case "down", "j", "tab":
		s.stepCursor(1)
		return followFocus(m), nil
	case "left", "h":
		switch s.currentRow().kind {
		case rowAdvanced:
			if s.advancedOpen {
				s.toggleAdvanced()
			}
		case rowAdvSetting:
			s.cycleSetting(-1)
		}
		return m, nil
	case "right", "l":
		switch s.currentRow().kind {
		case rowAdvanced:
			if !s.advancedOpen {
				s.toggleAdvanced()
			}
		case rowAdvSetting:
			s.cycleSetting(1)
		}
		return m, nil
	case " ", "x":
		if row := s.currentRow(); row.kind == rowSection {
			s.sectionToggle(row.index)
		}
		return m, nil
	case "enter":
		switch row := s.currentRow(); row.kind {
		case rowStart:
			return configStart(m)
		case rowSection:
			s.sectionToggle(row.index)
			return m, nil
		case rowAdvanced:
			s.toggleAdvanced()
			return followFocus(m), nil
		case rowAdvSetting:
			s.cycleSetting(1)
			return m, nil
		}
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Quick section toggle by digit.
		if idx := int(msg.String()[0] - '1'); idx < len(s.sectionIDs) {
			s.sectionToggle(idx)
		}
		return m, nil
	}
	return m, nil
}

// configStart launches the configured run. One report-kind rule, shared with
// the CLI and MCP: exactly the hardware section runs the bare benchmark (run
// report), anything else runs the suite.
func configStart(m Model) (tea.Model, tea.Cmd) {
	s := m.config
	if !s.sections.AnyEnabled() {
		return m, nil
	}
	if s.sections.HardwareOnly() {
		opts, err := vmbench.NormalizeOptions(s.buildRunOptions())
		if err != nil {
			return configToast(m, err.Error())
		}
		return m, func() tea.Msg { return hardwareStartMsg{opts: opts} }
	}
	norm, err := suite.NormalizeOptions(s.buildSuiteOptions())
	if err != nil {
		return configToast(m, err.Error())
	}
	return m, func() tea.Msg { return suiteStartMsg{opts: norm} }
}

func viewConfig(m Model) string {
	t := theme.Active
	s := m.config
	cur := s.currentRow()

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.config.title"))
	desc := lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.config.description"))

	lines := []string{title, desc, "", configStartRow(m, cur.kind == rowStart), configSeparator(m.width)}
	for i := range s.sectionIDs {
		lines = append(lines, configSectionRow(s, i, cur))
	}
	lines = append(lines, configSeparator(m.width), configAdvancedRow(s, cur))
	if s.advancedOpen {
		for j := 0; j < advCount; j++ {
			lines = append(lines, configAdvSettingRow(s, j, cur))
		}
	}
	if m.toast.Active() {
		lines = append(lines, "", m.toast.Render(m.width-4))
	}
	return strings.Join(lines, "\n")
}

// configFocusedLine reports the 0-based content line of the config cursor,
// mirroring viewConfig's layout; tests pin the two together.
func configFocusedLine(m Model) (int, bool) {
	s := m.config
	line := 3 // title, description, blank, then the start row
	switch row := s.currentRow(); row.kind {
	case rowStart:
		return line, true
	case rowSection:
		return line + 2 + row.index, true // +1 separator after the start row
	case rowAdvanced:
		return line + 3 + len(s.sectionIDs), true // +2 separators around the sections
	case rowAdvSetting:
		return line + 4 + len(s.sectionIDs) + row.index, true
	}
	return 0, false
}

// configBand is the focus indicator shared by every row.
func configBand(focus bool) string {
	if focus {
		return lipgloss.NewStyle().Foreground(theme.Active.Primary).Render("▎")
	}
	return " "
}

func configSeparator(width int) string {
	n := width - 6
	if n > 64 {
		n = 64
	}
	if n < 8 {
		n = 8
	}
	return lipgloss.NewStyle().Foreground(theme.Active.Subtle).Render(strings.Repeat("─", n))
}

func configStartRow(m Model, focus bool) string {
	t := theme.Active
	s := m.config
	enabled := s.sections.AnyEnabled()

	label := i18n.T("tui.config.start")
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Success)
	descStyle := lipgloss.NewStyle().Foreground(t.Muted)
	if enabled {
		desc := i18n.Tf("tui.config.startSummary", map[string]any{
			"Count":    s.enabledCount(),
			"Duration": formatDuration(estimateSuiteDuration(s, m.historyStats)),
		})
		return configBand(focus) + " " + labelStyle.Render(label) + "  " + descStyle.Render(desc)
	}
	return configBand(focus) + " " +
		lipgloss.NewStyle().Bold(true).Foreground(t.Muted).Render(i18n.T("tui.config.startDisabled")) + "  " +
		lipgloss.NewStyle().Foreground(t.Danger).Render(i18n.T("tui.config.needOneSection"))
}

func configSectionRow(s configState, i int, cur configRow) string {
	t := theme.Active
	on := s.sectionGet(i)
	focus := cur.kind == rowSection && cur.index == i

	iconStyle := lipgloss.NewStyle().Foreground(t.Subtle)
	labelStyle := lipgloss.NewStyle().Foreground(t.Fg)
	if on {
		iconStyle = iconStyle.Foreground(t.Success)
		labelStyle = labelStyle.Bold(true)
	}
	if focus {
		labelStyle = labelStyle.Foreground(t.Primary)
	}
	label := labelStyle.Render(i18n.SectionLabel(string(s.sectionIDs[i])))
	desc := lipgloss.NewStyle().Foreground(t.Subtle).Render(i18n.T("tui.config.sectionDesc." + string(s.sectionIDs[i])))
	return configBand(focus) + " " + iconStyle.Render(configCheckbox(on)) + " " + label + "  " + desc
}

func configCheckbox(on bool) string {
	if on {
		return "☑"
	}
	return "☐"
}

func configAdvancedRow(s configState, cur configRow) string {
	t := theme.Active
	focus := cur.kind == rowAdvanced
	arrow := "▸"
	if s.advancedOpen {
		arrow = "▾"
	}
	summary := strings.Join([]string{
		i18n.Tf("tui.config.iterations", map[string]any{"Count": s.iterations}),
		i18n.Tf("tui.config.ipVersion", map[string]any{"Version": s.ipVersion}),
		i18n.Tf("tui.config.timeout", map[string]any{"Timeout": s.timeoutValue().String()}),
	}, " · ")
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary)
	if focus {
		labelStyle = labelStyle.Foreground(t.Primary)
	}
	return configBand(focus) + " " + lipgloss.NewStyle().Foreground(t.Secondary).Render(arrow) + " " +
		labelStyle.Render(i18n.T("tui.config.advanced")) + "  " +
		lipgloss.NewStyle().Foreground(t.Subtle).Render(summary)
}

func configAdvSettingRow(s configState, j int, cur configRow) string {
	t := theme.Active
	focus := cur.kind == rowAdvSetting && cur.index == j

	var label, value string
	switch j {
	case advIterations:
		label, value = i18n.T("tui.config.advIterations"), fmt.Sprintf("%d", s.iterations)
	case advIPVersion:
		label, value = i18n.T("tui.config.advIPVersion"), s.ipVersion
	case advTimeout:
		label, value = i18n.T("tui.config.advTimeout"), s.timeoutValue().String()
	case advCatalogSource:
		label = i18n.T("tui.config.advCatalog")
		value = i18n.T("tui.config.catalogSource." + s.catalogSource())
	}

	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Fg)
	if focus {
		valueStyle = valueStyle.Foreground(t.Primary)
	}
	// Indent settings under the advanced row; the band keeps the focus column.
	return " " + configBand(focus) + "   " +
		lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.PadCells(label, 12)) + "  " +
		valueStyle.Render("‹ "+value+" ›")
}
