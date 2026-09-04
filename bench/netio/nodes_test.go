package netio

import (
	"context"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/nodecatalog"
)

func TestDefaultNodesUseEmbeddedCatalog(t *testing.T) {
	nodes := DefaultNodes()
	if len(nodes) != 15 {
		t.Fatalf("DefaultNodes() count = %d, want 15", len(nodes))
	}
	if nodes[0].ID != "download-vultr-tokyo" || nodes[0].TrafficBytes != 104857600 {
		t.Fatalf("DefaultNodes()[0] = %+v", nodes[0])
	}
}

func TestManifestPinnedWorkloadConstructors(t *testing.T) {
	now := time.Now().UTC()
	manifest := nodecatalog.Manifest{
		SchemaVersion: nodecatalog.SchemaVersion,
		Revision:      "custom-r1",
		GeneratedAt:   now.Add(-time.Hour),
		ExpiresAt:     now.Add(time.Hour),
		Nodes: []nodecatalog.Node{
			{ID: "custom-ping", Name: "Custom ping", Kind: nodecatalog.KindRoutePing, Region: "test", City: "test", Carrier: "test", ASN: 64512, IPFamily: "v6", Protocol: "tcp", Endpoint: "2001:db8::1", Port: 443, Source: "test"},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}

	ping := NewPingWorkloadWithManifest(manifest, "v6").(*pingWorkload)
	if len(ping.targets) != 1 || ping.targets[0].ID != "custom-ping" {
		t.Fatalf("pinned ping targets = %+v", ping.targets)
	}
	ping.probeTargets = func(_ context.Context, targets []PingTarget) ([]PingProbeResult, error) {
		if len(targets) != 1 || targets[0].ID != "custom-ping" {
			t.Fatalf("runtime ping targets = %+v", targets)
		}
		return []PingProbeResult{{ID: targets[0].ID, Name: targets[0].Name, Status: "ok", Sent: 1, Received: 1, AvgLatencyMs: 1}}, nil
	}
	if _, _, err := ping.Run(context.Background()); err != nil {
		t.Fatalf("pinned ping Run() error = %v", err)
	}

	trace := NewTracerouteWorkloadWithManifest(manifest, "v6").(*tracerouteWorkload)
	if len(trace.targets) != 1 || trace.targets[0].ID != "custom-ping" {
		t.Fatalf("pinned trace targets = %+v", trace.targets)
	}

	empty := NewPingWorkloadWithManifest(manifest, "v4").(*pingWorkload)
	empty.probeTargets = func(_ context.Context, targets []PingTarget) ([]PingProbeResult, error) {
		if len(targets) != 0 {
			t.Fatalf("mismatched family fell back to targets: %+v", targets)
		}
		return nil, nil
	}
	if _, _, err := empty.Run(context.Background()); err == nil || err.Error() != "all 0 ping targets failed" {
		t.Fatalf("empty pinned ping error = %v", err)
	}
}

func TestCatalogRouteAndPingMappings(t *testing.T) {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	v4 := TraceTargetsFromManifest(manifest, "v4")
	v6 := TraceTargetsFromManifest(manifest, "v6")
	dual := PingTargetsFromManifest(manifest, "dual")
	if len(v4) == 0 || len(v6) == 0 || len(dual) != len(v4)+len(v6) {
		t.Fatalf("mapped counts v4=%d v6=%d dual=%d", len(v4), len(v6), len(dual))
	}
	if v4[0].ID == "" || v4[0].IPFamily != "v4" || v6[0].IPFamily != "v6" {
		t.Fatalf("mapped targets v4=%+v v6=%+v", v4[0], v6[0])
	}
	if len(DefaultTraceTargets()) != len(v4) {
		t.Fatalf("DefaultTraceTargets() count = %d, want %d", len(DefaultTraceTargets()), len(v4))
	}
}

func TestISPDownloadNodesUseEmbeddedCatalog(t *testing.T) {
	nodes := DefaultISPDownloadNodes()
	if len(nodes) != 12 {
		t.Fatalf("DefaultISPDownloadNodes() count = %d, want 12", len(nodes))
	}
	perCarrier := map[string]int{}
	for _, node := range nodes {
		if node.Carrier == "" || node.Region != "China" {
			t.Fatalf("unexpected isp node %+v", node)
		}
		if node.TrafficBytes <= 0 {
			t.Fatalf("isp node %s missing traffic budget", node.ID)
		}
		perCarrier[node.Carrier]++
	}
	for _, carrier := range CarrierOrder() {
		if perCarrier[carrier] != 4 {
			t.Fatalf("carrier %s node count = %d, want 4", carrier, perCarrier[carrier])
		}
	}
	// isp_download nodes must not leak into the generic download node list.
	for _, node := range DefaultNodes() {
		if node.Carrier == "telecom" || node.Carrier == "unicom" || node.Carrier == "mobile" {
			t.Fatalf("isp node %s leaked into DefaultNodes()", node.ID)
		}
	}
}

func TestOoklaCarrierServersCoverAllCarriers(t *testing.T) {
	servers := OoklaCarrierServers()
	for _, carrier := range CarrierOrder() {
		ids := servers[carrier]
		if len(ids) == 0 {
			t.Fatalf("carrier %s has no Ookla server IDs", carrier)
		}
		for _, id := range ids {
			if id <= 0 {
				t.Fatalf("carrier %s has invalid server ID %d", carrier, id)
			}
		}
	}
}
