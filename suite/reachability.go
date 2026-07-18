package suite

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runReachabilitySection(ctx context.Context, opts Options, report *SuiteReport) {
	_ = opts
	section := &report.Reachability
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	section.Results = netio.ProbeDefaultReachability(ctx)
	section.FinishTime = time.Now().Unix()
	section.Status, section.Message = summarizeReachability(section.Results)
}

func summarizeReachability(results []ReachabilityResult) (string, string) {
	if len(results) == 0 {
		return "error", "no reachability targets selected"
	}
	reachable := 0
	for _, result := range results {
		if result.Status == netio.ReachabilityStatusReachable {
			reachable++
		}
	}
	failed := len(results) - reachable
	return statusFromCounts(reachable, failed), fmt.Sprintf("%d/%d targets reachable", reachable, len(results))
}
