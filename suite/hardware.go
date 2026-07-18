package suite

import (
	"context"
	"fmt"
	"time"

	vmbench "github.com/cloudapp3/vmbench"
)

func runHardwareSection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.Hardware
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	result := vmbench.RunCore(ctx, vmbench.Options{
		DiskPath:      opts.DiskPath,
		Timeout:       opts.Timeout,
		Iterations:    opts.Iterations,
		Filter:        opts.Filter,
		Mode:          "single",
		Engine:        "external",
		Scope:         vmbench.ScopeHardware,
		HardwareTools: append([]string(nil), opts.HardwareTools...),
	})
	section.Report = &result
	section.FinishTime = time.Now().Unix()
	if len(result.Results.Workloads) == 0 && len(result.Extensions.Workloads) == 0 {
		section.Status = "error"
		section.Message = "hardware benchmark produced no workloads"
	} else {
		okCount, failCount := 0, 0
		for _, item := range append(result.Results.Workloads, result.Extensions.Workloads...) {
			if item.Result == nil || item.Result.Error != "" {
				failCount++
				continue
			}
			okCount++
		}
		section.Message = fmt.Sprintf("%d ok/%d failed", okCount, failCount)
		section.Status = statusFromCounts(okCount, failCount)
	}
	report.Warnings = append(report.Warnings, result.Warnings...)
}
