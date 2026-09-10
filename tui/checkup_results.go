package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui/comp"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func updateCheckupResults(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.reportFrom != pageDashboard {
				from := m.reportFrom
				m.reportFrom = pageDashboard
				m.page = from
				return m, nil
			}
			m.page = pageDashboard
			m.checkupReport = nil
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

func viewCheckupResults(m Model) string {
	if m.checkupReport == nil {
		return lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  " + i18n.T("tui.checkupResults.noResults"))
	}
	t := theme.Active
	width := m.width
	r := *m.checkupReport

	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(t.Primary).Render(i18n.T("tui.checkupResults.title"))

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
		return viewCheckupResultsCompact(m, r, headerTitle, statusIcon, statusColor)
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

func viewCheckupResultsCompact(
	m Model,
	r checkup.CheckupReport,
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
		parts = append(parts, checkupResultCompactLine(section, lineWidth))
	}
	if len(parts) == 3 {
		parts = append(parts, lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.checkupResults.noSections")))
	}
	if m.toast.Active() {
		parts = append(parts, "", m.toast.Render(lineWidth))
	}
	return strings.Join(parts, "\n")
}

func checkupResultCompactLine(section checkup.SectionSummary, width int) string {
	label := checkupSectionLabel(section.ID)
	labelWidth := comp.ColWidth(label, 20)
	styledLabel := lipgloss.NewStyle().Bold(true).Foreground(sectionAccent(section.ID)).Width(labelWidth).
		Render(truncStr(label, labelWidth))
	statusText := truncStr(firstStr(section.Status, "unknown"), 10)
	status := comp.StatusPill(comp.StatusFromString(section.Status), statusText)
	remaining := width - labelWidth - lipgloss.Width(status) - 2
	if section.Message == "" || remaining <= 0 {
		return styledLabel + status
	}
	detail := lipgloss.NewStyle().Foreground(theme.Active.Muted).Render("  " + truncStr(section.Message, remaining))
	return styledLabel + status + detail
}

// checkupSectionLabel is the single render-time section label source; the
// machine ID itself never changes.
func checkupSectionLabel(id checkup.SectionID) string {
	return i18n.SectionLabel(string(id))
}

func cardWidth(width int) int {
	w := width - 4
	if w < 30 {
		w = 30
	}
	return w
}

func hardwareResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if r.Hardware.Report == nil {
		return sectionStateCard(checkup.SectionHardware, r.Hardware.SectionState, width)
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
		Title:    checkupSectionLabel(checkup.SectionHardware),
		Subtitle: r.Hardware.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionHardware),
		Width:    cardWidth(width),
	}.Render()
}

func networkInfoResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if r.NetworkInfo.Result == nil {
		return sectionStateCard(checkup.SectionNetworkInfo, r.NetworkInfo.SectionState, width)
	}
	result := r.NetworkInfo.Result
	lines := make([]string, 0, 6)
	appendPublic := func(label string, identity *checkup.PublicIPIdentity) {
		if identity == nil {
			return
		}
		lines = append(lines,
			lipgloss.NewStyle().Foreground(t.Muted).Width(8).Render(label)+
				lipgloss.NewStyle().Foreground(t.Fg).Render(truncStr(publicIdentityValue(identity), width-16)),
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
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.checkupResults.noIdentityEvidence")))
	}
	return comp.Card{Title: checkupSectionLabel(checkup.SectionNetworkInfo), Subtitle: r.NetworkInfo.Status, Body: strings.Join(lines, "\n"), Accent: sectionAccent(checkup.SectionNetworkInfo), Width: cardWidth(width)}.Render()
}

func speedResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if r.Speed.Result == nil {
		return sectionStateCard(checkup.SectionSpeed, r.Speed.SectionState, width)
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
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Render(i18n.T("tui.checkupResults.noProviders")))
	}
	return comp.Card{
		Title:    checkupSectionLabel(checkup.SectionSpeed),
		Subtitle: r.Speed.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionSpeed),
		Width:    cardWidth(width),
	}.Render()
}

func pingResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if len(r.Ping.Results) == 0 {
		return sectionStateCard(checkup.SectionPing, r.Ping.SectionState, width)
	}
	var lines []string
	for _, p := range r.Ping.Results {
		name := firstStr(p.Name, "?")
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		connectionState := strings.ToLower(strings.TrimSpace(p.ConnectionState))
		icmpFallback := strings.Contains(strings.ToLower(p.ProbeProtocol), "icmp")
		var metric string
		if strings.EqualFold(p.Status, "ok") {
			color := t.Success
			if connectionState == netio.PingConnectionStateRefused || connectionState == netio.PingConnectionStateMixed || icmpFallback {
				color = t.Warning
			}
			text := fmt.Sprintf("%s avg  %.0f%% loss", fmtMs(p.AvgLatencyMs), p.PacketLoss)
			if connectionState != "" {
				text += "  " + connectionState
			}
			if icmpFallback {
				text += "  icmp"
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
		Title:    checkupSectionLabel(checkup.SectionPing),
		Subtitle: r.Ping.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionPing),
		Width:    cardWidth(width),
	}.Render()
}

func routeResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if len(r.Route.Results) == 0 {
		return sectionStateCard(checkup.SectionRoute, r.Route.SectionState, width)
	}
	cardW := cardWidth(width)
	nameW := cardW / 4
	if nameW > 18 {
		nameW = 18
	}
	if nameW < 10 {
		nameW = 10
	}
	var lines []string
	for _, item := range r.Route.Results {
		name := firstStr(item.Target.Name, strings.TrimSpace(item.Target.City+" "+item.Target.Carrier))
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(nameW).Render(truncStr(name, nameW))
		ip := firstStr(item.ResolvedTarget, "-")
		ipW := 15
		if strings.Contains(ip, ":") {
			ipW = 22
		}
		ipStyled := lipgloss.NewStyle().Foreground(t.Muted).Width(ipW).Render(truncStr(ip, ipW))
		lineW := cardW - nameW - ipW - 14
		if lineW < 10 {
			lineW = 10
		}
		color := t.Fg
		switch checkup.RouteLineTone(item) {
		case "ok":
			color = t.Success
		case "warn":
			color = t.Warning
		}
		lineStyled := lipgloss.NewStyle().Foreground(color).Width(lineW).Render(truncStr(checkup.RouteLineText(item), lineW))
		status := tuiTraceStatus(item.EffectiveStatus())
		lines = append(lines, nameStyled+" "+ipStyled+"  "+lineStyled+"  "+status)
	}
	return comp.Card{
		Title:    checkupSectionLabel(checkup.SectionRoute),
		Subtitle: r.Route.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionRoute),
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

func ipQualityResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if r.IPQuality.Result == nil {
		return sectionStateCard(checkup.SectionIPQuality, r.IPQuality.SectionState, width)
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
		Title:    checkupSectionLabel(checkup.SectionIPQuality),
		Subtitle: r.IPQuality.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionIPQuality),
		Width:    cardWidth(width),
	}.Render()
}

func reachabilityResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if len(r.Reachability.Results) == 0 {
		return sectionStateCard(checkup.SectionReachability, r.Reachability.SectionState, width)
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
	return comp.Card{Title: checkupSectionLabel(checkup.SectionReachability), Subtitle: r.Reachability.Status, Body: strings.Join(lines, "\n"), Accent: sectionAccent(checkup.SectionReachability), Width: cardWidth(width)}.Render()
}

func mailResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if len(r.Mail.Results) == 0 {
		return sectionStateCard(checkup.SectionMail, r.Mail.SectionState, width)
	}
	var lines []string
	for _, p := range r.Mail.Results {
		name := firstStr(p.Title, fmt.Sprintf("%d", p.Port))
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		status := tuiCellStatus(strings.EqualFold(p.Status, "open") || strings.EqualFold(p.Status, "ok"))
		lines = append(lines, nameStyled+" "+status+"  "+lipgloss.NewStyle().Foreground(t.Muted).Render(p.Status))
	}
	return comp.Card{
		Title:    checkupSectionLabel(checkup.SectionMail),
		Subtitle: r.Mail.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionMail),
		Width:    cardWidth(width),
	}.Render()
}

// mediaCardLimit caps rendered media rows so a full-platform run stays
// readable inside the TUI card.
const mediaCardLimit = 24

func mediaResultCard(r checkup.CheckupReport, width int) string {
	t := theme.Active
	if r.Media.Result == nil || len(r.Media.Result.Items) == 0 {
		return sectionStateCard(checkup.SectionMedia, r.Media.SectionState, width)
	}
	var lines []string
	for _, item := range r.Media.Result.Items {
		if len(lines) >= mediaCardLimit {
			lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).
				Render(i18n.Tf("tui.checkupResults.moreItems", map[string]any{"Count": len(r.Media.Result.Items) - mediaCardLimit})))
			break
		}
		name := firstStr(item.Title, item.ID)
		nameStyled := lipgloss.NewStyle().Foreground(t.Fg).Width(28).Render(truncStr(name, 28))
		ok := strings.EqualFold(item.Status, "available")
		status := tuiCellStatus(ok)
		if item.RawStatus == "Restricted" {
			status = lipgloss.NewStyle().Foreground(t.Warning).Render("~")
		}
		region := lipgloss.NewStyle().Foreground(t.Muted).Render(firstStr(item.Region, ""))
		lines = append(lines, nameStyled+" "+status+"  "+region)
	}
	return comp.Card{
		Title:    checkupSectionLabel(checkup.SectionMedia),
		Subtitle: r.Media.Status,
		Body:     strings.Join(lines, "\n"),
		Accent:   sectionAccent(checkup.SectionMedia),
		Width:    cardWidth(width),
	}.Render()
}

func sectionStateCard(id checkup.SectionID, st checkup.SectionState, width int) string {
	t := theme.Active
	body := lipgloss.NewStyle().Foreground(t.Muted).Render(firstStr(st.Message, i18n.T("tui.checkupResults.noData")))
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
