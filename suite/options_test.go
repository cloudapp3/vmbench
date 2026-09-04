package suite

import (
	"testing"

	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

func TestPrepareOptionsAppliesPreset(t *testing.T) {
	opts := PrepareOptions(Options{Preset: "proxy"})

	if opts.Preset != "proxy" {
		t.Fatalf("preset = %q, want proxy", opts.Preset)
	}
	if !opts.Sections.NetworkInfo || !opts.Sections.Route || !opts.Sections.Ping || !opts.Sections.Speed || !opts.Sections.IPQuality || !opts.Sections.Reachability || !opts.Sections.Media {
		t.Fatalf("proxy sections = %+v, want network_info/route/ping/speed/ip_quality/reachability/media enabled", opts.Sections)
	}
	if opts.Sections.Hardware || opts.Sections.Mail {
		t.Fatalf("proxy sections = %+v, want hardware/mail disabled", opts.Sections)
	}
}

func TestDefaultSectionsIncludeNetworkEvidence(t *testing.T) {
	sections := DefaultSections()
	if !sections.NetworkInfo || !sections.Reachability {
		t.Fatalf("DefaultSections() = %+v, want network_info and reachability enabled", sections)
	}
}

func TestSectionsFromListSupportsNetworkEvidenceAliases(t *testing.T) {
	tests := []struct {
		value            string
		wantNetworkInfo  bool
		wantReachability bool
	}{
		{value: "network_info", wantNetworkInfo: true},
		{value: "network-identity", wantNetworkInfo: true},
		{value: "netinfo", wantNetworkInfo: true},
		{value: "reachability", wantReachability: true},
		{value: "website", wantReachability: true},
		{value: "telegram", wantReachability: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			sections := SectionsFromList(tt.value, SectionSelector{}, true)
			if sections.NetworkInfo != tt.wantNetworkInfo || sections.Reachability != tt.wantReachability {
				t.Fatalf("SectionsFromList(%q) = %+v", tt.value, sections)
			}
		})
	}
}

func TestLegacyNetworkAliasStillSelectsSpeed(t *testing.T) {
	sections := SectionsFromList("network", SectionSelector{}, true)
	if !sections.Speed || sections.NetworkInfo {
		t.Fatalf("SectionsFromList(network) = %+v, want speed only", sections)
	}
}

func TestPrepareOptionsKeepsExplicitSectionsOverPreset(t *testing.T) {
	opts := PrepareOptions(Options{
		Preset:   "website",
		Sections: SectionSelector{Mail: true},
	})

	if opts.Preset != "website" {
		t.Fatalf("preset = %q, want website", opts.Preset)
	}
	if opts.Sections != (SectionSelector{Mail: true}) {
		t.Fatalf("sections = %+v, want explicit mail-only selector", opts.Sections)
	}
}

func TestSectionSelectorString(t *testing.T) {
	sections := SectionSelector{Hardware: true, Ping: true, IPQuality: true}

	if got, want := sections.String(), "hardware,ping,ip_quality"; got != want {
		t.Fatalf("SectionSelector.String() = %q, want %q", got, want)
	}
}

