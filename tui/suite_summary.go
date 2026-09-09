package tui

import (
	"encoding/json"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/suite"
)

// sectionEnabled mirrors SectionSelector's bools by ID; the selector has no
// per-ID accessor.
func sectionEnabled(sel suite.SectionSelector, id suite.SectionID) bool {
	switch id {
	case suite.SectionHardware:
		return sel.Hardware
	case suite.SectionNetworkInfo:
		return sel.NetworkInfo
	case suite.SectionRoute:
		return sel.Route
	case suite.SectionPing:
		return sel.Ping
	case suite.SectionSpeed:
		return sel.Speed
	case suite.SectionIPQuality:
		return sel.IPQuality
	case suite.SectionReachability:
		return sel.Reachability
	case suite.SectionMail:
		return sel.Mail
	case suite.SectionMedia:
		return sel.Media
	}
	return false
}

// sectionStates extracts each typed section's SectionState for averaging.
func sectionStates(rep suite.SuiteReport) map[suite.SectionID]suite.SectionState {
	return map[suite.SectionID]suite.SectionState{
		suite.SectionHardware:     rep.Hardware.SectionState,
		suite.SectionNetworkInfo:  rep.NetworkInfo.SectionState,
		suite.SectionRoute:        rep.Route.SectionState,
		suite.SectionPing:         rep.Ping.SectionState,
		suite.SectionSpeed:        rep.Speed.SectionState,
		suite.SectionIPQuality:    rep.IPQuality.SectionState,
		suite.SectionReachability: rep.Reachability.SectionState,
		suite.SectionMail:         rep.Mail.SectionState,
		suite.SectionMedia:        rep.Media.SectionState,
	}
}

// historyStats averages per-section wall times from stored suite reports.
// SectionState granularity is one second, so short sections are coarse;
// the estimate is advisory only.
type historyStats struct {
	avg     map[suite.SectionID]time.Duration
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
		stats := historyStats{avg: map[suite.SectionID]time.Duration{}}
		totals := map[suite.SectionID]time.Duration{}
		counts := map[suite.SectionID]int{}
		seen := 0
		for _, rec := range records {
			if rec.Kind != history.KindSuite || seen >= historyStatsLimit {
				continue
			}
			seen++
			var rep suite.SuiteReport
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
func roughSectionSeconds(id suite.SectionID, iterations int) time.Duration {
	base := map[suite.SectionID]float64{
		suite.SectionHardware:     150,
		suite.SectionNetworkInfo:  10,
		suite.SectionRoute:        20,
		suite.SectionPing:         25,
		suite.SectionSpeed:        30,
		suite.SectionIPQuality:    15,
		suite.SectionReachability: 10,
		suite.SectionMail:         10,
		suite.SectionMedia:        30,
	}[id]
	if id == suite.SectionHardware {
		scale := float64(iterations) / 3.0
		if scale < 0.34 {
			scale = 0.34
		}
		base *= scale
	}
	return time.Duration(base * float64(time.Second))
}

func estimateSectionDuration(id suite.SectionID, s configState, stats historyStats) time.Duration {
	if avg, ok := stats.avg[id]; ok && avg > 0 {
		return avg
	}
	return roughSectionSeconds(id, s.iterations)
}

func estimateSuiteDuration(s configState, stats historyStats) time.Duration {
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
			return i18n.Tf("tui.suiteSummary.minutes", map[string]any{"Minutes": m})
		}
		return i18n.Tf("tui.suiteSummary.minutesSeconds", map[string]any{"Minutes": m, "Seconds": sec})
	default:
		return i18n.Tf("tui.suiteSummary.seconds", map[string]any{"Seconds": int(d.Seconds())})
	}
}
