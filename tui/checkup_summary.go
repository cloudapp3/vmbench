package tui

import (
	"encoding/json"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
)

// sectionEnabled mirrors SectionSelector's bools by ID; the selector has no
// per-ID accessor.
func sectionEnabled(sel checkup.SectionSelector, id checkup.SectionID) bool {
	switch id {
	case checkup.SectionHardware:
		return sel.Hardware
	case checkup.SectionNetworkInfo:
		return sel.NetworkInfo
	case checkup.SectionRoute:
		return sel.Route
	case checkup.SectionPing:
		return sel.Ping
	case checkup.SectionSpeed:
		return sel.Speed
	case checkup.SectionIPQuality:
		return sel.IPQuality
	case checkup.SectionReachability:
		return sel.Reachability
	case checkup.SectionMail:
		return sel.Mail
	case checkup.SectionMedia:
		return sel.Media
	}
	return false
}

// sectionStates extracts each typed section's SectionState for averaging.
func sectionStates(rep checkup.CheckupReport) map[checkup.SectionID]checkup.SectionState {
	return map[checkup.SectionID]checkup.SectionState{
		checkup.SectionHardware:     rep.Hardware.SectionState,
		checkup.SectionNetworkInfo:  rep.NetworkInfo.SectionState,
		checkup.SectionRoute:        rep.Route.SectionState,
		checkup.SectionPing:         rep.Ping.SectionState,
		checkup.SectionSpeed:        rep.Speed.SectionState,
		checkup.SectionIPQuality:    rep.IPQuality.SectionState,
		checkup.SectionReachability: rep.Reachability.SectionState,
		checkup.SectionMail:         rep.Mail.SectionState,
		checkup.SectionMedia:        rep.Media.SectionState,
	}
}

// historyStats averages per-section wall times from stored checkup reports.
// SectionState granularity is one second, so short sections are coarse;
// the estimate is advisory only.
type historyStats struct {
	avg     map[checkup.SectionID]time.Duration
	samples int
}

type historyStatsMsg struct{ stats historyStats }

const historyStatsLimit = 10

func loadHistoryStatsCmd() tea.Cmd {
	return func() tea.Msg {
		store, err := history.Open("")
		if err != nil {
			return historyStatsMsg{}
		}
		records, err := store.List()
		if err != nil {
			return historyStatsMsg{}
		}
		stats := historyStats{avg: map[checkup.SectionID]time.Duration{}}
		totals := map[checkup.SectionID]time.Duration{}
		counts := map[checkup.SectionID]int{}
		seen := 0
		for _, rec := range records {
			if rec.Kind != history.KindCheckup || seen >= historyStatsLimit {
				continue
			}
			seen++
			var rep checkup.CheckupReport
			if err := json.Unmarshal(rec.Report, &rep); err != nil {
				continue
			}
			for id, st := range sectionStates(rep) {
				if st.StartedTime > 0 && st.FinishTime >= st.StartedTime {
					totals[id] += time.Duration(st.FinishTime-st.StartedTime) * time.Second
					counts[id]++
				}
			}
		}
		for id, total := range totals {
			stats.avg[id] = total / time.Duration(counts[id])
		}
		stats.samples = seen
		return historyStatsMsg{stats: stats}
	}
}

// roughSectionSeconds are fallback estimates when no history exists.
// Advisory only; hardware scales with the configured iteration count.
func roughSectionSeconds(id checkup.SectionID, iterations int) time.Duration {
	base := map[checkup.SectionID]float64{
		checkup.SectionHardware:     150,
		checkup.SectionNetworkInfo:  10,
		checkup.SectionRoute:        20,
		checkup.SectionPing:         25,
		checkup.SectionSpeed:        30,
		checkup.SectionIPQuality:    15,
		checkup.SectionReachability: 10,
		checkup.SectionMail:         10,
		checkup.SectionMedia:        30,
	}[id]
	if id == checkup.SectionHardware {
		scale := float64(iterations) / 3.0
		if scale < 0.34 {
			scale = 0.34
		}
		base *= scale
	}
	return time.Duration(base * float64(time.Second))
}

func estimateSectionDuration(id checkup.SectionID, s configState, stats historyStats) time.Duration {
	if avg, ok := stats.avg[id]; ok && avg > 0 {
		return avg
	}
	return roughSectionSeconds(id, s.iterations)
}

func estimateCheckupDuration(s configState, stats historyStats) time.Duration {
	total := time.Duration(0)
	for _, id := range s.sectionIDs {
		if !sectionEnabled(s.sections, id) {
			continue
		}
		total += estimateSectionDuration(id, s, stats)
	}
	return total.Round(time.Second)
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Minute:
		m := int(d.Minutes())
		sec := int(d.Seconds()) % 60
		if sec == 0 {
			return i18n.Tf("tui.checkupSummary.minutes", map[string]any{"Minutes": m})
		}
		return i18n.Tf("tui.checkupSummary.minutesSeconds", map[string]any{"Minutes": m, "Seconds": sec})
	default:
		return i18n.Tf("tui.checkupSummary.seconds", map[string]any{"Seconds": int(d.Seconds())})
	}
}
