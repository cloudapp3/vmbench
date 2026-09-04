package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/cloudapp3/vmbench/suite"
)

func TestNewSuiteConfigStateUsesQuickDefaults(t *testing.T) {
	state := newSuiteConfigState()
	quick, ok := suite.LookupPreset("quick")
	if !ok {
		t.Fatal("quick preset not found")
	}

	if got := state.presetIDs[state.preset]; got != "quick" {
		t.Fatalf("selected preset = %q, want quick", got)
	}
	if state.sections != quick.Sections {
		t.Fatalf("sections = %+v, want quick sections %+v", state.sections, quick.Sections)
	}
	if !state.speedProviders[suite.SpeedProviderCloudflare] {
		t.Fatal("Cloudflare should be selected by default")
	}
	if state.speedProviders[suite.SpeedProviderIperf3] {
		t.Fatal("iperf3 should not be selected without a host")
	}
	if !slices.Contains(state.sectionKeys, "NetworkInfo") || !slices.Contains(state.sectionKeys, "Reachability") {
		t.Fatalf("section keys = %v, want network evidence sections", state.sectionKeys)
	}
	for _, id := range []string{"cd", "cernet", "cstnet"} {
		if !slices.Contains(state.routeIDs, id) {
			t.Fatalf("route IDs = %v, want %q", state.routeIDs, id)
		}
	}

	opts := state.buildOptions("")
	if len(opts.IperfHosts) != 0 {
		t.Fatalf("IperfHosts = %v, want none", opts.IperfHosts)
	}
	for _, provider := range opts.SpeedProviders {
		if provider == suite.SpeedProviderIperf3 {
			t.Fatalf("SpeedProviders = %v, want iperf3 omitted", opts.SpeedProviders)
		}
	}
}

func TestSuiteConfigBuildsCanonicalOptions(t *testing.T) {
	state := newSuiteConfigState()
	state.preset = 0
	state.sections = suite.SectionSelector{Hardware: true, Ping: true, Reachability: true}
	state.iterations = 5
	state.ipVersion = "dual"
	state.timeoutIndex = 2
	state.iperfHost = "iperf.example:5201"
	state.catalogSource = nodecatalog.SourceEmbedded
	state.catalogRevision = ""
	for id := range state.hardwareTools {
		state.hardwareTools[id] = false
	}
	state.hardwareTools[catalog.HardwareToolOpenSSL] = true

	raw := state.buildOptions("")
	if raw.Iterations != 5 || raw.IPVersion != "dual" || raw.Timeout != 10*time.Minute {
		t.Fatalf("runtime options = %+v", raw)
	}
	if !slices.Equal(raw.HardwareTools, []string{catalog.HardwareToolOpenSSL}) || !slices.Equal(raw.IperfHosts, []string{"iperf.example:5201"}) {
		t.Fatalf("tool options = %+v", raw)
	}
	norm, err := suite.NormalizeOptions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if norm.CatalogRevision == "" || norm.ResolvedCatalog == nil || len(norm.NodeIDs) == 0 {
		t.Fatalf("normalized TUI provenance = %+v", norm)
	}
}

func TestNewSuiteSectionsIncludesNetworkEvidence(t *testing.T) {
	sections := newSuiteSections(suite.SectionSelector{NetworkInfo: true, Reachability: true})
	if len(sections) != 2 || sections[0].id != suite.SectionNetworkInfo || sections[1].id != suite.SectionReachability {
		t.Fatalf("newSuiteSections() = %+v", sections)
	}
}

func TestNewSuiteConfigStateDefaultsForNewFields(t *testing.T) {
	state := newSuiteConfigState()
	if !state.mediaSets[suite.DefaultMediaSet()] {
		t.Errorf("default media set %s should be selected", suite.DefaultMediaSet())
	}
	if !state.ipSources[suite.IPSourceBuiltin] {
		t.Error("builtin IP source should be selected by default")
	}
	if state.ipSources[suite.IPSourceSecurityCheck] {
		t.Error("securitycheck should be opt-in only")
	}
	for _, id := range []string{suite.SpeedProviderChinaISP, suite.SpeedProviderSpeedtestISP} {
		if !slices.Contains(state.speedIDs, id) {
			t.Errorf("speed provider %s missing from TUI list", id)
		}
	}
}

func TestToggleMediaSetMutualExclusion(t *testing.T) {
	state := newSuiteConfigState()
	state.toggleMediaSet("jp")
	if state.mediaSets[suite.DefaultMediaSet()] {
		t.Error("selecting a region must clear the all-platform set")
	}
	if !state.mediaSets["jp"] {
		t.Fatal("jp should stay selected")
	}
	state.toggleMediaSet("kr")
	if !state.mediaSets["jp"] || !state.mediaSets["kr"] {
		t.Error("region sets must combine")
	}
	state.toggleMediaSet(suite.DefaultMediaSet())
	for _, id := range state.mediaIDs {
		if id != suite.DefaultMediaSet() && state.mediaSets[id] {
			t.Errorf("selecting all must clear %s", id)
		}
	}
}

func TestBuildOptionsCarriesMediaSetAndIPSources(t *testing.T) {
	state := newSuiteConfigState()
	state.sections = suite.SectionSelector{Media: true, IPQuality: true}
	state.toggleMediaSet("jp")
	state.toggleMediaSet("kr")
	state.ipSources[suite.IPSourceSecurityCheck] = true

	norm, err := suite.NormalizeOptions(state.buildOptions(""))
	if err != nil {
		t.Fatal(err)
	}
	if norm.MediaSet != "jp,kr" {
		t.Fatalf("normalized MediaSet = %q, want jp,kr", norm.MediaSet)
	}
	if strings.Join(norm.IPSources, ",") != "builtin,securitycheck" {
		t.Fatalf("normalized IPSources = %v", norm.IPSources)
	}
}
