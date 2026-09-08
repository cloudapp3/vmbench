package tui

import (
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

type runConfigField int

const (
	fieldRunIterations runConfigField = iota
	fieldRunTools
	fieldRunFilter
	fieldRunStart
)

const runConfigFieldCount = int(fieldRunStart) + 1

// filterChips maps quick filter presets to the regex the runner applies to
// workload Name/Category ("" = no filter).
var runFilterChips = []struct {
	id       int
	labelKey string
	expr     string
}{
	{0, "tui.runConfig.filterAll", ""},
	{1, "tui.runConfig.filterCPU", "CPU"},
	{2, "tui.runConfig.filterDisk", "Disk"},
	{3, "tui.runConfig.filterMemory", "Memory"},
	{4, "tui.runConfig.filterCustom", ""},
}

type runConfigState struct {
	field      runConfigField
	iterations int // 1..9
	toolIDs    []string
	toolNames  map[string]string
	tools      map[string]bool
	toolCursor int
	filterChip int // index into runFilterChips
	filterText string
	missing    []string
	missingOK  bool // preflight result present
}

func newRunConfigState() runConfigState {
	s := runConfigState{
		field:      fieldRunIterations,
		iterations: 3,
		toolIDs:    catalog.HardwareToolIDs(),
		toolNames:  map[string]string{},
		tools:      map[string]bool{},
	}
	for _, spec := range catalog.HardwareTools() {
		s.toolNames[spec.ID] = spec.Name
	}
	for _, id := range s.toolIDs {
		s.tools[id] = false
	}
	for _, id := range catalog.DefaultHardwareTools() {
		s.tools[id] = true
	}
	return s
}

func (s runConfigState) selectedTools() []string {
	var out []string
	for _, id := range s.toolIDs {
		if s.tools[id] {
			out = append(out, id)
		}
	}
	return out
}

func (s runConfigState) filterExpr() string {
	if s.filterChip == 4 {
		return strings.TrimSpace(s.filterText)
	}
	return runFilterChips[s.filterChip].expr
}

func (s runConfigState) buildOptions() vmbench.Options {
	return vmbench.Options{
		Engine:        "external",
		Iterations:    s.iterations,
		Filter:        s.filterExpr(),
		HardwareTools: s.selectedTools(),
	}
}

// plannedWorkloads lists the workload rows the run will actually execute,
// mirroring the runner's filter so the running page never shows ghost rows.
func (s runConfigState) plannedWorkloads() []catalog.Definition {
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

func runMissingToolsCmd(s runConfigState) tea.Cmd {
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

func updateRunConfig(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.runConfig
	textEntry := s.field == fieldRunFilter && s.filterChip == 4
	switch msg.String() {
	case "up", "tab":
		s.field = runConfigField((int(s.field) + runConfigFieldCount - 1) % runConfigFieldCount)
	case "down", "shift+tab":
		s.field = runConfigField((int(s.field) + 1) % runConfigFieldCount)
	case "left", "h":
		switch s.field {
		case fieldRunIterations:
			if s.iterations > 1 {
				s.iterations--
			}
		case fieldRunTools:
			if s.toolCursor > 0 {
				s.toolCursor--
			}
		case fieldRunFilter:
			s.filterChip = (s.filterChip + len(runFilterChips) - 1) % len(runFilterChips)
			s.missingOK = false
		}
	case "right", "l":
		switch s.field {
		case fieldRunIterations:
			if s.iterations < 9 {
				s.iterations++
			}
		case fieldRunTools:
			if s.toolCursor < len(s.toolIDs)-1 {
				s.toolCursor++
			}
		case fieldRunFilter:
			s.filterChip = (s.filterChip + 1) % len(runFilterChips)
			s.missingOK = false
		}
	case " ", "x":
		if s.field == fieldRunTools {
			id := s.toolIDs[s.toolCursor]
			s.tools[id] = !s.tools[id]
			s.missingOK = false
		}
	case "backspace", "ctrl+h":
		if s.field == fieldRunFilter && s.filterChip == 4 && len(s.filterText) > 0 {
			s.filterText = s.filterText[:len(s.filterText)-1]
			s.missingOK = false
		}
	case "ctrl+u":
		if s.field == fieldRunFilter && s.filterChip == 4 {
			s.filterText = ""
			s.missingOK = false
		}
	case "enter":
		opts, err := vmbench.NormalizeOptions(s.buildOptions())
		if err != nil {
			var cmd tea.Cmd
			m.toast, cmd = comp.ShowToast(err.Error(), comp.ToastError, 4*time.Second)
			return m, cmd
		}
		return m, func() tea.Msg { return hardwareStartMsg{opts: opts} }
	case "esc":
		m.page = pageDashboard
		return m, nil
	case "q":
		if !textEntry {
			return m, tea.Quit
		}
	}
	if textEntry && (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) {
		s.filterText += string(msg.Runes)
		s.missingOK = false
	}
	m.runConfig = s
	return m, nil
}

func viewRunConfig(m Model) string {
	t := theme.Active
	s := m.runConfig
	width := m.width

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.runConfig.title"))
	desc := lipgloss.NewStyle().Foreground(t.Muted).Render(truncStr(i18n.T("tui.runConfig.description"), width-4))

	cardWidth := width - 4
	if cardWidth < 32 {
		cardWidth = 32
	}
	if width >= 100 {
		cardWidth = (width - 8) / 2
	}

	itersCard := runFieldIterations(s, cardWidth, s.field == fieldRunIterations)
	toolsCard := runFieldTools(s, cardWidth, s.field == fieldRunTools)
	filterCard := runFieldFilter(s, cardWidth, s.field == fieldRunFilter)
	preflightCard := runPreflightCard(s, cardWidth)
	startBtn := runStartButton(s, s.field == fieldRunStart)

	planned := i18n.Tf("tui.runConfig.plannedWorkloads", map[string]any{"Count": len(s.plannedWorkloads())})
	plannedLine := lipgloss.NewStyle().Foreground(t.Secondary).Render(planned)

	var fields string
	if width >= 100 {
		fields = strings.Join([]string{
			lipgloss.JoinHorizontal(lipgloss.Top, itersCard, "  ", toolsCard),
			lipgloss.JoinHorizontal(lipgloss.Top, filterCard, "  ", preflightCard),
		}, "\n")
	} else {
		fields = strings.Join([]string{itersCard, toolsCard, filterCard, preflightCard}, "\n")
	}
	parts := []string{title, desc, plannedLine, "", fields, "", startBtn}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width-4))
	}
	return strings.Join(parts, "\n")
}

