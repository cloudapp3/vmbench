package suite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runNetworkInfoSection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.NetworkInfo
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	result, err := netio.ProbeNetworkIdentity(ctx, opts.IPVersion)
	section.FinishTime = time.Now().Unix()
	section.Result = result
	section.Status, section.Message = summarizeNetworkIdentity(result, err)
}

func summarizeNetworkIdentity(result *NetworkIdentityResult, probeErr error) (string, string) {
	if result == nil {
		if probeErr != nil {
			return "error", probeErr.Error()
		}
		return "error", "network identity result missing"
	}
	okCount := 0
	failCount := 0
	for _, provider := range result.Providers {
		switch provider.Status {
		case netio.NetworkIdentityProviderOK:
			okCount++
		case netio.NetworkIdentityProviderSkipped:
			continue
		default:
			failCount++
		}
	}
	status := statusFromCounts(okCount, failCount)
	if probeErr != nil {
		if okCount > 0 {
			status = "partial"
		} else {
			status = "error"
		}
	} else if status == "skipped" {
		status = "error"
	}
	parts := []string{fmt.Sprintf("%d/%d providers ok", okCount, okCount+failCount)}
	for _, nat := range result.NAT {
		parts = append(parts, fmt.Sprintf("%s NAT %s", nat.IPVersion, nat.Status))
	}
	if probeErr != nil {
		parts = append(parts, probeErr.Error())
	}
	return status, strings.Join(parts, " · ")
}
