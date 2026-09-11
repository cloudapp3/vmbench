package checkup

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/textgrid"
)

func WriteConsole(w io.Writer, report CheckupReport) error {
	if w == nil {
		w = os.Stdout
	}
	if _, err := fmt.Fprintln(w, i18n.T("report.checkup.title")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s: %s\n", i18n.T("report.checkup.status"), defaultText(report.Status, "unknown")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s: %s\n", i18n.T("report.checkup.message"), defaultText(report.Message, "-")); err != nil {
		return err
	}
	if preset := strings.TrimSpace(report.Config.Preset); preset != "" {
		if _, err := fmt.Fprintf(w, "%s: %s\n", i18n.T("report.checkup.preset"), preset); err != nil {
			return err
		}
	}
	if sections := report.Config.Sections.String(); sections != "" {
		if _, err := fmt.Fprintf(w, "%s: %s\n", i18n.T("report.checkup.sections"), sections); err != nil {
			return err
		}
	}

	if report.Hardware.Enabled {
		if _, err := writeSectionBanner(w, SectionHardware); err != nil {
			return err
		}
		if err := writeHardwareBody(w, report.Hardware); err != nil {
			return err
		}
	}

	if report.NetworkInfo.Enabled {
		if err := writeNetworkInfoConsole(w, report.NetworkInfo); err != nil {
			return err
		}
	}

	if report.Route.Enabled {
		if _, err := writeSectionBanner(w, SectionRoute); err != nil {
			return err
		}
		if err := writeRouteBody(w, report.Route); err != nil {
			return err
		}
	}

	if report.Ping.Enabled {
		if _, err := writeSectionBanner(w, SectionPing); err != nil {
			return err
		}
		if err := writePingBody(w, report.Ping); err != nil {
			return err
		}
	}

	if report.Speed.Enabled {
		if _, err := writeSectionBanner(w, SectionSpeed); err != nil {
			return err
		}
		if err := writeSpeedBody(w, report.Speed); err != nil {
			return err
		}
	}

	if report.IPQuality.Enabled {
		if _, err := writeSectionBanner(w, SectionIPQuality); err != nil {
			return err
		}
		if err := writeIPQualityBody(w, report.IPQuality); err != nil {
			return err
		}
	}

	if report.Reachability.Enabled {
		if err := writeReachabilityConsole(w, report.Reachability); err != nil {
			return err
		}
	}

	if report.Mail.Enabled {
		if _, err := writeSectionBanner(w, SectionMail); err != nil {
			return err
		}
		if err := writeMailBody(w, report.Mail); err != nil {
			return err
		}
	}

	if report.Media.Enabled {
		if _, err := writeSectionBanner(w, SectionMedia); err != nil {
			return err
		}
		if err := writeMediaBody(w, report.Media); err != nil {
			return err
		}
	}

	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintf(w, "\n%s:\n", i18n.T("report.checkup.warnings")); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeHardwareBody prints the hardware section content: the section state
// plus the embedded run report rendered by the shared console writer.
func writeHardwareBody(w io.Writer, section HardwareSection) error {
	if section.Report != nil {
		if err := gbreport.WriteConsole(w, *section.Report); err != nil {
			return err
		}
	} else if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	return nil
}

// writeRouteBody prints the route section content.
func writeRouteBody(w io.Writer, section RouteSection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if len(section.Results) == 0 {
		return nil
	}
	headers := []string{
		i18n.T("report.checkup.col.target"), i18n.T("report.checkup.col.resolved"),
		i18n.T("report.checkup.col.line"), i18n.T("report.checkup.col.confidence"),
		i18n.T("report.checkup.col.status"),
	}
	rows := make([][]string, 0, len(section.Results))
	for _, item := range section.Results {
		rows = append(rows, []string{
			defaultText(item.Target.Name, strings.TrimSpace(item.Target.City+" "+item.Target.Carrier)),
			defaultText(item.ResolvedTarget, "unknown"),
			RouteLineText(item),
			routeConfidenceText(item.Classification),
			routeStatusText(item),
		})
	}
	return writeGrid(w, headers, rows)
}

// writePingBody prints the ping section content.
func writePingBody(w io.Writer, section PingSection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if len(section.Results) == 0 {
		return nil
	}
	headers := []string{
		i18n.T("report.checkup.col.target"), i18n.T("report.checkup.col.city"), i18n.T("report.checkup.col.carrier"),
		i18n.T("report.checkup.col.ip"), i18n.T("report.checkup.col.probe"), i18n.T("report.checkup.col.connection"),
		i18n.T("report.checkup.col.avg"), i18n.T("report.checkup.col.jitter"), i18n.T("report.checkup.col.loss"),
		i18n.T("report.checkup.col.status"),
	}
	rows := make([][]string, 0, len(section.Results))
	for _, item := range section.Results {
		status := defaultText(item.Status, "unknown")
		if item.Status != "ok" && item.Message != "" {
			status = item.Message
		}
		rows = append(rows, []string{
			defaultText(item.Name, "-"),
			defaultText(item.City, "-"),
			defaultText(item.Carrier, "-"),
			defaultText(item.IPFamily, "-"),
			defaultText(item.ProbeProtocol, "unknown") + "/" + defaultText(item.ProbeTool, "unknown"),
			defaultText(item.ConnectionState, "unknown"),
			formatMaybeFloat(item.AvgLatencyMs, "ms"),
			formatMaybeFloat(item.JitterMs, "ms"),
			fmt.Sprintf("%.0f%%", item.PacketLoss),
			status,
		})
	}
	return writeGrid(w, headers, rows)
}

// writeSpeedBody prints the speed section content.
func writeSpeedBody(w io.Writer, section SpeedSection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if section.Result == nil {
		return nil
	}
	if len(section.Result.Groups) > 0 {
		headers := []string{
			i18n.T("report.checkup.col.group"), i18n.T("report.checkup.col.status"), i18n.T("report.checkup.col.ok"),
			i18n.T("report.checkup.col.fail"), i18n.T("report.checkup.col.dl"), i18n.T("report.checkup.col.ul"),
			i18n.T("report.checkup.col.latency"), i18n.T("report.checkup.col.message"),
		}
		rows := make([][]string, 0, len(section.Result.Groups))
		for _, group := range section.Result.Groups {
			rows = append(rows, []string{
				defaultText(group.ProviderLabel, group.Provider),
				defaultText(group.Status, "unknown"),
				fmt.Sprintf("%d", group.Available),
				fmt.Sprintf("%d", group.Failed),
				formatMaybeFloat(group.SummaryValue("download"), "Mbps"),
				formatMaybeFloat(group.SummaryValue("upload"), "Mbps"),
				formatMaybeFloat(group.SummaryValue("latency"), "ms"),
				defaultText(group.Message, "-"),
			})
		}
		if err := writeGrid(w, headers, rows); err != nil {
			return err
		}
		for _, group := range section.Result.Groups {
			if len(group.Providers) == 0 {
				continue
			}
			if _, err := fmt.Fprintf(w, "\n  [%s]\n", defaultText(group.ProviderLabel, group.Provider)); err != nil {
				return err
			}
			if err := writeSpeedProviderRows(w, group.Providers); err != nil {
				return err
			}
		}
	} else if len(section.Result.Providers) > 0 {
		if err := writeSpeedProviderRows(w, section.Result.Providers); err != nil {
			return err
		}
	}
	return nil
}

// writeIPQualityBody prints the IP quality section content.
func writeIPQualityBody(w io.Writer, section IPQualitySection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	result := section.Result
	if result == nil {
		return nil
	}
	if info := result.BasicInfo; info != nil {
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.checkup.ipSummary", map[string]any{
			"IP":      defaultText(info.IP, "-"),
			"Country": defaultText(info.CountryCode, defaultText(info.Country, "-")),
			"ASN":     fmt.Sprintf("%d", info.ASN),
			"Org":     defaultText(info.Org, defaultText(info.ISP, "-")),
		})); err != nil {
			return err
		}
	}
	if score := result.Score; score != nil {
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.checkup.scoreSummary", map[string]any{
			"Total": fmt.Sprintf("%d", score.Total),
			"Max":   fmt.Sprintf("%d", score.MaxTotal),
			"Level": defaultText(score.Level, "unknown"),
		})); err != nil {
			return err
		}
	}
	if cross := result.IPAPIIS; cross != nil && cross.Supported {
		if _, err := fmt.Fprintf(w, "%s | %s | %s\n", defaultText(cross.Company, "-"), defaultText(cross.ASN, "-"), defaultText(cross.Location, "-")); err != nil {
			return err
		}
	}
	if len(result.Sources) > 0 {
		parts := make([]string, 0, len(result.Sources))
		for _, source := range result.Sources {
			note := source.Source + "=" + source.Status
			if source.Message != "" {
				note += " (" + source.Message + ")"
			}
			parts = append(parts, note)
		}
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.checkup.sources", map[string]any{"Sources": strings.Join(parts, ", ")})); err != nil {
			return err
		}
	}
	if sc := result.SecurityCheck; sc != nil {
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.checkup.securitycheck", map[string]any{"Status": sc.Status, "Suffix": scMessageSuffix(sc)})); err != nil {
			return err
		}
		if len(sc.Fields) > 0 {
			rows := make([][]string, 0, len(sc.Fields))
			for _, field := range sc.Fields {
				rows = append(rows, []string{field.Name, field.Value})
			}
			if err := writeGrid(w, []string{i18n.T("report.checkup.col.field"), i18n.T("report.checkup.col.value")}, rows); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeMailBody prints the mail section content.
func writeMailBody(w io.Writer, section MailSection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if len(section.Results) == 0 {
		return nil
	}
	headers := []string{
		i18n.T("report.checkup.col.port"), i18n.T("report.checkup.col.status"), i18n.T("report.checkup.col.latency"),
		i18n.T("report.checkup.col.method"), i18n.T("report.checkup.col.message"),
	}
	rows := make([][]string, 0, len(section.Results))
	for _, item := range section.Results {
		rows = append(rows, []string{
			defaultText(item.Title, fmt.Sprintf("%d", item.Port)),
			defaultText(item.Status, "unknown"),
			formatMaybeFloat(item.LatencyMs, "ms"),
			defaultText(item.Method, "-"),
			defaultText(item.Message, "-"),
		})
	}
	return writeGrid(w, headers, rows)
}

// writeMediaBody prints the media section with the full per-service table.
func writeMediaBody(w io.Writer, section MediaSection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if section.Result == nil || len(section.Result.Items) == 0 {
		return nil
	}
	if set := strings.TrimSpace(section.Result.Set); set != "" {
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.checkup.mediaSet", map[string]any{"Set": set})); err != nil {
			return err
		}
	}
	headers := []string{
		i18n.T("report.checkup.col.item"), i18n.T("report.checkup.col.ip"), i18n.T("report.checkup.col.region"),
		i18n.T("report.checkup.col.status"), i18n.T("report.checkup.col.message"),
	}
	rows := make([][]string, 0, len(section.Result.Items))
	for _, item := range section.Result.Items {
		rows = append(rows, []string{
			defaultText(item.Title, item.ID),
			defaultText(item.IPVersion, "-"),
			defaultText(item.Region, "-"),
			mediaItemStatus(item),
			defaultText(item.Message, "-"),
		})
	}
	return writeGrid(w, headers, rows)
}

