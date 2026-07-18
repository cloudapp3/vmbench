package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/nodecatalog"
)

func TestRunNodesRejectsInvalidArguments(t *testing.T) {
	tests := [][]string{
		{"nodes"},
		{"nodes", "unknown"},
		{"nodes", "list", "--kind", "mystery"},
		{"nodes", "list", "--ip-family", "v5"},
		{"nodes", "verify", "--signature", "nodes.sig"},
		{"nodes", "update", "--url", "https://example.com/nodes.json", "--signature", "nodes.sig"},
		{"nodes", "health", "--timeout", "0s"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestRunNodesVerifiesPinnedEmbeddedCatalog(t *testing.T) {
	if code := run([]string{"nodes", "verify", "--node-revision", "2026-07-13.1", "--json"}); code != 0 {
		t.Fatalf("nodes verify exit = %d, want 0", code)
	}
	if code := run([]string{"nodes", "verify", "--node-revision", "not-the-embedded-revision"}); code != 1 {
		t.Fatalf("nodes verify wrong revision exit = %d, want 1", code)
	}
}

func TestRunNodesHealthJSONIsStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	manifest := nodecatalog.Manifest{
		SchemaVersion: nodecatalog.SchemaVersion,
		Revision:      "health-cli-r1",
		GeneratedAt:   time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
		ExpiresAt:     time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		Nodes: []nodecatalog.Node{{
			ID: "health-http", Name: "Health HTTP", Kind: nodecatalog.KindDownload,
			Region: "test", City: "local", Carrier: "test", IPFamily: "v4",
			Protocol: "http", Endpoint: "127.0.0.1", URL: server.URL + "/probe",
			TrafficBytes: 1, Source: "test",
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestNodesCLIHelperProcess$", "--",
		"nodes", "health", "--node-catalog", path, "--timeout", "2s", "--json")
	command.Env = append(os.Environ(), "VMBENCH_NODES_HELPER_PROCESS=1")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	exitCode := -1
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	if err == nil || exitCode != 1 {
		t.Fatalf("nodes health error = %v, exit = %d, stderr = %s", err, exitCode, stderr.String())
	}

	var output struct {
		Revision string                     `json:"revision"`
		Source   string                     `json:"source"`
		Healthy  int                        `json:"healthy"`
		Failed   int                        `json:"failed"`
		Results  []nodecatalog.HealthResult `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode nodes health JSON: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if output.Revision != manifest.Revision || output.Source != nodecatalog.SourcePath || output.Healthy != 0 || output.Failed != 1 {
		t.Fatalf("nodes health summary = %+v", output)
	}
	if len(output.Results) != 1 {
		t.Fatalf("nodes health results = %+v", output.Results)
	}
	result := output.Results[0]
	if result.NodeID != "health-http" || result.Status != "error" || result.Method != "HEAD" ||
		result.Endpoint != server.URL+"/probe" || result.HTTPStatus != http.StatusServiceUnavailable || result.Error != "HTTP 503" {
		t.Fatalf("nodes health result = %+v", result)
	}
}

func TestNodesCLIHelperProcess(t *testing.T) {
	if os.Getenv("VMBENCH_NODES_HELPER_PROCESS") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			os.Exit(run(os.Args[index+1:]))
		}
	}
	t.Fatal("missing helper process argument separator")
}