func runStartButton(s runConfigState, focus bool) string {
	t := theme.Active
	enabled := len(s.selectedTools()) > 0 && len(s.plannedWorkloads()) > 0
	label := i18n.T("tui.runConfig.start")
	var btn lipgloss.Style
	switch {
	case !enabled:
		btn = lipgloss.NewStyle().Foreground(t.Muted).Background(t.Subtle).Padding(0, 4).Bold(true)
		label = i18n.T("tui.runConfig.startDisabled")
	case focus:
		btn = lipgloss.NewStyle().Foreground(t.Bg).Background(t.Success).Padding(0, 4).Bold(true)
	default:
		btn = lipgloss.NewStyle().Foreground(t.Success).Padding(0, 4).Bold(true)
	}
	return btn.Render(label)
}

func runFieldIterations(s runConfigState, width int, focused bool) string {
	t := theme.Active
	var cells []string
	for i := 1; i <= 9; i++ {
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(t.Subtle)
		if i == s.iterations {
			style = style.Foreground(t.Fg).Bold(true).Background(t.Surface)
		}
		cells = append(cells, style.Render(fmtDigit(i)))
	}
	body := strings.Join(cells, " ")
	return comp.Card{
		Title:   i18n.T("tui.runConfig.iterations"),
		Body:    body,
		Accent:  t.CategorySystem,
		Width:   width,
		Focused: focused,
	}.Render()
}

func fmtDigit(i int) string {
	return string(rune('0' + i))
}

func runFieldTools(s runConfigState, width int, focused bool) string {
	t := theme.Active
	var lines []string
	for i, id := range s.toolIDs {
		mark := "☐"
		style := lipgloss.NewStyle().Foreground(t.Fg)
		if s.tools[id] {
			mark = "☑"
			style = style.Foreground(t.Success).Bold(true)
		}
		cursor := "  "
		if i == s.toolCursor && focused {
			cursor = lipgloss.NewStyle().Foreground(t.Primary).Render("▎ ")
			style = style.Bold(true)
		}
		name := i18n.Tfallback("tool."+id, s.toolNames[id])
		lines = append(lines, cursor+mark+" "+style.Render(name))
	}
	return comp.Card{
		Title:   i18n.T("tui.runConfig.tools"),
		Body:    strings.Join(lines, "\n"),
		Accent:  t.CategorySystem,
		Width:   width,
		Focused: focused,
	}.Render()
}

func runFieldFilter(s runConfigState, width int, focused bool) string {
	t := theme.Active
	var cells []string
	for i, chip := range runFilterChips {
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(t.Subtle)
		if i == s.filterChip {
			style = style.Foreground(t.Fg).Bold(true).Background(t.Surface)
		}
		cells = append(cells, style.Render(i18n.T(chip.labelKey)))
	}
	body := strings.Join(cells, " ") + "\n"
	if s.filterChip == 4 {
		text := s.filterText
		if text == "" {
			text = i18n.T("tui.runConfig.filterCustomHint")
		}
		style := lipgloss.NewStyle().Foreground(t.Fg)
		if text == "" {
			style = lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
		}
		body += "\n" + style.Render(text) + "▏"
	}
	return comp.Card{
		Title:   i18n.T("tui.runConfig.filter"),
		Body:    body,
		Accent:  t.CategorySystem,
		Width:   width,
		Focused: focused,
	}.Render()
}

func runPreflightCard(s runConfigState, width int) string {
	t := theme.Active
	if !s.missingOK {
		return comp.Card{
			Title:  i18n.T("tui.runConfig.preflight"),
			Body:   lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(i18n.T("tui.runConfig.preflightPending")),
			Accent: t.CategorySystem,
			Width:  width,
		}.Render()
	}
	if len(s.missing) == 0 {
		return comp.Card{
			Title:  i18n.T("tui.runConfig.preflight"),
			Body:   lipgloss.NewStyle().Foreground(t.Success).Render("✓ " + i18n.T("tui.runConfig.missingNone")),
			Accent: t.CategorySystem,
			Width:  width,
		}.Render()
	}
	missing := append([]string(nil), s.missing...)
	sort.Strings(missing)
	body := lipgloss.NewStyle().Foreground(t.Warning).Render("! "+i18n.T("tui.runConfig.missingTools")) + "\n" +
		strings.Join(missing, ", ")
	return comp.Card{
		Title:  i18n.T("tui.runConfig.preflight"),
		Body:   body,
		Accent: t.Warning,
		Width:  width,
	}.Render()
}
