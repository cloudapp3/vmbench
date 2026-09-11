package checkup

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/i18n"
)

// WriteMarkdown writes a forum-pasteable markdown summary of a checkup run:
// one heading per section with the section body inside a fenced code block,
// and the media section folded to per-region counts plus exception items so
// the paste stays readable with 200+ probed services.
func WriteMarkdown(w io.Writer, report CheckupReport) error {
	if w == nil {
		w = io.Discard
	}
	if _, err := fmt.Fprintf(w, "# %s\n\n", i18n.Tf("report.markdown.checkupTitle", map[string]any{"Version": defaultText(report.App.Version, "unknown")})); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "> %s\n\n", checkupMetaLine(report)); err != nil {
		return err
	}

	if report.Hardware.Enabled {
		if err := writeMarkdownSection(w, SectionHardware, func(w io.Writer) error {
			return writeHardwareBody(w, report.Hardware)
		}); err != nil {
			return err
		}
	}
	if report.NetworkInfo.Enabled {
		if err := writeMarkdownSection(w, SectionNetworkInfo, func(w io.Writer) error {
			return writeNetworkInfoBody(w, report.NetworkInfo)
		}); err != nil {
			return err
		}
	}
	if report.Route.Enabled {
		if err := writeMarkdownSection(w, SectionRoute, func(w io.Writer) error {
			return writeRouteBody(w, report.Route)
		}); err != nil {
			return err
		}
	}
	if report.Ping.Enabled {
		if err := writeMarkdownSection(w, SectionPing, func(w io.Writer) error {
			return writePingBody(w, report.Ping)
		}); err != nil {
			return err
		}
	}
	if report.Speed.Enabled {
		if err := writeMarkdownSection(w, SectionSpeed, func(w io.Writer) error {
			return writeSpeedBody(w, report.Speed)
		}); err != nil {
			return err
		}
	}
	if report.IPQuality.Enabled {
		if err := writeMarkdownSection(w, SectionIPQuality, func(w io.Writer) error {
			return writeIPQualityBody(w, report.IPQuality)
		}); err != nil {
			return err
		}
	}
	if report.Reachability.Enabled {
		if err := writeMarkdownSection(w, SectionReachability, func(w io.Writer) error {
			return writeReachabilityBody(w, report.Reachability)
		}); err != nil {
			return err
		}
	}
	if report.Mail.Enabled {
		if err := writeMarkdownSection(w, SectionMail, func(w io.Writer) error {
			return writeMailBody(w, report.Mail)
		}); err != nil {
			return err
		}
	}
	if report.Media.Enabled {
		if err := writeMarkdownSection(w, SectionMedia, func(w io.Writer) error {
			return writeMediaFoldBody(w, report.Media)
		}); err != nil {
			return err
		}
	}

	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintf(w, "\n## %s\n\n", i18n.T("report.checkup.warnings")); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", strings.TrimSpace(warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeMarkdownSection renders one section as a heading plus the section
// body wrapped in a fenced code block.
func writeMarkdownSection(w io.Writer, id SectionID, body func(io.Writer) error) error {
	if _, err := fmt.Fprintf(w, "## %s\n\n", i18n.SectionLabel(string(id))); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "```"); err != nil {
		return err
	}
	if err := body(w); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "```\n\n")
	return err
}

