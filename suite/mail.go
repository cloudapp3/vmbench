package suite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runMailSection(ctx context.Context, opts Options, report *SuiteReport) {
	_ = opts
	section := &report.Mail
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	results := netio.ProbeMailPorts(ctx, nil)
	section.FinishTime = time.Now().Unix()
	section.Results = results
	section.Status, section.Message = summarizeMailResults(results)
}

func summarizeMailResults(results []netio.PortProbe) (string, string) {
	if len(results) == 0 {
		return "error", "no mail ports selected"
	}
	openCount := 0
	refusedCount := 0
	timeoutCount := 0
	errorCount := 0
	for _, item := range results {
		switch strings.ToLower(strings.TrimSpace(item.Status)) {
		case netio.MailPortStatusOpen:
			openCount++
		case netio.MailPortStatusRefused:
			refusedCount++
		case netio.MailPortStatusTimeout:
			timeoutCount++
		default:
			errorCount++
		}
	}
	status := "ok"
	if errorCount == len(results) {
		status = "error"
	} else if errorCount > 0 {
		status = "partial"
	}
	parts := []string{fmt.Sprintf("%d/%d ports open", openCount, len(results))}
	if refusedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d refused", refusedCount))
	}
	if timeoutCount > 0 {
		parts = append(parts, fmt.Sprintf("%d timeout", timeoutCount))
	}
	if errorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errorCount))
	}
	return status, strings.Join(parts, "; ")
}
