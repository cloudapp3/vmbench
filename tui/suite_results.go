package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func updateSuiteResults(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.page = pageDashboard
			m.suiteReport = nil
			return m, nil
		case "q":
			return m, tea.Quit
		}
	case comp.ToastExpireMsg:
		if msg.Stamp == m.toast.Until {
			m.toast = comp.Toast{}
		}
		return m, nil
	}
	return m, nil
}

func viewSuiteResults(m Model) string {
	if m.suiteReport == nil {
		return lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  No suite results.")
	}
	t := theme.Active
	width := m.width
	r := *m.suiteReport

	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render("◈ Suite Report")

	statusColor := t.Success
	statusIcon := "✓"
	if !strings.EqualFold(r.Status, "ok") {
		statusColor = t.Danger
		statusIcon = "✗"
	}
	statusLine := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(
		fmt.Sprintf("  %s %s", statusIcon, strings.ToUpper(r.Status)),
	) +
		lipgloss.NewStyle().Foreground(t.Muted).Render("  "+r.Message)
	if m.height < 40 {
		return viewSuiteResultsCompact(m, r, headerTitle, statusIcon, statusColor)
	}

	cards := []string{}
	if r.Hardware.Enabled {
		cards = append(cards, hardwareResultCard(r, width))
	}
	if r.NetworkInfo.Enabled {
		cards = append(cards, networkInfoResultCard(r, width))
	}
	if r.Speed.Enabled {
		cards = append(cards, speedResultCard(r, width))
	}
	if r.Ping.Enabled {
		cards = append(cards, pingResultCard(r, width))
	}
	if r.Route.Enabled {
		cards = append(cards, routeResultCard(r, width))
	}
	if r.IPQuality.Enabled {
		cards = append(cards, ipQualityResultCard(r, width))
	}
	if r.Reachability.Enabled {
		cards = append(cards, reachabilityResultCard(r, width))
	}
	if r.Mail.Enabled {
		cards = append(cards, mailResultCard(r, width))
	}
	if r.Media.Enabled {
		cards = append(cards, mediaResultCard(r, width))
	}

	body := strings.Join(cards, "\n")
	parts := []string{headerTitle, statusLine, "", body}

	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(width))
	}
	return strings.Join(parts, "\n")
}

func viewSuiteResultsCompact(
	m Model,
	r suite.SuiteReport,
	title, statusIcon string,
	statusColor lipgloss.AdaptiveColor,
) string {
	t := theme.Active
	lineWidth := m.width - 4
	statusText := fmt.Sprintf("%s %s  %s", statusIcon, strings.ToUpper(firstStr(r.Status, "unknown")), r.Message)
	statusLine := lipgloss.NewStyle().Bold(true).Foreground(statusColor).Render(truncStr(statusText, lineWidth))
	parts := []string{title, statusLine, ""}
	for _, section := range r.Sections() {
		if !section.Enabled {
			continue
		}
		parts = append(parts, suiteResultCompactLine(section, lineWidth))
	}
	if len(parts) == 3 {
		parts = append(parts, lipgloss.NewStyle().Foreground(t.Muted).Render("No enabled sections."))
	}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(lineWidth))
	}
	return strings.Join(parts, "\n")
}

func suiteResultCompactLine(section suite.SectionSummary, width int) string {
	const labelWidth = 20
	label := lipgloss.NewStyle().Bold(true).Foreground(sectionAccent(section.ID)).Width(labelWidth).
		Render(truncStr(suiteSectionLabel(section.ID), labelWidth))
	statusText := truncStr(firstStr(section.Status, "unknown"), 10)
	status := comp.StatusPill(comp.StatusFromString(section.Status), statusText)
	remaining := width - labelWidth - lipgloss.Width(status) - 2
	if section.Message == "" || remaining <= 0 {
		return label + status
	}
	detail := lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  " + truncStr(section.Message, remaining))
	return label + status + detail
}

func suiteSectionLabel(id suite.SectionID) string {
	switch id {
	case suite.SectionHardware:
		return "Hardware"
	case suite.SectionNetworkInfo:
		return "Network Info"
	case suite.SectionRoute:
		return "Route"
	case suite.SectionPing:
		return "Ping"
	case suite.SectionSpeed:
		return "Speed"
	case suite.SectionIPQuality:
		return "IP Quality"
	case suite.SectionReachability:
		return "Reachability"
	case suite.SectionMail:
		return "Mail Ports"
	case suite.SectionMedia:
		return "Media Unlock"
	default:
		return string(id)
	}
}

