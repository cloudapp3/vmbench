package suite

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunIperfWithoutHostFailsConsistently(t *testing.T) {
	events := make([]Event, 0, 4)
	report := Run(context.Background(), Options{
		Sections:       SectionSelector{Speed: true},
		SpeedProviders: []string{SpeedProviderIperf3},
		Timeout:        time.Second,
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	if report.Speed.Status != "error" {
		t.Fatalf("Speed.Status = %q, want error", report.Speed.Status)
	}
	if report.Status != "failed" {
		t.Fatalf("Status = %q, want failed", report.Status)
	}
	if !report.HasFailures() {
		t.Fatal("HasFailures() = false, want true")
	}
	if report.Speed.Result == nil || len(report.Speed.Result.Groups) != 1 || report.Speed.Result.Groups[0].Status != "error" {
		t.Fatalf("Speed.Result = %+v, want one failed iperf3 group", report.Speed.Result)
	}

	relevantEvents := make([]Event, 0, 3)
	for _, event := range events {
		if event.Section == SectionSpeed || event.Kind == EventSuiteDone {
			relevantEvents = append(relevantEvents, event)
		}
	}
	wantEvents := []struct {
		kind   EventKind
		status string
	}{
		{kind: EventSectionStart, status: "running"},
		{kind: EventSectionFail, status: "error"},
		{kind: EventSuiteDone, status: "failed"},
	}
	if len(relevantEvents) != len(wantEvents) {
		t.Fatalf("relevant events = %+v, want %d events", relevantEvents, len(wantEvents))
	}
	for i, want := range wantEvents {
		if relevantEvents[i].Kind != want.kind || relevantEvents[i].Status != want.status {
			t.Fatalf("relevant events[%d] = %+v, want kind=%q status=%q", i, relevantEvents[i], want.kind, want.status)
		}
	}
}

func TestRunAcceptsNilParentContext(t *testing.T) {
	report := Run(nil, Options{
		Sections:       SectionSelector{Speed: true},
		SpeedProviders: []string{SpeedProviderIperf3},
		Timeout:        time.Second,
	})
	if report.Speed.Status != "error" || report.Status != "failed" {
		t.Fatalf("Run(nil) status = suite %q, speed %q; want failed/error", report.Status, report.Speed.Status)
	}
}

func TestRunSectionWithContextAppliesNetworkTimeout(t *testing.T) {
	state := SectionState{Enabled: true}
	start := time.Now()
	runSectionWithContext(context.Background(), 10*time.Millisecond, true, func(ctx context.Context) {
		<-ctx.Done()
		state.Status = "ok"
		state.Message = "probe returned a result"
	}, &state)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("section returned after %s, want timeout near 10ms", elapsed)
	}
	if state.Status != "error" {
		t.Fatalf("Status = %q, want error", state.Status)
	}
	if !strings.Contains(state.Message, "timed out") || !strings.Contains(state.Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("Message = %q, want structured timeout error", state.Message)
	}
}

func TestRunSectionWithContextLeavesHardwareWithoutSectionDeadline(t *testing.T) {
	state := SectionState{Enabled: true}
	hadDeadline := true
	runSectionWithContext(context.Background(), time.Nanosecond, false, func(ctx context.Context) {
		_, hadDeadline = ctx.Deadline()
		state.Status = "ok"
	}, &state)

	if hadDeadline {
		t.Fatal("hardware context has a section deadline, want workload-level timeout only")
	}
	if state.Status != "ok" {
		t.Fatalf("Status = %q, want ok", state.Status)
	}
}

func TestSectionUsesTimeoutPolicy(t *testing.T) {
	tests := []struct {
		section SectionID
		want    bool
	}{
		{section: SectionHardware},
		{section: SectionNetworkInfo, want: true},
		{section: SectionRoute, want: true},
		{section: SectionPing, want: true},
		{section: SectionSpeed, want: true},
		{section: SectionIPQuality, want: true},
		{section: SectionReachability, want: true},
		{section: SectionMail, want: true},
		{section: SectionMedia, want: true},
	}
	for _, tt := range tests {
		if got := sectionUsesTimeout(tt.section); got != tt.want {
			t.Errorf("sectionUsesTimeout(%q) = %t, want %t", tt.section, got, tt.want)
		}
	}
}

func TestFinalizeTreatsEnabledSkippedSectionAsFailure(t *testing.T) {
	report := SuiteReport{
		Speed: SpeedSection{SectionState: SectionState{Enabled: true, Status: "skipped"}},
	}
	status, message := finalize(report)
	if status != "failed" || !strings.Contains(message, "speed") {
		t.Fatalf("finalize() = %q, %q; want failed speed section", status, message)
	}
}
