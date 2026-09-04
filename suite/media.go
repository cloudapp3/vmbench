package suite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runMediaSection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.Media
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	result, err := netio.ProbeMedia(ctx, netio.MediaProbeOptions{
		Set:       opts.MediaSet,
		IPVersion: opts.IPVersion,
	})
	section.FinishTime = time.Now().Unix()
	// Keep partial evidence even when the probe reports an error.
	section.Result = result
	if err != nil {
		section.Status = "error"
		section.Message = err.Error()
		return
	}
	section.Status = "ok"
	set := strings.TrimSpace(result.Set)
	if set == "" {
		set = "all"
	}
	section.Message = fmt.Sprintf("set %s · available %d · restricted %d · blocked %d · unknown %d",
		set, result.Summary.Available, result.Summary.Restricted, result.Summary.Blocked, result.Summary.Unknown)
}
