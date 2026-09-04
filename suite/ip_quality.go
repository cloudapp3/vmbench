package suite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runIPQualitySection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.IPQuality
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	result, err := netio.ProbeIPQualityWithOptions(ctx, netio.IPQualityOptions{Sources: opts.IPSources})
	section.FinishTime = time.Now().Unix()
	section.Result = result
	if err != nil {
		section.Status = "error"
		section.Message = err.Error()
		return
	}
	if result.Score != nil {
		section.Status = "ok"
		message := fmt.Sprintf("score %d/%d (%s)", result.Score.Total, result.Score.MaxTotal, result.Score.Level)
		if notes := unavailableSourceNotes(result); len(notes) > 0 {
			message += "; " + strings.Join(notes, "; ")
		}
		section.Message = message
		return
	}
	section.Status = "error"
	section.Message = "ip quality result missing score"
}

// unavailableSourceNotes surfaces supplementary evidence sources that were
// requested but could not contribute, so an ok section never hides them.
func unavailableSourceNotes(result *netio.IPQualityResult) []string {
	var notes []string
	for _, source := range result.Sources {
		switch source.Status {
		case "unavailable", "error":
			note := source.Source + " " + source.Status
			if source.Message != "" {
				note += ": " + source.Message
			}
			notes = append(notes, note)
		}
	}
	return notes
}

// scMessageSuffix renders a short securitycheck annotation for console output.
func scMessageSuffix(sc *netio.SecurityCheckResult) string {
	if sc.Message == "" {
		return ""
	}
	return " (" + sc.Message + ")"
}
