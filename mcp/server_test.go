package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
)

func TestNormalizeRunArgsRejectsInvalidValues(t *testing.T) {
	_, warnings := normalizeRunArgs(runArgs{
		Iterations:    json.RawMessage("0"),
		TimeoutMS:     json.RawMessage("-1"),
		Filter:        "[",
		HardwareTools: []string{"openssl", "unknown"},
	})
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"iterations", "timeout_ms", "filter regex", "unknown hardware tool"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings = %v, want %q", warnings, want)
		}
	}
}

func TestNormalizeSuiteArgsRejectsInvalidValues(t *testing.T) {
	_, warnings := normalizeSuiteArgs(suiteArgs{
		Iterations:     json.RawMessage("1"),
		TimeoutMS:      json.RawMessage("1000"),
		Filter:         "(",
		Only:           []string{"hardware"},
		SpeedProviders: []string{"cloudflare", "unknown"},
		IPVersion:      "v5",
	})
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"filter regex", "unknown speed provider", "ip_version"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings = %v, want %q", warnings, want)
		}
	}
}

func TestFailedReportsRemainStructuredToolErrors(t *testing.T) {
	runReport := gbreport.Document{}
	runResult := okToolResult(formatRunSummary(runReport), map[string]any{"report": runReport})
	runResult.IsError = gbreport.HasFailures(runReport)
	if !runResult.IsError || runResult.StructuredContent == nil {
		t.Fatalf("run result = %+v, want structured error", runResult)
	}
	if !strings.Contains(runResult.Content[0].Text, "status=failed") {
		t.Fatalf("run summary = %q, want failed status", runResult.Content[0].Text)
	}

	suiteReport := suite.SuiteReport{
		Speed: suite.SpeedSection{SectionState: suite.SectionState{Enabled: true, Status: "partial"}},
	}
	suiteResult := okToolResult(formatSuiteSummary(suiteReport), map[string]any{"report": suiteReport})
	suiteResult.IsError = suiteReport.HasFailures()
	if !suiteResult.IsError || suiteResult.StructuredContent == nil {
		t.Fatalf("suite result = %+v, want structured error", suiteResult)
	}
}

func TestFormatRunSummaryCountsMissingResultAsFailure(t *testing.T) {
	doc := gbreport.Document{Results: gbreport.ResultsSection{Workloads: []gbreport.WorkloadEntry{{Name: "missing"}}}}
	if summary := formatRunSummary(doc); !strings.Contains(summary, "status=failed") || !strings.Contains(summary, "failed=1") {
		t.Fatalf("summary = %q, want failed status and failed=1", summary)
	}
}

func TestNormalizeArgsRejectExplicitNullNumbers(t *testing.T) {
	_, runWarnings := normalizeRunArgs(runArgs{
		Iterations: json.RawMessage("null"),
		TimeoutMS:  json.RawMessage("null"),
	})
	joined := strings.Join(runWarnings, "\n")
	if !strings.Contains(joined, "iterations must be an integer") || !strings.Contains(joined, "timeout_ms must be an integer") {
		t.Fatalf("run warnings = %v, want null numeric validation errors", runWarnings)
	}

	_, suiteWarnings := normalizeSuiteArgs(suiteArgs{Iterations: json.RawMessage("null"), TimeoutMS: json.RawMessage("null")})
	joined = strings.Join(suiteWarnings, "\n")
	if !strings.Contains(joined, "iterations must be an integer") || !strings.Contains(joined, "timeout_ms must be an integer") {
		t.Fatalf("suite warnings = %v, want null numeric validation errors", suiteWarnings)
	}
}

func TestNormalizeSuiteArgsUsesCanonicalSectionsAndCatalog(t *testing.T) {
	identityOnly, warnings := normalizeSuiteArgs(suiteArgs{
		Iterations: json.RawMessage("1"),
		Only:       []string{"identity", "telegram"},
		IPVersion:  "dual",
	})
	if len(warnings) != 0 {
		t.Fatalf("identity warnings = %v", warnings)
	}
	if !identityOnly.Sections.NetworkInfo || !identityOnly.Sections.Reachability || identityOnly.Sections.Speed {
		t.Fatalf("identity sections = %+v", identityOnly.Sections)
	}
	if identityOnly.CatalogRevision != "" || identityOnly.ResolvedCatalog != nil {
		t.Fatalf("non-node suite retained catalog: %+v", identityOnly)
	}

	pingOnly, warnings := normalizeSuiteArgs(suiteArgs{
		Iterations:   json.RawMessage("1"),
		Only:         []string{"ping"},
		RoutePresets: []string{"cd", "cernet", "cstnet"},
		IPVersion:    "dual",
	})
	if len(warnings) != 0 {
		t.Fatalf("ping warnings = %v", warnings)
	}
	if pingOnly.ResolvedCatalog == nil || pingOnly.CatalogRevision == "" || len(pingOnly.NodeIDs) == 0 {
		t.Fatalf("ping catalog provenance = %+v", pingOnly)
	}
}

func TestNormalizeArgsRejectCatalogRevisionMismatch(t *testing.T) {
	_, runWarnings := normalizeRunArgs(runArgs{
		Iterations:      json.RawMessage("1"),
		Scope:           "network",
		CatalogRevision: "missing-revision",
	})
	if !strings.Contains(strings.Join(runWarnings, "\n"), "pinned revision") {
		t.Fatalf("run warnings = %v", runWarnings)
	}
	_, suiteWarnings := normalizeSuiteArgs(suiteArgs{
		Iterations:      json.RawMessage("1"),
		Only:            []string{"ping"},
		CatalogRevision: "missing-revision",
	})
	if !strings.Contains(strings.Join(suiteWarnings, "\n"), "pinned revision") {
		t.Fatalf("suite warnings = %v", suiteWarnings)
	}
}

func TestToolSpecsExposeCatalogAndNetworkEvidence(t *testing.T) {
	var suiteSchema map[string]any
	for _, spec := range toolSpecs() {
		if spec.Name == "vmbench_suite" {
			suiteSchema = spec.InputSchema
			break
		}
	}
	properties, _ := suiteSchema["properties"].(map[string]any)
	for _, key := range []string{"catalog_source", "catalog_revision", "catalog_cache_path"} {
		if _, ok := properties[key]; !ok {
			t.Fatalf("suite MCP schema missing %q: %#v", key, properties)
		}
	}
	only, _ := properties["only"].(map[string]any)
	items, _ := only["items"].(map[string]any)
	enum, _ := items["enum"].([]string)
	joined := strings.Join(enum, ",")
	for _, section := range []string{"network_info", "reachability"} {
		if !strings.Contains(joined, section) {
			t.Fatalf("suite MCP section enum = %v, want %q", enum, section)
		}
	}
	payload := capabilitiesPayload()
	if _, ok := payload["node_catalog"]; !ok {
		t.Fatalf("capabilities missing node_catalog: %#v", payload)
	}
}
