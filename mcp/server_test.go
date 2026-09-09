package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/checkup"
	gbreport "github.com/cloudapp3/vmbench/report"
)

func TestNormalizeBenchArgsRejectsInvalidValues(t *testing.T) {
	_, warnings := normalizeBenchArgs(benchArgs{
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

func TestNormalizeBenchArgsRejectsInvalidSectionValues(t *testing.T) {
	_, warnings := normalizeBenchArgs(benchArgs{
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

func TestNormalizeBenchArgsSelectsKind(t *testing.T) {
	defaultPlan, warnings := normalizeBenchArgs(benchArgs{Iterations: json.RawMessage("1")})
	if len(warnings) != 0 {
		t.Fatalf("default warnings = %v", warnings)
	}
	if defaultPlan.Kind != benchKindRun {
		t.Fatalf("default kind = %q, want %q", defaultPlan.Kind, benchKindRun)
	}
	if defaultPlan.Run.Iterations != 1 || defaultPlan.Run.Engine != "external" {
		t.Fatalf("default run options = %+v", defaultPlan.Run)
	}

	hardwareOnly, _ := normalizeBenchArgs(benchArgs{Only: []string{"hardware"}})
	if hardwareOnly.Kind != benchKindRun {
		t.Fatalf("only=hardware kind = %q, want %q", hardwareOnly.Kind, benchKindRun)
	}

	presetPlan, _ := normalizeBenchArgs(benchArgs{Preset: "quick"})
	if presetPlan.Kind != benchKindCheckup {
		t.Fatalf("preset=quick kind = %q, want %q", presetPlan.Kind, benchKindCheckup)
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

	checkupReport := checkup.CheckupReport{
		Speed: checkup.SpeedSection{SectionState: checkup.SectionState{Enabled: true, Status: "partial"}},
	}
	checkupResult := okToolResult(formatCheckupSummary(checkupReport), map[string]any{"report": checkupReport})
	checkupResult.IsError = checkupReport.HasFailures()
	if !checkupResult.IsError || checkupResult.StructuredContent == nil {
		t.Fatalf("checkup result = %+v, want structured error", checkupResult)
	}
}

func TestFormatRunSummaryCountsMissingResultAsFailure(t *testing.T) {
	doc := gbreport.Document{Results: gbreport.ResultsSection{Workloads: []gbreport.WorkloadEntry{{Name: "missing"}}}}
	if summary := formatRunSummary(doc); !strings.Contains(summary, "status=failed") || !strings.Contains(summary, "failed=1") {
		t.Fatalf("summary = %q, want failed status and failed=1", summary)
	}
}

func TestNormalizeArgsRejectExplicitNullNumbers(t *testing.T) {
	_, warnings := normalizeBenchArgs(benchArgs{
		Iterations: json.RawMessage("null"),
		TimeoutMS:  json.RawMessage("null"),
	})
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "iterations must be an integer") || !strings.Contains(joined, "timeout_ms must be an integer") {
		t.Fatalf("warnings = %v, want null numeric validation errors", warnings)
	}
}

func TestNormalizeBenchArgsUsesCanonicalSectionsAndCatalog(t *testing.T) {
	identityOnly, warnings := normalizeBenchArgs(benchArgs{
		Iterations: json.RawMessage("1"),
		Only:       []string{"identity", "telegram"},
		IPVersion:  "dual",
	})
	if len(warnings) != 0 {
		t.Fatalf("identity warnings = %v", warnings)
	}
	if identityOnly.Kind != benchKindCheckup {
		t.Fatalf("identity kind = %q, want %q", identityOnly.Kind, benchKindCheckup)
	}
	if !identityOnly.Checkup.Sections.NetworkInfo || !identityOnly.Checkup.Sections.Reachability || identityOnly.Checkup.Sections.Speed {
		t.Fatalf("identity sections = %+v", identityOnly.Checkup.Sections)
	}
	if identityOnly.Checkup.CatalogRevision != "" || identityOnly.Checkup.ResolvedCatalog != nil {
		t.Fatalf("non-node checkup retained catalog: %+v", identityOnly.Checkup)
	}

	pingOnly, warnings := normalizeBenchArgs(benchArgs{
		Iterations:   json.RawMessage("1"),
		Only:         []string{"ping"},
		RoutePresets: []string{"cd", "cernet", "cstnet"},
		IPVersion:    "dual",
	})
	if len(warnings) != 0 {
		t.Fatalf("ping warnings = %v", warnings)
	}
	if pingOnly.Checkup.ResolvedCatalog == nil || pingOnly.Checkup.CatalogRevision == "" || len(pingOnly.Checkup.NodeIDs) == 0 {
		t.Fatalf("ping catalog provenance = %+v", pingOnly.Checkup)
	}
}

func TestNormalizeBenchArgsRejectCatalogRevisionMismatch(t *testing.T) {
	_, warnings := normalizeBenchArgs(benchArgs{
		Iterations:      json.RawMessage("1"),
		Only:            []string{"ping"},
		CatalogRevision: "missing-revision",
	})
	if !strings.Contains(strings.Join(warnings, "\n"), "pinned revision") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestCallToolRoutesBenchTool(t *testing.T) {
	s := &Server{}
	for _, name := range []string{"vmbench_run"} {
		res, err := s.callTool(context.Background(), name, json.RawMessage(`{"iterations":0}`))
		if err != nil {
			t.Fatalf("callTool(%s) error = %v", name, err)
		}
		if !res.IsError || !strings.Contains(res.Content[0].Text, "iterations") {
			t.Fatalf("callTool(%s) = %+v, want iterations validation error", name, res)
		}
	}
}

func TestToolSpecsExposeMergedSchema(t *testing.T) {
	schemas := map[string]map[string]any{}
	for _, spec := range toolSpecs() {
		schemas[spec.Name] = spec.InputSchema
	}
	runSchema, ok := schemas["vmbench_run"]
	if !ok {
		t.Fatal("vmbench_run spec missing")
	}

	properties, _ := runSchema["properties"].(map[string]any)
	for _, key := range []string{"catalog_source", "catalog_revision", "catalog_cache_path", "preset", "only", "skip"} {
		if _, ok := properties[key]; !ok {
			t.Fatalf("vmbench_run schema missing %q: %s", key, formatProperties(properties))
		}
	}
	only, _ := properties["only"].(map[string]any)
	items, _ := only["items"].(map[string]any)
	enum, _ := items["enum"].([]string)
	joined := strings.Join(enum, ",")
	for _, section := range []string{"network_info", "reachability"} {
		if !strings.Contains(joined, section) {
			t.Fatalf("section enum = %v, want %q", enum, section)
		}
	}
	payload := capabilitiesPayload()
	if _, ok := payload["node_catalog"]; !ok {
		t.Fatalf("capabilities missing node_catalog: %#v", payload)
	}
}

func formatProperties(properties map[string]any) string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	return fmt.Sprintf("properties %v", keys)
}
