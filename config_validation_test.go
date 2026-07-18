package vmbench

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/nodecatalog"
)

func TestNormalizeOptionsUsesCanonicalDefaults(t *testing.T) {
	norm, err := NormalizeOptions(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if norm.Iterations != 3 || norm.Timeout != 5*time.Minute || norm.Scope != ScopeHardware || norm.Mode != "single" {
		t.Fatalf("NormalizeOptions() = %+v, want canonical defaults", norm)
	}
}

func TestValidateOptionsRejectsInvalidRunConfiguration(t *testing.T) {
	err := ValidateOptions(Options{
		Iterations:    10,
		Timeout:       -time.Second,
		Filter:        "[",
		Mode:          "parallel",
		Engine:        "unknown",
		Scope:         "internet",
		HardwareTools: []string{"openssl", "unknown"},
	})
	if err == nil {
		t.Fatal("ValidateOptions() error = nil")
	}
	for _, want := range []string{"iterations", "timeout", "filter regex", "mode", "engine", "scope", "unknown hardware tool"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateOptions() error = %q, want %q", err, want)
		}
	}
}

func TestNormalizeOptionsDropsUnusedNetworkConfiguration(t *testing.T) {
	norm, err := NormalizeOptions(Options{
		Scope:           ScopeHardware,
		IperfHosts:      []string{"one.example:5201"},
		CatalogSource:   nodecatalog.SourceAuto,
		CatalogRevision: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(norm.IperfHosts) != 0 {
		t.Fatalf("IperfHosts = %v, want none for hardware scope", norm.IperfHosts)
	}
	if norm.CatalogSource != "" || norm.CatalogRevision != "" || norm.ResolvedCatalog != nil {
		t.Fatalf("unused catalog configuration was retained: %+v", norm)
	}
}

func TestNormalizeOptionsPinsEmbeddedNetworkCatalog(t *testing.T) {
	norm, err := NormalizeOptions(Options{Scope: ScopeNetwork, CatalogSource: nodecatalog.SourceEmbedded})
	if err != nil {
		t.Fatal(err)
	}
	if norm.ResolvedCatalog == nil || norm.CatalogSource != nodecatalog.SourceEmbedded || norm.CatalogRevision == "" {
		t.Fatalf("catalog provenance = source %q revision %q manifest %v", norm.CatalogSource, norm.CatalogRevision, norm.ResolvedCatalog)
	}
	if _, err := NormalizeOptions(Options{Scope: ScopeNetwork, CatalogRevision: "missing-revision"}); err == nil || !strings.Contains(err.Error(), "pinned revision") {
		t.Fatalf("revision mismatch error = %v", err)
	}
}
