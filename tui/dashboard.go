package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/sysinfo"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func updateDashboard(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := menuItems()
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(items)-1 {
			m.cursor++
		}
	case "enter":
		item := items[m.cursor]
		switch item.mode {
		case "bench":
			m.page = pageConfig
			if m.historyStats.samples == 0 && len(m.historyStats.avg) == 0 {
				return m, loadHistoryStatsCmd()
			}
			return m, nil
		case "compare":
			m.page = pageComparePicker
			if m.picker.records == nil && !m.picker.loading && m.picker.err == nil {
				m.picker.loading = true
				return m, loadHistoryCmd()
			}
			return m, nil
		case "history":
			m.page = pageHistory
			if m.history.records == nil && !m.history.loading && m.history.err == nil {
				m.history.loading = true
				return m, loadHistoryCmd()
			}
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
			i18n.T("tui.dashboard.tagline"),
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
		Render(i18n.T("tui.dashboard.menu"))

	var lines []string
	lines = append(lines, header)
	lines = append(lines, "")

	for i, item := range menuItems() {
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
	themeLine := lipgloss.NewStyle().Foreground(t.Muted).Render("  "+i18n.T("tui.dashboard.theme")+": ") +
		lipgloss.NewStyle().Foreground(t.Secondary).Bold(true).Render(theme.Active.Name) +
		lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.Tf("tui.dashboard.themeCycle", nil))
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
		{Key: "", Value: i18n.Tf("tui.sys.coresThreads", map[string]any{"Cores": cpu.PhysicalCores, "Threads": cpu.LogicalCores})},
		{Key: i18n.T("tui.sys.memory"), Value: fmt.Sprintf("%.1f GB %s", float64(mem.TotalBytes)/(1024*1024*1024), strings.TrimSpace(mem.Type))},
		{Key: i18n.T("tui.sys.os"), Value: truncStr(osInfo.Name, width-12)},
		{Key: i18n.T("tui.sys.kernel"), Value: truncStr(osInfo.Kernel, width-12)},
	}
	if nic := m.sysInfo.Network.PrimaryNIC(); nic != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.nic"), Value: truncStr(nic, width-12)})
	}
	if m.netIdent.loading {
		rows = append(rows, comp.KV{Key: "IPv4", Value: i18n.T("tui.sys.netLoading")})
	} else {
		if m.netIdent.v4 != nil {
			rows = append(rows, comp.KV{Key: "IPv4", Value: truncStr(publicIdentityValue(m.netIdent.v4), width-12)})
		}
		if m.netIdent.v6 != nil {
			rows = append(rows, comp.KV{Key: "IPv6", Value: truncStr(publicIdentityValue(m.netIdent.v6), width-12)})
		}
	}
	if oversell := m.sysInfo.Platform.OversellSignalsText(); oversell != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.oversell"), Value: truncStr(oversell, width-12)})
	}

	body := comp.KVGrid(width-4, rows)

	footer := ""
	if cpu.BaseFreqMHz > 0 {
		footer = i18n.Tf("tui.sys.freq", map[string]any{"Base": fmt.Sprintf("%.0f", cpu.BaseFreqMHz), "Max": fmt.Sprintf("%.0f", cpu.MaxFreqMHz)})
	}

	card := comp.Card{
		Title:    i18n.T("tui.sys.cardTitle"),
		Subtitle: fmt.Sprintf("%s/%s", osInfo.Hostname, cpu.Arch),
		Body:     body,
		Footer:   footer,
		Accent:   t.CategorySystem,
		Width:    width,
	}
	return card.Render()
}

