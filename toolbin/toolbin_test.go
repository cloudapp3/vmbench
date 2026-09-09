package toolbin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testTool(sum string) Tool {
	return Tool{
		Name:    "fio",
		Version: "3.39",
		Sums:    map[string]string{"linux/amd64": sum, "linux/arm64": sum},
	}
}

func TestFetchInstallsPinnedBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fetch installs a linux binary")
	}
	payload := []byte("#!/bin/sh\nexit 0\n")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vmbench-tools-fio-linux-amd64" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dest := t.TempDir()
	result, err := Fetch(context.Background(), testTool(hex.EncodeToString(sum[:])), Options{
		Base: server.URL, Dest: dest, GOOS: "linux", GOARCH: "amd64",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	want := filepath.Join(dest, "fio_x64")
	if result.Path != want {
		t.Fatalf("Fetch() path = %q, want %q", result.Path, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("installed binary is not executable: %v", info.Mode())
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatal("installed binary content mismatch")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 1 {
		t.Fatalf("destination has %d entries, want exactly the installed binary (temp cleaned)", len(entries))
	}
}

func TestFetchFailsClosedOnChecksumMismatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fetch installs a linux binary")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tampered payload"))
	}))
	defer server.Close()

	dest := t.TempDir()
	_, err := Fetch(context.Background(), testTool(strings.Repeat("ab", 32)), Options{
		Base: server.URL, Dest: dest, GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Fetch() error = %v, want checksum mismatch", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("destination has %d entries after failed fetch, want 0 (fail-closed)", len(entries))
	}
}

func TestFetchRejectsPlaceholderPin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("anything"))
	}))
	defer server.Close()

	_, err := Fetch(context.Background(), testTool(strings.Repeat("0", 64)), Options{
		Base: server.URL, Dest: t.TempDir(), GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "no pinned build") {
		t.Fatalf("Fetch() error = %v, want no pinned build rejection", err)
	}
}

func TestFetchRejectsUnsupportedPlatform(t *testing.T) {
	_, err := Fetch(context.Background(), testTool(strings.Repeat("c", 64)), Options{
		Dest: t.TempDir(), GOOS: "darwin", GOARCH: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "no pinned build") {
		t.Fatalf("Fetch() error = %v, want missing pin error", err)
	}
	_, err = Fetch(context.Background(), testTool(strings.Repeat("c", 64)), Options{
		Dest: t.TempDir(), GOOS: "linux", GOARCH: "386",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported architecture") {
		t.Fatalf("Fetch() error = %v, want unsupported architecture error", err)
	}
}

func TestFetchEnforcesSizeCap(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fetch installs a linux binary")
	}
	big := make([]byte, maxAssetBytes+1)
	sum := sha256.Sum256(big)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(big)
	}))
	defer server.Close()

	dest := t.TempDir()
	_, err := Fetch(context.Background(), testTool(hex.EncodeToString(sum[:])), Options{
		Base: server.URL, Dest: dest, GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Fetch() error = %v, want size cap error", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("destination has %d entries after oversized fetch, want 0", len(entries))
	}
}

func TestFetchSurfacesHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := Fetch(context.Background(), testTool(strings.Repeat("d", 64)), Options{
		Base: server.URL, Dest: t.TempDir(), GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("Fetch() error = %v, want HTTP 404 error", err)
	}
}

func TestRegistryCoversLinuxAmd64AndArm64(t *testing.T) {
	for _, tool := range Registry {
		for _, plat := range []string{"linux/amd64", "linux/arm64"} {
			if tool.Sums[plat] == "" {
				t.Errorf("%s has no pin for %s", tool.Name, plat)
			}
		}
	}
	if _, ok := Find("fio"); !ok {
		t.Error("Find(fio) missing from registry")
	}
	if _, ok := Find("openssl"); ok {
		t.Error("openssl must stay PATH-only, not provisionable")
	}
}
