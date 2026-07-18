package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/nodecatalog"
)

func TestApplySectionNamesUsesSharedAliases(t *testing.T) {
	sections, err := ApplySectionNames(SectionSelector{}, []string{"hw", "identity", "website", "telegram"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !sections.Hardware || !sections.NetworkInfo || !sections.Reachability {
		t.Fatalf("sections = %+v", sections)
	}
	if sections.Speed {
		t.Fatalf("website alias unexpectedly enabled speed: %+v", sections)
	}
}

func TestApplySectionNamesRejectsUnknownName(t *testing.T) {
	if _, err := ApplySectionNames(SectionSelector{}, []string{"hardware", "unknown"}, true); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("ApplySectionNames() error = %v", err)
	}
}

func TestNormalizeOptionsUsesCanonicalSuiteDefaults(t *testing.T) {
	norm, err := NormalizeOptions(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if norm.Iterations != 3 || norm.Timeout != 5*time.Minute || norm.IPVersion != "v4" || !norm.Sections.AnyEnabled() {
		t.Fatalf("NormalizeOptions() = %+v", norm)
	}
}

func TestValidateOptionsRejectsInvalidSuiteConfiguration(t *testing.T) {
	err := ValidateOptions(Options{
		Iterations:     10,
		Timeout:        -time.Second,
		Filter:         "[",
		Preset:         "unknown",
		IPVersion:      "v5",
		RoutePresets:   []string{"unknown"},
		SpeedProviders: []string{"unknown"},
		HardwareTools:  []string{"unknown"},
	})
	if err == nil {
		t.Fatal("ValidateOptions() error = nil")
	}
	for _, want := range []string{"iterations", "timeout", "filter regex", "preset", "ip_version", "route preset", "speed provider", "hardware tool"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateOptions() error = %q, want %q", err, want)
		}
	}
}

func TestNormalizeOptionsResolvesCatalogAndSelectedNodes(t *testing.T) {
	norm, err := NormalizeOptions(Options{
		Sections:      SectionSelector{Route: true, Ping: true},
		RoutePresets:  []string{"cd", "cernet", "cstnet"},
		IPVersion:     "dual",
		CatalogSource: nodecatalog.SourceEmbedded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if norm.ResolvedCatalog == nil || norm.CatalogRevision == "" || norm.CatalogSource != nodecatalog.SourceEmbedded {
		t.Fatalf("catalog provenance = source %q revision %q manifest %v", norm.CatalogSource, norm.CatalogRevision, norm.ResolvedCatalog)
	}
	if len(norm.NodeIDs) == 0 {
		t.Fatal("NodeIDs is empty")
	}
	report := NewSuiteReport(norm)
	if report.Config.CatalogRevision != norm.CatalogRevision || len(report.Config.NodeIDs) != len(norm.NodeIDs) {
		t.Fatalf("report config = %+v, normalized = %+v", report.Config, norm)
	}
}

func TestNormalizeOptionsDropsCatalogForNonNodeSuite(t *testing.T) {
	norm, err := NormalizeOptions(Options{
		Sections:        SectionSelector{Hardware: true},
		CatalogSource:   nodecatalog.SourceAuto,
		CatalogRevision: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	if norm.CatalogSource != "" || norm.CatalogRevision != "" || norm.ResolvedCatalog != nil || len(norm.NodeIDs) != 0 {
		t.Fatalf("unused catalog configuration retained: %+v", norm)
	}
}

func TestRunRejectsCatalogRevisionBeforeSectionsStart(t *testing.T) {
	events := make([]Event, 0)
	report := Run(context.Background(), Options{
		Sections:        SectionSelector{Ping: true},
		CatalogRevision: "not-the-embedded-revision",
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})
	if report.Status != "failed" || report.Ping.Status != "error" || len(report.Ping.Results) != 0 {
		t.Fatalf("report = %+v", report)
	}
	for _, event := range events {
		if event.Kind == EventSectionStart {
			t.Fatalf("probe section started before revision validation: %+v", event)
		}
	}
}
