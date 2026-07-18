package suite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/sysinfo"
)

func Run(ctx context.Context, opts Options) SuiteReport {
	if ctx == nil {
		ctx = context.Background()
	}
	prepared := PrepareOptions(opts)
	norm, configErr := NormalizeOptions(opts)
	if configErr != nil {
		norm = prepared
	}
	report := NewSuiteReport(norm)
	started := time.Now().UTC()
	report.StartedAt = started
	report.StartedTime = started.Unix()
	report.UpdatedTime = report.StartedTime
	report.Status = "running"
	report.Message = "running"
	if !norm.Sections.Hardware || configErr != nil {
		report.System, report.Warnings = sysinfo.Collect(ctx)
	}
	if norm.CatalogWarning != "" {
		report.Warnings = append(report.Warnings, norm.CatalogWarning)
	}

	emit := func(kind EventKind, sec SectionID, status, msg string) {
		if opts.OnEvent == nil {
			return
		}
		opts.OnEvent(Event{Kind: kind, Section: sec, Status: status, Message: msg, Time: time.Now()})
	}
	if configErr != nil {
		message := "configuration: " + configErr.Error()
		report.Warnings = append(report.Warnings, message)
		failSuitePreflight(&report, message, emit)
		finished := time.Now().UTC()
		report.FinishedAt = finished
		report.DurationMS = finished.Sub(started).Milliseconds()
		report.UpdatedTime = finished.Unix()
		report.FinishedTime = report.UpdatedTime
		report.Status, report.Message = finalize(report)
		emit(EventSuiteDone, "", report.Status, report.Message)
		return report
	}

	runOne := func(sec SectionID, enabled bool, fn func(context.Context), getState func() *SectionState) {
		if !enabled {
			emit(EventSectionSkip, sec, "skipped", "")
			return
		}
		emit(EventSectionStart, sec, "running", "")
		runSectionWithContext(ctx, norm.Timeout, sectionUsesTimeout(sec), fn, getState())
		st := getState()
		kind := EventSectionDone
		if !sectionStatusOK(st.Status) {
			kind = EventSectionFail
		}
		emit(kind, sec, st.Status, st.Message)
	}

	runOne(SectionHardware, norm.Sections.Hardware,
		func(runCtx context.Context) { runHardwareSection(runCtx, norm, &report) },
		func() *SectionState { return &report.Hardware.SectionState })
	if report.Hardware.Report != nil {
		report.System = report.Hardware.Report.System
	}
	runOne(SectionNetworkInfo, norm.Sections.NetworkInfo,
		func(runCtx context.Context) { runNetworkInfoSection(runCtx, norm, &report) },
		func() *SectionState { return &report.NetworkInfo.SectionState })
	runOne(SectionRoute, norm.Sections.Route,
		func(runCtx context.Context) { runRouteSection(runCtx, norm, &report) },
		func() *SectionState { return &report.Route.SectionState })
	runOne(SectionPing, norm.Sections.Ping,
		func(runCtx context.Context) { runPingSection(runCtx, norm, &report) },
		func() *SectionState { return &report.Ping.SectionState })
	runOne(SectionSpeed, norm.Sections.Speed,
		func(runCtx context.Context) { runSpeedSection(runCtx, norm, &report) },
		func() *SectionState { return &report.Speed.SectionState })
	runOne(SectionIPQuality, norm.Sections.IPQuality,
		func(runCtx context.Context) { runIPQualitySection(runCtx, norm, &report) },
		func() *SectionState { return &report.IPQuality.SectionState })
	runOne(SectionReachability, norm.Sections.Reachability,
		func(runCtx context.Context) { runReachabilitySection(runCtx, norm, &report) },
		func() *SectionState { return &report.Reachability.SectionState })
	runOne(SectionMail, norm.Sections.Mail,
		func(runCtx context.Context) { runMailSection(runCtx, norm, &report) },
		func() *SectionState { return &report.Mail.SectionState })
	runOne(SectionMedia, norm.Sections.Media,
		func(runCtx context.Context) { runMediaSection(runCtx, norm, &report) },
		func() *SectionState { return &report.Media.SectionState })

	finished := time.Now().UTC()
	report.FinishedAt = finished
	report.DurationMS = finished.Sub(started).Milliseconds()
	report.UpdatedTime = finished.Unix()
	report.FinishedTime = report.UpdatedTime
	report.Status, report.Message = finalize(report)
	emit(EventSuiteDone, "", report.Status, report.Message)
	return report
}

func failSuitePreflight(report *SuiteReport, message string, emit func(EventKind, SectionID, string, string)) {
	now := time.Now().Unix()
	sections := []struct {
		id    SectionID
		state *SectionState
	}{
		{SectionHardware, &report.Hardware.SectionState},
		{SectionNetworkInfo, &report.NetworkInfo.SectionState},
		{SectionRoute, &report.Route.SectionState},
		{SectionPing, &report.Ping.SectionState},
		{SectionSpeed, &report.Speed.SectionState},
		{SectionIPQuality, &report.IPQuality.SectionState},
		{SectionReachability, &report.Reachability.SectionState},
		{SectionMail, &report.Mail.SectionState},
		{SectionMedia, &report.Media.SectionState},
	}
	for _, section := range sections {
		if !section.state.Enabled {
			emit(EventSectionSkip, section.id, "skipped", "")
			continue
		}
		section.state.Status = "error"
		section.state.Message = message
		section.state.StartedTime = now
		section.state.FinishTime = now
		emit(EventSectionFail, section.id, "error", message)
	}
}

func sectionUsesTimeout(section SectionID) bool {
	switch section {
	case SectionNetworkInfo, SectionRoute, SectionPing, SectionSpeed, SectionIPQuality, SectionReachability, SectionMail, SectionMedia:
		return true
	default:
		return false
	}
}

func runSectionWithContext(parent context.Context, timeout time.Duration, applyTimeout bool, run func(context.Context), state *SectionState) {
	runCtx := parent
	cancel := func() {}
	if applyTimeout {
		runCtx, cancel = context.WithTimeout(parent, timeout)
	}

	run(runCtx)
	contextErr := runCtx.Err()
	cancel()
	if !applyTimeout || contextErr == nil {
		return
	}

	state.Status = "error"
	switch {
	case errors.Is(contextErr, context.DeadlineExceeded):
		state.Message = "section timed out: " + contextErr.Error()
	default:
		state.Message = "section canceled: " + contextErr.Error()
	}
	if state.FinishTime == 0 {
		state.FinishTime = time.Now().Unix()
	}
}

func finalize(report SuiteReport) (string, string) {
	enabled := 0
	okCount := 0
	failures := make([]string, 0, 5)
	for _, item := range report.Sections() {
		if !item.Enabled {
			continue
		}
		enabled++
		if sectionStatusOK(item.Status) {
			okCount++
		} else {
			failures = append(failures, string(item.ID))
		}
	}
	if enabled == 0 {
		return "failed", "no sections enabled"
	}
	if len(failures) > 0 {
		return "failed", fmt.Sprintf("%d/%d sections ok; failed: %s", okCount, enabled, strings.Join(failures, ", "))
	}
	return "ok", fmt.Sprintf("%d/%d sections ok", okCount, enabled)
}

func sectionStatusOK(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "ok")
}