// mediaItemStatus maps a media item to its display status, surfacing the
// restricted-within-available nuance kept in RawStatus.
func mediaItemStatus(item MediaServiceResult) string {
	if item.RawStatus == "Restricted" {
		return "restricted"
	}
	return defaultText(item.Status, "unknown")
}

// writeSectionBanner prints the localized "[Section]" divider.
func writeSectionBanner(w io.Writer, id SectionID) (int, error) {
	return fmt.Fprintf(w, "\n[%s]\n", i18n.SectionLabel(string(id)))
}

// writeSectionState prints the status/message key-value lines of a section.
func writeSectionState(w io.Writer, state SectionState) error {
	if _, err := fmt.Fprintf(w, "%s: %s\n", i18n.T("report.checkup.stateStatus"), defaultText(state.Status, "unknown")); err != nil {
		return err
	}
	if message := strings.TrimSpace(state.Message); message != "" {
		if _, err := fmt.Fprintf(w, "%s: %s\n", i18n.T("report.checkup.stateMessage"), message); err != nil {
			return err
		}
	}
	return nil
}

func writeGrid(w io.Writer, headers []string, rows [][]string) error {
	_, err := fmt.Fprint(w, textgrid.Render(headers, rows, 2))
	return err
}

func writeSpeedProviderRows(w io.Writer, providers []SpeedProviderResult) error {
	headers := []string{
		i18n.T("report.checkup.col.provider"), i18n.T("report.checkup.col.kind"), i18n.T("report.checkup.col.node"),
		i18n.T("report.checkup.col.dl"), i18n.T("report.checkup.col.ul"), i18n.T("report.checkup.col.latency"),
		i18n.T("report.checkup.col.status"), i18n.T("report.checkup.col.message"),
	}
	rows := make([][]string, 0, len(providers))
	for _, item := range providers {
		rows = append(rows, []string{
			defaultText(item.ProviderLabel, item.Provider),
			defaultText(item.Kind, "-"),
			defaultText(item.Node, "-"),
			formatMaybeFloat(item.DownloadMbps, "Mbps"),
			formatMaybeFloat(item.UploadMbps, "Mbps"),
			formatMaybeFloat(item.LatencyMs, "ms"),
			defaultText(item.Status, "unknown"),
			defaultText(item.Message, "-"),
		})
	}
	return writeGrid(w, headers, rows)
}

func defaultText(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func formatMaybeFloat(value float64, suffix string) string {
	if value <= 0 {
		return "-"
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, suffix)
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}
