package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/suite"
)

func runningTestModel(t *testing.T) Model {
	t.Helper()
	m := scrollTestModel(t, pageRunning, nil)
	m.workloads = []workloadState{
		{name: "CPU Single-Core (sysbench)", category: "CPU", status: "waiting"},
		{name: "CPU Multi-Core (sysbench)", category: "CPU", status: "waiting"},
		{name: "Disk 4K Random Read Q1 (fio)", category: "Disk", status: "waiting"},
	}
	m.phase = "Hardware"
	m.engine = "external"
	m.startedAt = time.Now().Add(-30 * time.Second)
	m.eventCh = make(chan vmbench.Event, 10)
	return m
}

func TestRunETARequiresCompletion(t *testing.T) {
	m := runningTestModel(t)

	if _, ok := runETA(m); ok {
		t.Fatal("ETA must be unavailable before the first completion")
	}

	feed := func(ev vmbench.Event) Model {
		updated, _ := m.Update(benchmarkEventMsg{event: ev})
		m = updated.(Model)
		return m
	}

	// First workload starts, runs one iteration, completes.
	feed(vmbench.Event{Kind: vmbench.EventSuiteStart, Workload: "CPU Single-Core (sysbench)"})
	feed(vmbench.Event{Kind: vmbench.EventSuiteProgress, Workload: "CPU Single-Core (sysbench)", Iteration: 1, Current: 1, Total: 9})
	feed(vmbench.Event{Kind: vmbench.EventSuiteDone, Workload: "CPU Single-Core (sysbench)", Metric: "532 events/sec"})

	if _, ok := runETA(m); !ok {
		t.Fatal("ETA should be available after one completion with workloads remaining")
	}

	view := m.View()
	if !strings.Contains(view, i18n.Tf("tui.running.eta", map[string]any{"Eta": func() string {
		eta, _ := runETA(m)
		return eta.String()
	}()})) {
		t.Fatalf("running view should show the ETA line:\n%s", view)
	}
	if !strings.Contains(view, "1/9") {
		t.Fatalf("running view should show sample counters, got:\n%s", view)
	}
}

func TestRunETADisappearsWhenNothingRemains(t *testing.T) {
	m := runningTestModel(t)
	for i := range m.workloads {
		m.workloads[i].status = "done"
	}
	m.workloadDoneAt = []time.Duration{10 * time.Second}
	if _, ok := runETA(m); ok {
		t.Fatal("ETA should hide when no workloads remain")
	}
}

func TestRunningIterationMiniBarAndElapsed(t *testing.T) {
	m := runningTestModel(t)
	updated, _ := m.Update(benchmarkEventMsg{event: vmbench.Event{
		Kind: vmbench.EventSuiteStart, Workload: "CPU Single-Core (sysbench)",
	}})
	m = updated.(Model)
	updated, _ = m.Update(benchmarkEventMsg{event: vmbench.Event{
		Kind: vmbench.EventSuiteProgress, Workload: "CPU Single-Core (sysbench)",
		Iteration: 2, Current: 2, Total: 9,
	}})
	m = updated.(Model)

	if m.workloads[0].iterCur != 2 || m.workloads[0].iterTotal != 2 {
		t.Fatalf("iteration tracking = %d/%d, want 2/2", m.workloads[0].iterCur, m.workloads[0].iterTotal)
	}
	if m.runSamplesDone != 2 || m.runSamplesTotal != 9 {
		t.Fatalf("sample counters = %d/%d, want 2/9", m.runSamplesDone, m.runSamplesTotal)
	}

	view := m.View()
	if !strings.Contains(view, "2/2") {
		t.Fatalf("current workload mini bar missing:\n%s", view)
	}
}

func TestRunningWorstCaseBounds(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := runningTestModel(t)
		m.width = width
		m.workloads = append(m.workloads,
			workloadState{name: "Memory Read Bandwidth (sysbench)", category: "Memory", status: "done", metric: "4.1 GB/s", duration: "3s"},
			workloadState{name: "Memory Write Bandwidth (sysbench)", category: "Memory", status: "fail", metric: "sysbench: not found"},
			workloadState{name: "Disk 4K Random Write Q1 (fio)", category: "Disk", status: "skip"},
		)
		m.workloads[0].status = "running"
		m.workloads[0].startedAt = time.Now().Add(-5 * time.Second)
		m.workloads[0].iterCur, m.workloads[0].iterTotal = 2, 3
		m.workloadDoneAt = []time.Duration{12 * time.Second}
		m.runSamplesDone, m.runSamplesTotal = 5, 21
		m.showLog = true
		m.eventLog = []string{"12:00:00 ▸ start  CPU Single-Core", "12:00:03 ✓ done  CPU Single-Core  532 events/sec"}

		assertRenderBounds(t, m.View(), width, 24)

		m.confirm = true
		assertRenderBounds(t, m.View(), width, 24)
	}
}

func TestSuiteSectionElapsed(t *testing.T) {
	m := scrollTestModel(t, pageSuiteRunning, nil)
	m.suiteSections = []suiteSection{
		{id: suite.SectionRoute, label: "Route", status: "running", startedAt: time.Now().Add(-4 * time.Second)},
		{id: suite.SectionPing, label: "Ping", status: "waiting"},
	}
	m.startedAt = time.Now().Add(-10 * time.Second)

	view := m.View()
	if !strings.Contains(view, "4s") {
		t.Fatalf("running section should show elapsed time:\n%s", view)
	}

	updated, _ := m.Update(suiteEventMsg{event: suite.Event{
		Kind: suite.EventSectionDone, Section: suite.SectionRoute, Message: "ok",
	}})
	um := updated.(Model)
	if !um.suiteSections[0].startedAt.IsZero() {
		t.Fatal("section start timer should clear on done")
	}
}
