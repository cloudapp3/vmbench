package suite

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runMediaSection(ctx context.Context, opts Options, report *SuiteReport) {
	_ = opts
	section := &report.Media
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	result, err := netio.ProbeMedia(ctx)
	section.FinishTime = time.Now().Unix()
	if err != nil {
		section.Status = "error"
		section.Message = err.Error()
		return
	}
	section.Result = result
	section.Status = "ok"
	section.Message = fmt.Sprintf("available %d · blocked %d · unknown %d", result.Summary.Available, result.Summary.Blocked, result.Summary.Unknown)
}
