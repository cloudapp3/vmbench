package vmbench

import (
	"slices"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

func TestCatalogNodeIDsIncludesPurePingNodesOnlyWhenSelected(t *testing.T) {
	manifest := nodecatalog.Manifest{
		SchemaVersion: nodecatalog.SchemaVersion,
		Revision:      "pure-kinds",
		GeneratedAt:   time.Now().Add(-time.Hour),
		ExpiresAt:     time.Now().Add(time.Hour),
		Nodes: []nodecatalog.Node{
			{ID: "ping-only", Name: "Ping", Kind: nodecatalog.KindPing, IPFamily: "v4", Protocol: "tcp", Endpoint: "192.0.2.10", Port: 80, Source: "test"},
			{ID: "route-only", Name: "Route", Kind: nodecatalog.KindRoute, IPFamily: "v4", Protocol: "tcp", Endpoint: "192.0.2.20", Port: 80, Source: "test"},
		},
	}
	workloads := []bench.Workload{netio.NewPingWorkloadWithManifest(manifest, "v4")}
	got := catalogNodeIDs(Options{ResolvedCatalog: &manifest}, workloads)
	if !slices.Equal(got, []string{"ping-only"}) {
		t.Fatalf("catalogNodeIDs() = %v, want pure ping node only", got)
	}
}
