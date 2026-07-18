package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func updateDashboard(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case "enter":
		item := menuItems[m.cursor]
		switch item.mode {
		case "single", "multi", "all":
			m.page = pageRunning
			m.engine = item.engine
			return m, func() tea.Msg {
				return benchmarkStartMsg{mode: item.mode, engine: item.engine}
			}
		case "suite":
			m.page = pageSuiteConfig
			return m, nil
		case "compare":
			m.page = pageCompare
			return m, nil
		case "sysinfo":
			m.showSysInfo = !m.showSysInfo
		case "quit":
			return m, tea.Quit
		}
	case "t":
		theme.CycleTheme()
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func viewDashboard(m Model) string {
	t := theme.Active
	bp := comp.BreakpointFor(m.width)

	var sections []string

	if bp >= comp.BreakpointCompact {
		sections = append(sections, comp.Banner(m.width))
		tagline := lipgloss.NewStyle().Foreground(t.Muted).Italic(true).Render(
			"  cross-platform VPS benchmark · measured raw metrics",
		)
		sections = append(sections, tagline)
		sections = append(sections, "")
	}

	switch bp {
	case comp.BreakpointTiny, comp.BreakpointCompact:
		sections = append(sections, dashboardMenu(m, m.width-4))
		sections = append(sections, "")
		sections = append(sections, dashboardSysCard(m, m.width-4))
	default:
		menuW := m.width / 2
		cardW := m.width - menuW - 2
		menuBlock := dashboardMenu(m, menuW)
		card := dashboardSysCard(m, cardW)
		sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Top, menuBlock, "  ", card))
	}

	if m.showSysInfo {
		sections = append(sections, "")
		sections = append(sections, dashboardSysExpanded(m, m.width-4))
	}

	return strings.Join(sections, "\n")
}

func dashboardMenu(m Model, width int) string {
	t := theme.Active
	if width < 20 {
		width = 20
	}
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Primary).
		Render("Menu")

	var lines []string
	lines = append(lines, header)
	lines = append(lines, "")

	for i, item := range menuItems {
		var line string
		if i == m.cursor {
			band := lipgloss.NewStyle().Foreground(t.Primary).Render("▎")
			label := lipgloss.NewStyle().
				Bold(true).
				Foreground(t.Primary).
				Render(item.label)
			desc := lipgloss.NewStyle().Foreground(t.Muted).Render(item.desc)
			line = band + " " + label
			if desc != "" && lipgloss.Width(line)+lipgloss.Width(desc)+2 < width {
				line += "  " + desc
			}
		} else {
			label := lipgloss.NewStyle().Foreground(t.Fg).Render(item.label)
			desc := lipgloss.NewStyle().Foreground(t.Subtle).Render(item.desc)
			line = "  " + label
			if desc != "" && lipgloss.Width(line)+lipgloss.Width(desc)+2 < width {
				line += "  " + desc
			}
		}
		lines = append(lines, line)
	}

	lines = append(lines, "")
	themeLine := lipgloss.NewStyle().Foreground(t.Muted).Render("  theme: ") +
		lipgloss.NewStyle().Foreground(t.Secondary).Bold(true).Render(theme.Active.Name) +
		lipgloss.NewStyle().Foreground(t.Muted).Render("  press [t] to cycle")
	lines = append(lines, themeLine)

	return strings.Join(lines, "\n")
}

func dashboardSysCard(m Model, width int) string {
	t := theme.Active
	if width < 24 {
		width = 24
	}

	cpu := m.sysInfo.CPU
	mem := m.sysInfo.Memory
	osInfo := m.sysInfo.OS

	cpuLine := truncStr(cpu.Model, width-12)
	if cpuLine == "" {
		cpuLine = "—"
	}

	rows := []comp.KV{
		{Key: "CPU", Value: cpuLine},
		{Key: "", Value: fmt.Sprintf("%d cores / %d threads", cpu.PhysicalCores, cpu.LogicalCores)},
		{Key: "Memory", Value: fmt.Sprintf("%.1f GB %s", float64(mem.TotalBytes)/(1024*1024*1024), strings.TrimSpace(mem.Type))},
		{Key: "OS", Value: truncStr(osInfo.Name, width-12)},
		{Key: "Kernel", Value: truncStr(osInfo.Kernel, width-12)},
	}

	body := comp.KVGrid(width-4, rows)

	footer := ""
	if cpu.BaseFreqMHz > 0 {
		footer = fmt.Sprintf("freq %.0f / %.0f MHz", cpu.BaseFreqMHz, cpu.MaxFreqMHz)
	}

	card := comp.Card{
		Title:    "System",
		Subtitle: fmt.Sprintf("%s/%s", osInfo.Hostname, cpu.Arch),
		Body:     body,
		Footer:   footer,
		Accent:   t.CategorySystem,
		Width:    width,
	}
	return card.Render()
}

func dashboardSysExpanded(m Model, width int) string {
	t := theme.Active
	cpu := m.sysInfo.CPU
	var lines []string
	if len(cpu.Features) > 0 {
		feats := strings.Join(cpu.Features, ", ")
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("Features  ")+lipgloss.NewStyle().Foreground(t.Fg).Render(truncStr(feats, width-12)))
	}
	if cpu.MicroArch != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("Arch      ")+lipgloss.NewStyle().Foreground(t.Fg).Render(cpu.MicroArch))
	}
	if len(cpu.CacheSizes) > 0 {
		var parts []string
		for k, v := range cpu.CacheSizes {
			parts = append(parts, fmt.Sprintf("%s %s", k, formatBytesSmall(uint64(v))))
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("Cache     ")+lipgloss.NewStyle().Foreground(t.Fg).Render(strings.Join(parts, "  ")))
	}
	if len(lines) == 0 {
		return ""
	}

	card := comp.Card{
		Title:  "Details",
		Body:   strings.Join(lines, "\n"),
		Accent: t.CategorySystem,
		Width:  width,
	}
	return card.Render()
}

func formatBytesSmall(value uint64) string {
	if value < 1024 {
		return fmt.Sprintf("%dB", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%dK", value/1024)
	}
	return fmt.Sprintf("%dM", value/(1024*1024))
}

func truncStr(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max-1]) + "…"
}