// checkupMetaLine renders the provenance quote: run status, preset, binary
// version, start time, duration, and the resolved node catalog.
func checkupMetaLine(report CheckupReport) string {
	parts := []string{
		fmt.Sprintf("%s: %s", i18n.T("report.checkup.status"), defaultText(report.Status, "unknown")),
	}
	if preset := strings.TrimSpace(report.Config.Preset); preset != "" {
		parts = append(parts, fmt.Sprintf("%s: %s", i18n.T("report.checkup.preset"), preset))
	}
	version := defaultText(report.App.Version, "unknown")
	if commit := strings.TrimSpace(report.App.Commit); commit != "" {
		version += " (" + commit + ")"
	}
	parts = append(parts, "vmbench "+version)
	if !report.StartedAt.IsZero() {
		parts = append(parts, report.StartedAt.UTC().Format(time.RFC3339))
	}
	if report.DurationMS > 0 {
		parts = append(parts, i18n.Tf("report.markdown.duration", map[string]any{
			"Duration": formatMarkdownDuration(report.DurationMS),
		}))
	}
	if source := strings.TrimSpace(report.Config.CatalogSource); source != "" {
		catalog := source
		if revision := strings.TrimSpace(report.Config.CatalogRevision); revision != "" {
			catalog += "@" + revision
		}
		parts = append(parts, "catalog "+catalog)
	}
	return strings.Join(parts, " · ")
}

// formatMarkdownDuration renders milliseconds as a compact m/h/s duration.
func formatMarkdownDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	seconds := (ms + 999) / 1000
	hours := seconds / 3600
	seconds %= 3600
	minutes := seconds / 60
	seconds %= 60
	var b strings.Builder
	if hours > 0 {
		fmt.Fprintf(&b, "%dh", hours)
	}
	if minutes > 0 || hours > 0 {
		fmt.Fprintf(&b, "%dm", minutes)
	}
	fmt.Fprintf(&b, "%ds", seconds)
	return b.String()
}

// writeMediaFoldBody prints the media section folded for sharing: aggregate
// counts, one line per region, and only the exception services in full.
func writeMediaFoldBody(w io.Writer, section MediaSection) error {
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	result := section.Result
	if result == nil || len(result.Items) == 0 {
		return nil
	}
	if set := strings.TrimSpace(result.Set); set != "" {
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.checkup.mediaSet", map[string]any{"Set": set})); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.markdown.mediaTotals", map[string]any{
		"Available":  fmt.Sprintf("%d", result.Summary.Available),
		"Restricted": fmt.Sprintf("%d", result.Summary.Restricted),
		"Blocked":    fmt.Sprintf("%d", result.Summary.Blocked),
		"Unknown":    fmt.Sprintf("%d", result.Summary.Unknown),
	})); err != nil {
		return err
	}
	for _, region := range mediaRegionOrder(result.Items) {
		available, total := 0, 0
		for _, item := range result.Items {
			if item.Region != region {
				continue
			}
			total++
			if item.Status == "available" {
				available++
			}
		}
		if _, err := fmt.Fprintf(w, "%s\n", i18n.Tf("report.markdown.mediaRegion", map[string]any{
			"Region":    defaultText(region, "-"),
			"Available": fmt.Sprintf("%d", available),
			"Total":     fmt.Sprintf("%d", total),
		})); err != nil {
			return err
		}
	}
	exceptions := mediaExceptionItems(result.Items)
	if len(exceptions) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s\n", i18n.T("report.markdown.mediaExceptions")); err != nil {
		return err
	}
	for _, item := range exceptions {
		line := fmt.Sprintf("  - %s [%s] %s", defaultText(item.Title, item.ID), defaultText(item.Region, "-"), mediaItemStatus(item))
		if message := strings.TrimSpace(item.Message); message != "" {
			line += " " + message
		}
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}

// mediaRegionOrder returns the distinct regions in first-appearance order.
func mediaRegionOrder(items []MediaServiceResult) []string {
	seen := make(map[string]bool, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		if seen[item.Region] {
			continue
		}
		seen[item.Region] = true
		order = append(order, item.Region)
	}
	return order
}

// mediaExceptionItems returns the services worth listing individually in the
// folded view: anything not plain-available, including restricted unlocks.
func mediaExceptionItems(items []MediaServiceResult) []MediaServiceResult {
	exceptions := make([]MediaServiceResult, 0)
	for _, item := range items {
		if item.Status != "available" || item.RawStatus == "Restricted" {
			exceptions = append(exceptions, item)
		}
	}
	return exceptions
}
