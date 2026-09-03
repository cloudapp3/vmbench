package tui

import (
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/tui/comp"
)

func TestUpdateSuiteEventPreservesPartialStatus(t *testing.T) {
	m := NewModel("", "")
	m.suiteSections = []suiteSection{{
		id:     suite.SectionRoute,
		label:  "Route",
		status: "running",
	}}

	updatedModel, _ := updateSuiteEvent(m, suite.Event{
		Kind:    suite.EventSectionFail,
		Section: suite.SectionRoute,
		Status:  "partial",
		Message: "2/3 destinations reached",
	})
	updated := updatedModel.(Model)

	if got := updated.suiteSections[0].status; got != "partial" {
		t.Fatalf("section status = %q, want partial", got)
	}
	if len(updated.eventLog) != 1 {
		t.Fatalf("event log length = %d, want 1", len(updated.eventLog))
	}
	if got := updated.eventLog[0]; !strings.Contains(got, "partial   route") || strings.Contains(got, "fail") {
		t.Fatalf("event log = %q, want partial status without fail", got)
	}
	updated.width = 80
	updated.height = 24
	if got := viewSuiteRunning(updated); !strings.Contains(got, "1/1") || !strings.Contains(got, "✗1") {
		t.Fatalf("running view did not count partial as a terminal non-ok section:\n%s", got)
	}
}

func TestUpdateSuiteEventKeepsEnabledSkippedAsTerminalFailure(t *testing.T) {
	m := NewModel("", "")
	m.suiteSections = []suiteSection{{
		id:     suite.SectionSpeed,
		label:  "Speed",
		status: "running",
	}}

	updatedModel, _ := updateSuiteEvent(m, suite.Event{
		Kind:    suite.EventSectionFail,
		Section: suite.SectionSpeed,
		Status:  "skipped",
		Message: "no providers selected",
	})
	updated := updatedModel.(Model)

	if got := updated.suiteSections[0].status; got != "skipped" {
		t.Fatalf("section status = %q, want skipped", got)
	}
	if len(updated.eventLog) != 1 || !strings.Contains(updated.eventLog[0], "skipped   speed") {
		t.Fatalf("event log = %q, want skipped terminal status", updated.eventLog)
	}
	if got := suiteSectionStatus(updated, updated.suiteSections[0], 80); got != comp.StatusPill(comp.StatusSkip, "skipped") {
		t.Fatalf("suiteSectionStatus() = %q, want skipped pill", got)
	}
}

func TestSuiteSectionStatusRendersPartialPill(t *testing.T) {
	m := NewModel("", "")
	section := suiteSection{status: "partial", message: "2/3 destinations reached"}

	got := suiteSectionStatus(m, section, 80)
	want := comp.StatusPill(comp.StatusPartial, section.message)
	if got != want {
		t.Fatalf("suiteSectionStatus() = %q, want %q", got, want)
	}
}