// dashboardSysExpanded renders the full evidence below the sys card: CPU,
// network identity, virtualization, memory, storage, and runtime. Cards
// without evidence disappear; wide terminals pair them into two columns.
func dashboardSysExpanded(m Model, width int) string {
	twoCol := width >= 100
	colW := width
	if twoCol {
		colW = (width - 2) / 2
	}
	cards := sysDetailCards(m, colW)
	if len(cards) == 0 {
		return ""
	}
	if !twoCol || len(cards) == 1 {
		return strings.Join(cards, "\n\n")
	}
	half := (len(cards) + 1) / 2
	left := strings.Join(cards[:half], "\n\n")
	right := strings.Join(cards[half:], "\n\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

// sysDetailCards builds the expanded evidence cards in display order.
func sysDetailCards(m Model, cardW int) []string {
	var cards []string
	for _, build := range []func(Model, int) string{
		sysCPUDetailCard,
		sysNetDetailCard,
		sysVirtDetailCard,
		sysMemoryDetailCard,
		sysDiskDetailCard,
		sysRuntimeDetailCard,
	} {
		if card := build(m, cardW); card != "" {
			cards = append(cards, card)
		}
	}
	return cards
}

func sysDetailCard(title string, rows []comp.KV, cardW int) string {
	if len(rows) == 0 {
		return ""
	}
	card := comp.Card{
		Title:  title,
		Body:   comp.KVGrid(cardW-4, rows),
		Accent: theme.Active.CategorySystem,
		Width:  cardW,
	}
	return card.Render()
}

func sysCPUDetailCard(m Model, cardW int) string {
	cpu := m.sysInfo.CPU
	rows := []comp.KV{}
	if len(cpu.Features) > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.features"), Value: strings.Join(cpu.Features, ", ")})
	}
	if cpu.MicroArch != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.arch"), Value: cpu.MicroArch})
	}
	if cache := sysinfo.FormatCacheLine(cpu.CacheSizes); cache != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.cache"), Value: cache})
	}
	if cpu.Stepping > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.stepping"), Value: fmt.Sprintf("%d", cpu.Stepping)})
	}
	if cpu.NumaNodes > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.numa"), Value: i18n.Tf("tui.sys.numaNodes", map[string]any{"Count": cpu.NumaNodes})})
	}
	return sysDetailCard(i18n.T("tui.sys.cpuTitle"), rows, cardW)
}

func sysNetDetailCard(m Model, cardW int) string {
	rows := []comp.KV{}
	if ident := primaryPublicIdentity(m); ident != nil {
		if loc := identityLocation(ident); loc != "" {
			rows = append(rows, comp.KV{Key: i18n.T("tui.sys.country"), Value: loc})
		}
		if ident.ISP != "" {
			rows = append(rows, comp.KV{Key: i18n.T("tui.sys.isp"), Value: ident.ISP})
		}
	}
	return sysDetailCard(i18n.T("tui.sys.netTitle"), rows, cardW)
}

func sysVirtDetailCard(m Model, cardW int) string {
	sys := m.sysInfo
	rows := []comp.KV{}
	if sys.Virtualization.System != "" || sys.Virtualization.Role != "" {
		platform := firstStr(sys.Virtualization.System, "?")
		if sys.Virtualization.Role != "" {
			platform += " (" + sys.Virtualization.Role + ")"
		}
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.virt"), Value: platform})
	}
	if product := firstStr(sys.DMI.ProductName, sys.DMI.BoardName); product != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.dmiProduct"), Value: product})
	}
	if vendor := firstStr(sys.DMI.SysVendor, sys.DMI.BoardVendor); vendor != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.dmiVendor"), Value: vendor})
	}
	if sys.Platform.NestedVirtualization != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.nestedVirt"), Value: sys.Platform.NestedVirtualization})
	}
	for _, signal := range sys.Platform.OversellSignals() {
		value := signal.State
		if signal.Risk {
			value += " (!)"
		}
		if signal.Key == "ksm" && sys.Platform.KSMPagesShared > 0 {
			value += fmt.Sprintf(" · %d pages", sys.Platform.KSMPagesShared)
		}
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys." + signal.Key), Value: value})
	}
	return sysDetailCard(i18n.T("tui.sys.virtTitle"), rows, cardW)
}

func sysMemoryDetailCard(m Model, cardW int) string {
	mem := m.sysInfo.Memory
	rows := []comp.KV{}
	if spec := memorySpecText(mem); spec != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.memSpec"), Value: spec})
	}
	// Used/available is a runtime snapshot; zero means unknown (MemoryInfo
	// contract), so the water line only appears with real evidence.
	if mem.UsedBytes > 0 || mem.AvailableBytes > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.memWater"), Value: memoryWaterText(mem)})
	}
	return sysDetailCard(i18n.T("tui.sys.memTitle"), rows, cardW)
}