func cardWidth(width int) int {
	w := width - 4
	if w < 30 {
		w = 30
	}
	return w
}

func hardwareResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if r.Hardware.Report == nil {
		return sectionStateCard(suite.SectionHardware, r.Hardware.SectionState, width)
	}
	doc := *r.Hardware.Report
	var lines []string
	for _, w := range doc.Results.Workloads {
		nameW := width - 30
		if nameW < 14 {
			nameW = 14
		}
		name := lipgloss.NewStyle().Foreground(t.Fg).Width(nameW).Render(truncStr(w.Name, nameW))
		var metric string
		if w.Result == nil {
			metric = "—"
		} else if w.Result.Error != "" {
			metric = lipgloss.NewStyle().Foreground(t.Danger).Render("error")
		} else {
			thr := tuiThroughput(w.Result)
			if thr != "-" {
				metric = lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(thr)
			} else {
				metric = lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render(tuiTime(w.Result))
			}
		}
		lines = append(lines, name+" "+metric)
	}
	return comp.Card{
		Title:    "Hardware",
		Subtitle: r.Hardware.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionHardware),
		Width:    cardWidth(width),
	}.Render()
}

func networkInfoResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if r.NetworkInfo.Result == nil {
		return sectionStateCard(suite.SectionNetworkInfo, r.NetworkInfo.SectionState, width)
	}
	result := r.NetworkInfo.Result
	lines := make([]string, 0, 6)
	appendPublic := func(label string, identity *suite.PublicIPIdentity) {
		if identity == nil {
			return
		}
		value := identity.IP
		if identity.ASN > 0 {
			value += fmt.Sprintf("  AS%d %s", identity.ASN, firstStr(identity.Org, identity.ISP))
		}
		lines = append(lines,
			lipgloss.NewStyle().Foreground(t.Muted).Width(8).Render(label)+
				lipgloss.NewStyle().Foreground(t.Fg).Render(truncStr(value, width-16)),
		)
	}
	appendPublic("IPv4", result.PublicIPv4)
	appendPublic("IPv6", result.PublicIPv6)
	for _, nat := range result.NAT {
		color := t.Warning
		if nat.Status == "direct" {
			color = t.Success
		}
		lines = append(lines,
			lipgloss.NewStyle().Foreground(t.Muted).Width(8).Render("NAT "+nat.IPVersion)+
				lipgloss.NewStyle().Foreground(color).Bold(true).Render(nat.Status)+
				lipgloss.NewStyle().Foreground(t.Muted).Render("  "+truncStr(nat.Reason, width-24)),
		)
	}
	if len(lines) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("no identity evidence"))
	}
	return comp.Card{Title: "Network Info", Subtitle: r.NetworkInfo.Status, Body: strings.Join(lines, "\n"), Accent: sectionAccent(suite.SectionNetworkInfo), Width: cardWidth(width)}.Render()
}

func speedResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if r.Speed.Result == nil {
		return sectionStateCard(suite.SectionSpeed, r.Speed.SectionState, width)
	}
	var lines []string
	for _, g := range r.Speed.Result.Groups {
		label := firstStr(g.ProviderLabel, g.Provider)
		dl := g.SummaryValue("download")
		ul := g.SummaryValue("upload")
		lat := g.SummaryValue("latency")
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(20).Render(label)
		val := fmt.Sprintf("%s↓ %s↑ %s rtt",
			fmtBandwidth(dl), fmtBandwidth(ul), fmtMs(lat))
		statusColor := t.Success
		if !strings.EqualFold(g.Status, "ok") {
			statusColor = t.Danger
		}
		valStyled := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(val)
		lines = append(lines, nameStyled+" "+valStyled)
	}
	if len(lines) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render("no providers"))
	}
	return comp.Card{
		Title:    "Speed",
		Subtitle: r.Speed.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionSpeed),
		Width:    cardWidth(width),
	}.Render()
}

func pingResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if len(r.Ping.Results) == 0 {
		return sectionStateCard(suite.SectionPing, r.Ping.SectionState, width)
	}
	var lines []string
	for _, p := range r.Ping.Results {
		name := firstStr(p.Name, "?")
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		connectionState := strings.ToLower(strings.TrimSpace(p.ConnectionState))
		var metric string
		if strings.EqualFold(p.Status, "ok") {
			color := t.Success
			if connectionState == netio.PingConnectionStateRefused || connectionState == netio.PingConnectionStateMixed {
				color = t.Warning
			}
			text := fmt.Sprintf("%s avg  %.0f%% loss", fmtMs(p.AvgLatencyMs), p.PacketLoss)
			if connectionState != "" {
				text += "  " + connectionState
			}
			metric = lipgloss.NewStyle().Foreground(color).Render(text)
		} else {
			text := firstStr(p.Status, "fail")
			if connectionState != "" {
				text += "  " + connectionState
			}
			metric = lipgloss.NewStyle().Foreground(t.Danger).Render(text)
		}
		lines = append(lines, nameStyled+" "+metric)
	}
	return comp.Card{
		Title:    "Ping",
		Subtitle: r.Ping.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionPing),
		Width:    cardWidth(width),
	}.Render()
}

func routeResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if len(r.Route.Results) == 0 {
		return sectionStateCard(suite.SectionRoute, r.Route.SectionState, width)
	}
	var lines []string
	for _, item := range r.Route.Results {
		name := fmt.Sprintf("%s %s", item.Target.City, item.Target.Carrier)
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		hops := lipgloss.NewStyle().Foreground(t.Accent).Render(fmt.Sprintf("%d hops", len(item.Hops)))
		status := tuiTraceStatus(item.EffectiveStatus())
		lines = append(lines, nameStyled+" "+hops+"  "+status)
	}
	return comp.Card{
		Title:    "Route",
		Subtitle: r.Route.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionRoute),
		Width:    cardWidth(width),
	}.Render()
}

func tuiTraceStatus(status string) string {
	t := theme.Active
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok":
		return lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render("✓ OK")
	case "partial":
		return lipgloss.NewStyle().Foreground(t.Warning).Bold(true).Render("! PARTIAL")
	default:
		return lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render("✗ ERROR")
	}
}

func ipQualityResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if r.IPQuality.Result == nil {
		return sectionStateCard(suite.SectionIPQuality, r.IPQuality.SectionState, width)
	}
	res := r.IPQuality.Result
	var lines []string
	valueWidth := cardWidth(width) - 16
	if valueWidth < 12 {
		valueWidth = 12
	}
	if message := strings.TrimSpace(r.IPQuality.Message); message != "" && !strings.EqualFold(r.IPQuality.Status, "ok") {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Danger).Render(truncStr(message, valueWidth+10)))
	}
	if res.BasicInfo != nil {
		info := res.BasicInfo
		lines = append(lines,
			lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("IP")+
				lipgloss.NewStyle().Foreground(t.Fg).Render(info.IP),
			lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("Country")+
				lipgloss.NewStyle().Foreground(t.Fg).Render(firstStr(info.CountryCode, info.Country)),
			lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("ASN")+
				lipgloss.NewStyle().Foreground(t.Fg).Render(fmt.Sprintf("%d %s", info.ASN, firstStr(info.Org, info.ISP))),
		)
	}
	if res.Score != nil {
		score := res.Score
		ratio := 0.0
		if score.MaxTotal > 0 {
			ratio = float64(score.Total) / float64(score.MaxTotal)
		}
		bar := comp.ProgressBar(20, ratio, t.Success)
		lines = append(lines, "")
		lines = append(lines,
			lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("Score")+
				bar+
				lipgloss.NewStyle().Bold(true).Foreground(t.Success).Render(
					fmt.Sprintf(" %d/%d", score.Total, score.MaxTotal),
				),
			lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("Level")+
				lipgloss.NewStyle().Foreground(t.Fg).Render(score.Level),
		)
	}
	if res.RiskSummary != nil {
		if summary := strings.TrimSpace(res.RiskSummary.Summary); summary != "" {
			lines = append(lines,
				lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("Evidence")+
					lipgloss.NewStyle().Foreground(t.Fg).Render(truncStr(summary, valueWidth)),
			)
		}
	}
	port25 := res.Port25
	if port25 == nil {
		for i := range res.MailPorts {
			if res.MailPorts[i].Port == 25 {
				port25 = &res.MailPorts[i]
				break
			}
		}
	}
	if port25 != nil {
		detail := firstStr(port25.Status, "unknown")
		if message := strings.TrimSpace(port25.Message); message != "" {
			detail += ": " + message
		}
		color := t.Warning
		if strings.EqualFold(port25.Status, "open") {
			color = t.Success
		} else if strings.EqualFold(port25.Status, "error") {
			color = t.Danger
		}
		lines = append(lines,
			lipgloss.NewStyle().Foreground(t.Muted).Width(10).Render("Port 25")+
				lipgloss.NewStyle().Foreground(color).Render(truncStr(detail, valueWidth)),
		)
	}
	return comp.Card{
		Title:    "IP Quality",
		Subtitle: r.IPQuality.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionIPQuality),
		Width:    cardWidth(width),
	}.Render()
}

func reachabilityResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if len(r.Reachability.Results) == 0 {
		return sectionStateCard(suite.SectionReachability, r.Reachability.SectionState, width)
	}
	lines := make([]string, 0, len(r.Reachability.Results))
	for _, item := range r.Reachability.Results {
		name := lipgloss.NewStyle().Foreground(t.Fg).Width(24).Render(truncStr(item.ID, 24))
		ok := strings.EqualFold(item.Status, "reachable")
		status := tuiCellStatus(ok)
		detail := item.Status
		if ok {
			detail = fmtMs(item.LatencyMs)
		}
		lines = append(lines, name+" "+status+"  "+lipgloss.NewStyle().Foreground(t.Muted).Render(detail))
	}
	return comp.Card{Title: "Reachability", Subtitle: r.Reachability.Status, Body: strings.Join(lines, "\n"), Accent: sectionAccent(suite.SectionReachability), Width: cardWidth(width)}.Render()
}

func mailResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if len(r.Mail.Results) == 0 {
		return sectionStateCard(suite.SectionMail, r.Mail.SectionState, width)
	}
	var lines []string
	for _, p := range r.Mail.Results {
		name := firstStr(p.Title, fmt.Sprintf("%d", p.Port))
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		status := tuiCellStatus(strings.EqualFold(p.Status, "open") || strings.EqualFold(p.Status, "ok"))
		lines = append(lines, nameStyled+" "+status+"  "+lipgloss.NewStyle().Foreground(t.Muted).Render(p.Status))
	}
	return comp.Card{
		Title:    "Mail Ports",
		Subtitle: r.Mail.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionMail),
		Width:    cardWidth(width),
	}.Render()
}

func mediaResultCard(r suite.SuiteReport, width int) string {
	t := theme.Active
	if r.Media.Result == nil || len(r.Media.Result.Items) == 0 {
		return sectionStateCard(suite.SectionMedia, r.Media.SectionState, width)
	}
	var lines []string
	for _, item := range r.Media.Result.Items {
		name := firstStr(item.Title, item.ID)
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		ok := strings.EqualFold(item.Status, "yes") || strings.EqualFold(item.Status, "ok") || strings.EqualFold(item.Status, "unlock")
		status := tuiCellStatus(ok)
		region := lipgloss.NewStyle().Foreground(t.Muted).Render(firstStr(item.Region, ""))
		lines = append(lines, nameStyled+" "+status+"  "+region)
	}
	return comp.Card{
		Title:    "Media Unlock",
		Subtitle: r.Media.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(suite.SectionMedia),
		Width:    cardWidth(width),
	}.Render()
}

func sectionStateCard(id suite.SectionID, st suite.SectionState, width int) string {
	t := theme.Active
	body := lipgloss.NewStyle().Foreground(t.Muted).Render(firstStr(st.Message, "no data"))
	if !strings.EqualFold(st.Status, "ok") && st.Status != "" {
		body = lipgloss.NewStyle().Foreground(t.Danger).Render(st.Status + ": " + st.Message)
	}
	return comp.Card{
		Title:    string(id),
		Subtitle: st.Status,
		Body:     body,
		Accent:   sectionAccent(id),
		Width:    cardWidth(width),
	}.Render()
}

func fmtBandwidth(v float64) string {
	if v <= 0 {
		return "—"
	}
	if v >= 1000 {
		return fmt.Sprintf("%.2f Gbps", v/1000)
	}
	return fmt.Sprintf("%.0f Mbps", v)
}

func fmtMs(v float64) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f ms", v)
}

func tuiCellStatus(ok bool) string {
	t := theme.Active
	if ok {
		return lipgloss.NewStyle().Foreground(t.Success).Bold(true).Render("✓")
	}
	return lipgloss.NewStyle().Foreground(t.Danger).Bold(true).Render("✗")
}
