package netio

import (
	"testing"

	"github.com/cloudapp3/vmbench/bench"
)

func TestNetworkWorkloadsLimitIterations(t *testing.T) {
	workloads := []bench.Workload{
		NewDownloadWorkload(SpeedNode{Name: "test", TestURL: "https://example.com"}),
		NewMultiDownloadWorkload(),
		NewUploadWorkload(),
		NewPingWorkload(),
		NewTracerouteWorkload(),
		NewIPQualityWorkload(),
		NewStreamingUnlockWorkload(),
		NewIperfWorkload("127.0.0.1", 1),
	}

	for _, workload := range workloads {
		limiter, ok := workload.(bench.IterationLimiter)
		if !ok {
			t.Errorf("%s does not implement bench.IterationLimiter", workload.Name())
			continue
		}
		if got := limiter.MaxIterations(); got != 1 {
			t.Errorf("%s MaxIterations() = %d, want 1", workload.Name(), got)
		}
	}
}

func TestNetworkWorkloadProcessedMetricSemantics(t *testing.T) {
	byteWorkloads := []bench.Workload{
		NewDownloadWorkload(SpeedNode{Name: "test", TestURL: "https://example.com"}),
		NewMultiDownloadWorkload(),
		NewUploadWorkload(),
	}
	for _, workload := range byteWorkloads {
		if got := networkProcessedKind(workload); got != bench.ProcessedBytes {
			t.Errorf("%s processed kind = %v, want bytes", workload.Name(), got)
		}
	}

	nonCumulativeWorkloads := []bench.Workload{
		NewPingWorkload(),
		NewTracerouteWorkload(),
		NewIPQualityWorkload(),
		NewStreamingUnlockWorkload(),
		NewIperfWorkload("127.0.0.1", 1),
	}
	for _, workload := range nonCumulativeWorkloads {
		if got := networkProcessedKind(workload); got != bench.ProcessedUnknown {
			t.Errorf("%s processed kind = %v, want unknown", workload.Name(), got)
		}
	}
}

func networkProcessedKind(workload bench.Workload) bench.ProcessedKind {
	reporter, ok := workload.(bench.ProcessedMetricReporter)
	if !ok {
		return bench.ProcessedUnknown
	}
	return reporter.ProcessedKind()
}
