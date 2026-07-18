package suite

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runIPQualitySection(ctx context.Context, opts Options, report *SuiteReport) {
	_ = opts
	section := &report.IPQuality
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	result, err := netio.ProbeIPQuality(ctx)
	section.FinishTime = time.Now().Unix()
	section.Result = result
	if err != nil {
		section.Status = "error"
		section.Message = err.Error()
		return
	}
	if result.Score != nil {
		section.Status = "ok"
		section.Message = fmt.Sprintf("score %d/%d (%s)", result.Score.Total, result.Score.MaxTotal, result.Score.Level)
		return
	}
	section.Status = "error"
	section.Message = "ip quality result missing score"
}