func TestStandardizeSpeedProviders(t *testing.T) {
	got := StandardizeSpeedProviders([]string{"cloudflare", "speedtest.net", "iperf3", "cloudflare"})
	want := []string{SpeedProviderCloudflare, SpeedProviderSpeedtestNet, SpeedProviderIperf3}

	if len(got) != len(want) {
		t.Fatalf("len(StandardizeSpeedProviders) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("StandardizeSpeedProviders[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestPrepareOptionsDefaultSpeedProviders(t *testing.T) {
	opts := PrepareOptions(Options{
		Sections: SectionSelector{Speed: true},
	})

	if len(opts.SpeedProviders) != 1 || opts.SpeedProviders[0] != SpeedProviderCloudflare {
		t.Fatalf("SpeedProviders = %v, want cloudflare only", opts.SpeedProviders)
	}
}

func TestSpeedProviderCatalogKeepsAllProviders(t *testing.T) {
	want := []string{SpeedProviderCloudflare, SpeedProviderSpeedtestNet, SpeedProviderSpeedtestCN, SpeedProviderIperf3, SpeedProviderChinaISP, SpeedProviderSpeedtestISP}
	got := SpeedProviderIDs()
	if len(got) != len(want) {
		t.Fatalf("SpeedProviderIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SpeedProviderIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrepareOptionsHardwareTools(t *testing.T) {
	opts := PrepareOptions(Options{
		Sections:      SectionSelector{Hardware: true},
		HardwareTools: []string{"dd", "gb6", "stream_c"},
	})

	want := []string{catalog.HardwareToolDD, catalog.HardwareToolStream, catalog.HardwareToolGeekbench}
	if len(opts.HardwareTools) != len(want) {
		t.Fatalf("HardwareTools = %v, want %v", opts.HardwareTools, want)
	}
	for i := range want {
		if opts.HardwareTools[i] != want[i] {
			t.Fatalf("HardwareTools[%d] = %q, want %q (full=%v)", i, opts.HardwareTools[i], want[i], opts.HardwareTools)
		}
	}
}

func TestPrepareOptionsDefaultHardwareTools(t *testing.T) {
	opts := PrepareOptions(Options{
		Sections: SectionSelector{Hardware: true},
	})

	want := catalog.DefaultHardwareTools()
	if len(opts.HardwareTools) != len(want) {
		t.Fatalf("HardwareTools = %v, want %v", opts.HardwareTools, want)
	}
	for i := range want {
		if opts.HardwareTools[i] != want[i] {
			t.Fatalf("HardwareTools[%d] = %q, want %q", i, opts.HardwareTools[i], want[i])
		}
	}
}

func TestPrepareOptionsKeepsOnlyUsedIperfHosts(t *testing.T) {
	disabled := PrepareOptions(Options{
		Sections:   SectionSelector{Hardware: true},
		IperfHosts: []string{"unused.example:5201"},
	})
	if len(disabled.IperfHosts) != 0 {
		t.Fatalf("speed-disabled iperf hosts = %v, want none", disabled.IperfHosts)
	}

	enabled := PrepareOptions(Options{
		Sections:   SectionSelector{Speed: true},
		IperfHosts: []string{" one.example:5201 ", "", "one.example:5201"},
	})
	if len(enabled.IperfHosts) != 1 || enabled.IperfHosts[0] != "one.example:5201" {
		t.Fatalf("speed-enabled iperf hosts = %v, want one normalized host", enabled.IperfHosts)
	}
	if !containsString(enabled.SpeedProviders, SpeedProviderIperf3) {
		t.Fatalf("speed providers = %v, want iperf3", enabled.SpeedProviders)
	}
}

func TestExpandedRoutePresetsUseCatalogNodes(t *testing.T) {
	for _, id := range []string{"cd", "cernet", "cstnet"} {
		if got := StandardizeRoutePresets([]string{id}); len(got) != 1 || got[0] != id {
			t.Fatalf("route preset %q normalized to %v", id, got)
		}
	}
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	chengdu := routeTargetsForManifest(manifest, []string{"cd"}, "dual")
	if len(chengdu) == 0 {
		t.Fatal("Chengdu route preset has no catalog targets")
	}
	for _, target := range chengdu {
		if target.City != "Chengdu" || target.ID == "" || target.Protocol == "" || target.Source == "" {
			t.Fatalf("Chengdu target = %+v", target)
		}
	}
	cstnetV6 := pingTargetsForManifest(manifest, []string{"cstnet"}, "v6")
	if len(cstnetV6) == 0 {
		t.Fatal("CSTNET IPv6 ping preset has no catalog targets")
	}
	for _, target := range cstnetV6 {
		if target.Carrier != "CSTNET" || target.IPFamily != "v6" || target.ID == "" {
			t.Fatalf("CSTNET v6 target = %+v", target)
		}
	}
}
