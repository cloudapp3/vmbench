package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/selfupdate"
)

var updateMarker = []byte("cmd-test vmbench marker\n")

func updateHexSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// newUpdateTestServer serves a fake v9.9.9 release for the running platform.
// A non-empty wrongChecksum replaces the archive digest to force a mismatch.
func newUpdateTestServer(t *testing.T, wrongChecksum string) *httptest.Server {
	t.Helper()

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "vmbench", Mode: 0o755, Size: int64(len(updateMarker))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(updateMarker); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	archive := buffer.Bytes()
	if wrongChecksum == "" {
		wrongChecksum = updateHexSum(archive)
	}
	assetName := selfupdate.AssetName("9.9.9", runtime.GOOS, runtime.GOARCH)

	var server *httptest.Server
	releaseJSON := func() []byte {
		payload, _ := json.Marshal(map[string]any{
			"tag_name": "v9.9.9",
			"draft":    false,
			"assets": []map[string]any{
				{"name": assetName, "browser_download_url": server.URL + "/dl/" + assetName},
				{"name": "checksums.txt", "browser_download_url": server.URL + "/dl/checksums.txt"},
			},
		})
		return payload
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(releaseJSON())
	})
	mux.HandleFunc("/releases/tags/v9.9.9", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(releaseJSON())
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", wrongChecksum, assetName)
	})
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func withCurrentVersion(t *testing.T, version string) {
	t.Helper()
	original := currentVersion
	currentVersion = version
	t.Cleanup(func() { currentVersion = original })
}

func TestRunUpdateRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"update", "--timeout", "0s"},
		{"update", "stray"},
	} {
		if code := run(args); code != 2 {
			t.Errorf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestRunUpdateCheckJSON(t *testing.T) {
	server := newUpdateTestServer(t, "")
	output, code := captureStdout(t, func() int {
		return run([]string{"update", "--check", "--api-url", server.URL, "--json"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; output:\n%s", code, output)
	}
	var payload struct {
		Current         string `json:"current"`
		Latest          string `json:"latest"`
		UpdateAvailable bool   `json:"update_available"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode JSON %q: %v", output, err)
	}
	if !payload.UpdateAvailable || payload.Latest != "9.9.9" {
		t.Errorf("payload = %+v, want update available for 9.9.9", payload)
	}
}

func TestRunUpdateUpToDateLocalized(t *testing.T) {
	server := newUpdateTestServer(t, "")
	cases := []struct {
		lang string
		want string
	}{
		{"en", "already up to date"},
		{"zh-CN", "已是最新版本"},
	}
	for _, c := range cases {
		withLangEnv(t, c.lang)
		withCurrentVersion(t, "9.9.9")
		output, code := captureStdout(t, func() int {
			return run([]string{"update", "--check", "--api-url", server.URL})
		})
		if code != 0 {
			t.Fatalf("[%s] exit = %d, want 0", c.lang, code)
		}
		if !strings.Contains(output, c.want) {
			t.Errorf("[%s] output %q missing %q", c.lang, output, c.want)
		}
	}
}

func TestRunUpdateInstallsToDest(t *testing.T) {
	server := newUpdateTestServer(t, "")
	dest := filepath.Join(t.TempDir(), "vmbench")
	output, code := captureStdout(t, func() int {
		return run([]string{"update", "--api-url", server.URL, "--dest", dest, "--json"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; output:\n%s", code, output)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(data, updateMarker) {
		t.Errorf("dest content = %q, want marker", data)
	}
	var payload struct {
		Updated bool   `json:"updated"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode JSON %q: %v", output, err)
	}
	if !payload.Updated || payload.Path != dest {
		t.Errorf("payload = %+v, want updated at %s", payload, dest)
	}
}

func TestRunUpdatePinnedVersionReinstalls(t *testing.T) {
	server := newUpdateTestServer(t, "")
	withCurrentVersion(t, "9.9.9")
	dest := filepath.Join(t.TempDir(), "vmbench")
	_, code := captureStdout(t, func() int {
		return run([]string{"update", "--api-url", server.URL, "--version", "9.9.9", "--dest", dest})
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (pinned version reinstalls)", code)
	}
	if data, err := os.ReadFile(dest); err != nil || !bytes.Equal(data, updateMarker) {
		t.Errorf("dest content = %q (%v), want marker", data, err)
	}
}

func TestRunUpdateChecksumMismatch(t *testing.T) {
	server := newUpdateTestServer(t, strings.Repeat("0", 64))
	dest := filepath.Join(t.TempDir(), "vmbench")
	output, code := captureStderr(t, func() int {
		return run([]string{"update", "--api-url", server.URL, "--dest", dest})
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr:\n%s", code, output)
	}
	if !strings.Contains(output, "error:") {
		t.Errorf("stderr %q missing error prefix", output)
	}
	if !strings.Contains(output, "checksum mismatch") {
		t.Errorf("stderr %q missing checksum mismatch detail", output)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest should not exist after failed install (err=%v)", err)
	}
}
