package bench

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type progressTestWorkload struct {
	name  string
	delay time.Duration
}

func (w progressTestWorkload) Name() string        { return w.name }
func (w progressTestWorkload) Category() string    { return CategoryInteger }
func (w progressTestWorkload) Description() string { return "progress serialization test workload" }
func (w progressTestWorkload) Validate() error     { return nil }
func (w progressTestWorkload) SkipWarmup() bool    { return true }

func (w progressTestWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case <-time.After(w.delay):
	}
	return w.delay, 1, nil
}

type concurrencyTracker struct {
	mu        sync.Mutex
	active    int
	maxActive int
}

func (t *concurrencyTracker) start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active++
	if t.active > t.maxActive {
		t.maxActive = t.active
	}
}

func (t *concurrencyTracker) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active--
}

func (t *concurrencyTracker) maximum() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxActive
}

type trackedWorkload struct {
	progressTestWorkload
	tracker *concurrencyTracker
}

func (w trackedWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	w.tracker.start()
	defer w.tracker.finish()
	return w.progressTestWorkload.Run(ctx)
}

func TestRunAllMultiCoreStillRunsWorkloadsSequentially(t *testing.T) {
	tracker := &concurrencyTracker{}
	workloads := []Workload{
		trackedWorkload{progressTestWorkload: progressTestWorkload{name: "slow-1", delay: 10 * time.Millisecond}, tracker: tracker},
		trackedWorkload{progressTestWorkload: progressTestWorkload{name: "slow-2", delay: 10 * time.Millisecond}, tracker: tracker},
		trackedWorkload{progressTestWorkload: progressTestWorkload{name: "slow-3", delay: 10 * time.Millisecond}, tracker: tracker},
	}

	results, warnings := RunAll(context.Background(), workloads, RunConfig{
		Iterations: 1,
		MultiCore:  true,
	})
	if len(warnings) > 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(results) != len(workloads) {
		t.Fatalf("results len = %d, want %d", len(results), len(workloads))
	}
	if got := tracker.maximum(); got != 1 {
		t.Fatalf("maximum concurrent workloads = %d, want 1", got)
	}
	for i, result := range results {
		want := fmt.Sprintf("slow-%d", i+1)
		if result.Workload != want {
			t.Fatalf("result[%d].Workload = %q, want %q", i, result.Workload, want)
		}
	}
}

type limitedWorkload struct {
	name          string
	maxIterations int
	runs          int
}

func (w *limitedWorkload) Name() string        { return w.name }
func (w *limitedWorkload) Category() string    { return CategoryInteger }
func (w *limitedWorkload) Description() string { return "iteration limit test workload" }
func (w *limitedWorkload) Validate() error     { return nil }
func (w *limitedWorkload) SkipWarmup() bool    { return true }
func (w *limitedWorkload) MaxIterations() int  { return w.maxIterations }

func (w *limitedWorkload) Run(context.Context) (time.Duration, int64, error) {
	w.runs++
	return time.Millisecond, int64(w.runs), nil
}

func TestRunAllAppliesIterationLimitsToResultsAndProgress(t *testing.T) {
	limited := &limitedWorkload{name: "limited", maxIterations: 2}
	unlimited := &limitedWorkload{name: "unlimited"}
	var events []ProgressEvent

	results, warnings := RunAll(context.Background(), []Workload{limited, unlimited}, RunConfig{
		Iterations: 3,
		OnProgress: func(event ProgressEvent) {
			events = append(events, event)
		},
	})
	if len(warnings) > 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if limited.runs != 2 {
		t.Fatalf("limited runs = %d, want 2", limited.runs)
	}
	if unlimited.runs != 3 {
		t.Fatalf("unlimited runs = %d, want 3", unlimited.runs)
	}
	if got := results[0].Result.Iterations; got != 2 {
		t.Fatalf("limited result iterations = %d, want 2", got)
	}
	if got := results[1].Result.Iterations; got != 3 {
		t.Fatalf("unlimited result iterations = %d, want 3", got)
	}
	if len(events) != 5 {
		t.Fatalf("progress events = %d, want 5", len(events))
	}
	for i, event := range events {
		if event.Current != i+1 {
			t.Fatalf("progress[%d].Current = %d, want %d", i, event.Current, i+1)
		}
		if event.Total != 5 {
			t.Fatalf("progress[%d].Total = %d, want 5", i, event.Total)
		}
	}
}

func TestRunWorkloadUsesMedianPerSampleMetrics(t *testing.T) {
	workload := &sampleMetricWorkload{
		throughputs: []float64{100, 300, 200},
		latencies:   []float64{30, 10, 20},
	}
	detail, err := runWorkload(context.Background(), workload, 3, time.Second, func(int, string) {})
	if err != nil {
		t.Fatalf("runWorkload error = %v", err)
	}
	if detail.Throughput != 200 || detail.ThroughputUnit != "units/s" {
		t.Fatalf("throughput = %f %q, want 200 units/s", detail.Throughput, detail.ThroughputUnit)
	}
	if detail.AverageLatencyNS != 20 {
		t.Fatalf("latency = %f, want 20", detail.AverageLatencyNS)
	}
}

