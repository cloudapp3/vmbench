package tui

import (
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/tui/comp"
)

func TestNewCheckupSectionsIncludesNetworkEvidence(t *testing.T) {
	sections := newCheckupSections(checkup.SectionSelector{NetworkInfo: true, Reachability: true})
	if len(sections) != 2 || sections[0].id != checkup.SectionNetworkInfo || sections[1].id != checkup.SectionReachability {
		t.Fatalf("newCheckupSections() = %+v", sections)
	}
}

func TestStartCheckupLandsOnCheckupRunningPage(t *testing.T) {
	m := NewModel("", "")
	norm, err := checkup.NormalizeOptions(checkup.Options{Sections: checkup.SectionSelector{Hardware: true, Speed: true}})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := startCheckup(m, norm)
	um := updated.(Model)
	if um.page != pageRunning || um.runKind != "checkup" {
		t.Fatalf("startCheckup page=%d runKind=%q, want pageRunning/checkup", um.page, um.runKind)
	}
}

func TestUpdateCheckupEventPreservesPartialStatus(t *testing.T) {
	m := NewModel("", "")
	m.checkupSections = []checkupSection{{
		id:     checkup.SectionRoute,
		label:  "Route",
		status: "running",
	}}

	updatedModel, _ := updateCheckupEvent(m, checkup.Event{
		Kind:    checkup.EventSectionFail,
		Section: checkup.SectionRoute,
		Status:  "partial",
		Message: "2/3 destinations reached",
	})
	updated := updatedModel.(Model)

	if got := updated.checkupSections[0].status; got != "partial" {
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
	updated.runKind = "checkup"
	if got := viewRunning(updated); !strings.Contains(got, "1/1") || !strings.Contains(got, "✗1") {
		t.Fatalf("running view did not count partial as a terminal non-ok section:\n%s", got)
	}
}

func TestUpdateCheckupEventKeepsEnabledSkippedAsTerminalFailure(t *testing.T) {
	m := NewModel("", "")
	m.checkupSections = []checkupSection{{
		id:     checkup.SectionSpeed,
		label:  "Speed",
		status: "running",
	}}

	updatedModel, _ := updateCheckupEvent(m, checkup.Event{
		Kind:    checkup.EventSectionFail,
		Section: checkup.SectionSpeed,
		Status:  "skipped",
		Message: "no providers selected",
	})
	updated := updatedModel.(Model)

	if got := updated.checkupSections[0].status; got != "skipped" {
		t.Fatalf("section status = %q, want skipped", got)
	}
	if len(updated.eventLog) != 1 || !strings.Contains(updated.eventLog[0], "skipped   speed") {
		t.Fatalf("event log = %q, want skipped terminal status", updated.eventLog)
	}
	if got := checkupSectionStatus(updated, updated.checkupSections[0], 80); got != comp.StatusPill(comp.StatusSkip, "skipped") {
		t.Fatalf("checkupSectionStatus() = %q, want skipped pill", got)
	}
}

func TestCheckupSectionStatusRendersPartialPill(t *testing.T) {
	m := NewModel("", "")
	section := checkupSection{status: "partial", message: "2/3 destinations reached"}

	got := checkupSectionStatus(m, section, 80)
	want := comp.StatusPill(comp.StatusPartial, section.message)
	if got != want {
		t.Fatalf("checkupSectionStatus() = %q, want %q", got, want)
	}
}