// memorySpecText composes "DDR4 2666 MT/s ×2" from whatever is known.
func memorySpecText(mem sysinfo.MemoryInfo) string {
	var parts []string
	if strings.TrimSpace(mem.Type) != "" {
		parts = append(parts, mem.Type)
	}
	if mem.FreqMHz > 0 {
		parts = append(parts, fmt.Sprintf("%d MT/s", mem.FreqMHz))
	}
	if mem.Channels > 0 {
		parts = append(parts, fmt.Sprintf("×%d", mem.Channels))
	}
	return strings.Join(parts, " ")
}

func memoryWaterText(mem sysinfo.MemoryInfo) string {
	var text string
	if mem.UsedBytes > 0 {
		text = formatGiB(mem.UsedBytes)
	}
	if mem.AvailableBytes > 0 {
		if text != "" {
			text += " / "
		}
		text += i18n.Tf("tui.sys.memAvailable", map[string]any{"Value": formatGiB(mem.AvailableBytes)})
	}
	if mem.UsedPercent > 0 {
		text += fmt.Sprintf(" (%.0f%%)", mem.UsedPercent)
	}
	return text
}

func sysDiskDetailCard(m Model, cardW int) string {
	rows := []comp.KV{}
	if boot := m.sysInfo.Platform.BootDisk; boot != "" {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.bootDisk"), Value: boot})
	}
	for _, disk := range m.sysInfo.Disks {
		value := fmt.Sprintf("%s · %s", disk.FSType, disk.Mountpoint)
		if disk.TotalBytes > 0 {
			value = fmt.Sprintf("%s · %s · %s", disk.FSType, formatGiB(disk.TotalBytes), disk.Mountpoint)
		}
		rows = append(rows, comp.KV{Key: disk.Device, Value: value})
	}
	return sysDetailCard(i18n.T("tui.sys.diskTitle"), rows, cardW)
}

func sysRuntimeDetailCard(m Model, cardW int) string {
	p := m.sysInfo.Platform
	rows := []comp.KV{}
	if p.UptimeSeconds > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.uptime"), Value: formatDuration(time.Duration(p.UptimeSeconds) * time.Second)})
	}
	if p.Load1 > 0 || p.Load5 > 0 || p.Load15 > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.load"), Value: fmt.Sprintf("%.2f %.2f %.2f", p.Load1, p.Load5, p.Load15)})
	}
	if p.SwapTotalBytes > 0 {
		rows = append(rows, comp.KV{Key: i18n.T("tui.sys.swap"), Value: fmt.Sprintf("%s / %s", formatGiB(p.SwapUsedBytes), formatGiB(p.SwapTotalBytes))})
	}
	return sysDetailCard(i18n.T("tui.sys.runtimeTitle"), rows, cardW)
}

// formatGiB renders a byte count with a binary unit, KiB..TiB.
func formatGiB(bytes uint64) string {
	switch {
	case bytes >= 1<<40:
		return fmt.Sprintf("%.1f TiB", float64(bytes)/(1<<40))
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.0f MiB", float64(bytes)/(1<<20))
	default:
		return fmt.Sprintf("%.0f KiB", float64(bytes)/(1<<10))
	}
}

func truncStr(s string, max int) string {
	return i18n.TruncateCells(s, max)
}

// publicIdentityValue renders one public address the same way as the checkup
// results card: the address plus, when metadata landed, "  AS#### Org" (Org
// falling back to ISP).
func publicIdentityValue(identity *netio.PublicIPIdentity) string {
	value := identity.IP
	if identity.ASN > 0 {
		value += fmt.Sprintf("  AS%d %s", identity.ASN, firstStr(identity.Org, identity.ISP))
	}
	return value
}

// primaryPublicIdentity prefers the IPv4 identity and falls back to IPv6,
// mirroring the v4-first convention of the netio probes.
func primaryPublicIdentity(m Model) *netio.PublicIPIdentity {
	if m.netIdent.v4 != nil {
		return m.netIdent.v4
	}
	return m.netIdent.v6
}

// identityLocation renders "US United States"; either half may be absent.
func identityLocation(identity *netio.PublicIPIdentity) string {
	return strings.TrimSpace(strings.TrimSpace(identity.CountryCode) + " " + strings.TrimSpace(identity.Country))
}