type sampleMetricWorkload struct {
	throughputs []float64
	latencies   []float64
	idx         int
	lastTP      float64
	lastLatency float64
}

func (w *sampleMetricWorkload) Name() string        { return "sample-metrics" }
func (w *sampleMetricWorkload) Category() string    { return CategoryInteger }
func (w *sampleMetricWorkload) Description() string { return "sample metrics test workload" }
func (w *sampleMetricWorkload) Validate() error     { return nil }
func (w *sampleMetricWorkload) SkipWarmup() bool    { return true }

func (w *sampleMetricWorkload) Run(context.Context) (time.Duration, int64, error) {
	if w.idx >= len(w.throughputs) {
		return time.Millisecond, 1, nil
	}
	w.lastTP = w.throughputs[w.idx]
	w.lastLatency = w.latencies[w.idx]
	w.idx++
	return time.Duration(w.idx) * time.Millisecond, int64(w.idx), nil
}

func (w *sampleMetricWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.lastTP, "units/s"
}

func (w *sampleMetricWorkload) AverageLatencyNS(int64, time.Duration) float64 {
	return w.lastLatency
}

type processedMetricWorkload struct {
	kind   ProcessedKind
	values []int64
	idx    int
}

func (w *processedMetricWorkload) Name() string        { return "processed-metric" }
func (w *processedMetricWorkload) Category() string    { return CategoryInteger }
func (w *processedMetricWorkload) Description() string { return "processed metric semantics test" }
func (w *processedMetricWorkload) Validate() error     { return nil }
func (w *processedMetricWorkload) SkipWarmup() bool    { return true }
func (w *processedMetricWorkload) ProcessedKind() ProcessedKind {
	return w.kind
}

func (w *processedMetricWorkload) Run(context.Context) (time.Duration, int64, error) {
	value := w.values[w.idx]
	w.idx++
	return time.Duration(value) * time.Millisecond, value, nil
}

func (*processedMetricWorkload) Throughput(int64, time.Duration) (float64, string) {
	return 123.5, "rate/s"
}

func TestRunWorkloadOnlyReportsExplicitProcessedMetric(t *testing.T) {
	tests := []struct {
		name      string
		kind      ProcessedKind
		wantBytes int64
		wantOps   float64
	}{
		{name: "bytes", kind: ProcessedBytes, wantBytes: 20},
		{name: "operations", kind: ProcessedOperations, wantOps: 20},
		{name: "rate or unknown", kind: ProcessedUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &processedMetricWorkload{kind: tt.kind, values: []int64{10, 30, 20}}
			detail, err := runWorkload(context.Background(), workload, 3, time.Second, func(int, string) {})
			if err != nil {
				t.Fatalf("runWorkload error = %v", err)
			}
			if detail.BytesProcessed != tt.wantBytes || detail.OpsProcessed != tt.wantOps {
				t.Fatalf("processed metrics = bytes:%d ops:%v, want bytes:%d ops:%v", detail.BytesProcessed, detail.OpsProcessed, tt.wantBytes, tt.wantOps)
			}
			if detail.Throughput != 123.5 || detail.ThroughputUnit != "rate/s" {
				t.Fatalf("throughput = %v %q, want 123.5 rate/s", detail.Throughput, detail.ThroughputUnit)
			}
		})
	}
}

type startOrderWorkload struct {
	id       string
	sequence *[]string
}

func (*startOrderWorkload) Name() string        { return "duplicate" }
func (*startOrderWorkload) Category() string    { return CategoryInteger }
func (*startOrderWorkload) Description() string { return "workload start ordering test" }
func (*startOrderWorkload) Validate() error     { return nil }
func (*startOrderWorkload) SkipWarmup() bool    { return true }

func (w *startOrderWorkload) Run(context.Context) (time.Duration, int64, error) {
	*w.sequence = append(*w.sequence, "run-"+w.id)
	return time.Millisecond, 1, nil
}

func TestRunAllEmitsStartBeforeEachDuplicateNamedWorkloadRuns(t *testing.T) {
	var sequence []string
	var starts []ProgressEvent
	workloads := []Workload{
		&startOrderWorkload{id: "1", sequence: &sequence},
		&startOrderWorkload{id: "2", sequence: &sequence},
	}

	_, warnings := RunAll(context.Background(), workloads, RunConfig{
		Iterations: 1,
		OnWorkloadStart: func(event ProgressEvent) {
			starts = append(starts, event)
			sequence = append(sequence, "start")
		},
		OnProgress: func(ProgressEvent) {
			sequence = append(sequence, "progress")
		},
		OnWorkloadDone: func(BenchResult) {
			sequence = append(sequence, "done")
		},
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	wantSequence := []string{"start", "run-1", "progress", "done", "start", "run-2", "progress", "done"}
	if fmt.Sprint(sequence) != fmt.Sprint(wantSequence) {
		t.Fatalf("sequence = %v, want %v", sequence, wantSequence)
	}
	if len(starts) != 2 {
		t.Fatalf("start events = %d, want 2", len(starts))
	}
	for i, event := range starts {
		if event.Workload != "duplicate" || event.Current != i || event.Total != 2 || event.Iteration != 0 {
			t.Fatalf("start[%d] = %+v", i, event)
		}
	}
}
