package suite

import (
	"context"
	"fmt"
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

	openCount := 0
	for _, item := range results {
		if item.Status == "open" {
			openCount++
		}
	}
	if len(results) == 0 {
		section.Status = "error"
		section.Message = "no mail ports selected"
		return
	}
	section.Status = "ok"
	section.Message = fmt.Sprintf("%d/%d ports open", openCount, len(results))
}
