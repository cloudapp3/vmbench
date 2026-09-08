package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var marker = []byte("#!/bin/sh\nfake vmbench binary marker\n")

type fixture struct {
	linuxArchive   []byte
	windowsArchive []byte
	checksums      string
}

func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func buildFixture(t *testing.T) fixture {
	t.Helper()

	var linux bytes.Buffer
	gz := gzip.NewWriter(&linux)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "vmbench", Mode: 0o755, Size: int64(len(marker))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(marker); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	var windows bytes.Buffer
	zw := zip.NewWriter(&windows)
	entry, err := zw.Create("vmbench.exe")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write(marker); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	fx := fixture{linuxArchive: linux.Bytes(), windowsArchive: windows.Bytes()}
	fx.checksums = fx.checksumsFor(AssetName("9.9.9", "linux", "amd64"), AssetName("9.9.9", "windows", "amd64"))
	return fx
}

// checksumsFor rebuilds checksums.txt for the given asset names using the
// fixture archives, letting tests tamper with specific entries.
func (fx fixture) checksumsFor(names ...string) string {
	var builder strings.Builder
	for _, name := range names {
		data := fx.linuxArchive
		if strings.HasSuffix(name, ".zip") {
			data = fx.windowsArchive
		}
		fmt.Fprintf(&builder, "%s  %s\n", sha256hex(data), name)
	}
	return builder.String()
}

type updateServer struct {
	*httptest.Server
	linuxName   string
	windowsName string
	userAgents  []string
	authorities []string
}

func newUpdateServer(t *testing.T, fx fixture) *updateServer {
	t.Helper()
	mux := http.NewServeMux()
	var server *updateServer
	release := func(w http.ResponseWriter, tag string, draft bool, names ...string) {
		assets := make([]map[string]any, 0, len(names))
		for _, name := range names {
			assets = append(assets, map[string]any{
				"name":                 name,
				"browser_download_url": server.Server.URL + "/dl/" + name,
				"size":                 len(fx.linuxArchive),
			})
		}
		payload, err := json.Marshal(map[string]any{
			"tag_name": tag,
			"draft":    draft,
			"assets":   assets,
		})
		if err != nil {
			t.Errorf("marshal release JSON: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}

	linuxName := AssetName("9.9.9", "linux", "amd64")
	windowsName := AssetName("9.9.9", "windows", "amd64")
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		release(w, "v9.9.9", false, linuxName, windowsName, "checksums.txt")
	})
	mux.HandleFunc("/releases/tags/v0.4.0", func(w http.ResponseWriter, r *http.Request) {
		release(w, "v0.4.0", false, "vmbench-0.4.0-linux-amd64.tar.gz", "checksums.txt")
	})
	mux.HandleFunc("/releases/tags/v0.5.0", func(w http.ResponseWriter, r *http.Request) {
		release(w, "v0.4.0", false, linuxName, "checksums.txt")
	})
	mux.HandleFunc("/releases/tags/v0.8.0", func(w http.ResponseWriter, r *http.Request) {
		release(w, "v0.8.0", true, linuxName, "checksums.txt")
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fx.checksums))
	})
	mux.HandleFunc("/dl/"+linuxName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fx.linuxArchive)
	})
	mux.HandleFunc("/dl/"+windowsName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fx.windowsArchive)
	})

	server = &updateServer{linuxName: linuxName, windowsName: windowsName}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.userAgents = append(server.userAgents, r.Header.Get("User-Agent"))
		server.authorities = append(server.authorities, r.Header.Get("Authorization"))
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Server.Close)
	return server
}

func TestCheckLatest(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	release, err := Check(context.Background(), Options{APIBase: server.URL})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if release.Tag != "v9.9.9" || release.Ver != "9.9.9" {
		t.Errorf("release = %q/%q, want v9.9.9/9.9.9", release.Tag, release.Ver)
	}
	if len(release.Assets) != 3 {
		t.Errorf("assets = %d, want 3", len(release.Assets))
	}
}

func TestCheckPinnedTag(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	release, err := Check(context.Background(), Options{APIBase: server.URL, Tag: "0.4.0"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if release.Tag != "v0.4.0" {
		t.Errorf("tag = %q, want v0.4.0", release.Tag)
	}
}

func TestCheckPinnedNotFound(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	_, err := Check(context.Background(), Options{APIBase: server.URL, Tag: "v-nope"})
	if err == nil || !strings.Contains(err.Error(), "v-nope") {
		t.Errorf("error = %v, want release v-nope not found", err)
	}
}

func TestCheckRejectsDraft(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	_, err := Check(context.Background(), Options{APIBase: server.URL, Tag: "v0.8.0"})
	if err == nil || !strings.Contains(err.Error(), "draft") {
		t.Errorf("error = %v, want draft rejection", err)
	}
}

func TestCheckPinnedTagMismatch(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	_, err := Check(context.Background(), Options{APIBase: server.URL, Tag: "v0.5.0"})
	if err == nil || !strings.Contains(err.Error(), "want v0.5.0") {
		t.Errorf("error = %v, want tag mismatch", err)
	}
}

func TestCheckRequestHeaders(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	_, err := Check(context.Background(), Options{APIBase: server.URL, Current: "0.6.0", Token: "secret"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(server.userAgents) == 0 || server.userAgents[0] != "vmbench/0.6.0" {
		t.Errorf("User-Agent = %v, want vmbench/0.6.0", server.userAgents)
	}
	if len(server.authorities) == 0 || server.authorities[0] != "Bearer secret" {
		t.Errorf("Authorization = %v, want Bearer secret", server.authorities)
	}
}

func TestInstallLinuxEndToEnd(t *testing.T) {
	fx := buildFixture(t)
	server := newUpdateServer(t, fx)
	dest := filepath.Join(t.TempDir(), "vmbench")
	options := Options{
		APIBase: server.URL,
		Current: "0.6.0",
		GOOS:    "linux",
		GOARCH:  "amd64",
		Dest:    dest,
		Client:  server.Server.Client(),
	}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	result, err := Install(context.Background(), options, release)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(data, marker) {
		t.Errorf("dest content = %q, want marker", data)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("dest mode = %o, want 755", info.Mode().Perm())
	}
	if result.NewVersion != "9.9.9" || result.OldVersion != "0.6.0" || result.Path != dest {
		t.Errorf("result = %+v", result)
	}
	if result.AssetName != server.linuxName || result.Bytes != int64(len(fx.linuxArchive)) {
		t.Errorf("result asset/bytes = %q/%d", result.AssetName, result.Bytes)
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("dest dir has %d entries (%v), want 1 (no temp leftovers)", len(entries), entries)
	}
}

func TestInstallPreservesExistingMode(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	dir := t.TempDir()
	dest := filepath.Join(dir, "vmbench")
	if err := os.WriteFile(dest, []byte("old"), 0o700); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: dest}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := Install(context.Background(), options, release); err != nil {
		t.Fatalf("Install: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("dest mode = %o, want preserved 700", info.Mode().Perm())
	}
	data, _ := os.ReadFile(dest)
	if !bytes.Equal(data, marker) {
		t.Errorf("dest content = %q, want marker", data)
	}
}

func TestInstallWindowsZip(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	dir := t.TempDir()
	dest := filepath.Join(dir, "vmbench.exe")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	options := Options{APIBase: server.URL, GOOS: "windows", GOARCH: "amd64", Dest: dest}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := Install(context.Background(), options, release); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(data, marker) {
		t.Errorf("dest content = %q, want marker", data)
	}
	if _, err := os.Stat(dest + ".old"); !os.IsNotExist(err) {
		t.Errorf("aside file %s.old should be removed (err=%v)", dest, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1", len(entries))
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	fx := buildFixture(t)
	fx.checksums = "0000000000000000000000000000000000000000000000000000000000000000  " + AssetName("9.9.9", "linux", "amd64") + "\n"
	server := newUpdateServer(t, fx)
	dir := t.TempDir()
	dest := filepath.Join(dir, "vmbench")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: dest}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	_, err = Install(context.Background(), options, release)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "old" {
		t.Errorf("dest content = %q, want unchanged", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1", len(entries))
	}
}

func TestInstallChecksumEntryMissing(t *testing.T) {
	fx := buildFixture(t)
	fx.checksums = fx.checksumsFor("some-other-archive.tar.gz")
	server := newUpdateServer(t, fx)
	dest := filepath.Join(t.TempDir(), "vmbench")
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: dest}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	_, err = Install(context.Background(), options, release)
	if err == nil || !strings.Contains(err.Error(), "no entry for") {
		t.Errorf("error = %v, want missing checksum entry", err)
	}
}

func TestInstallReleaseWithoutChecksums(t *testing.T) {
	release := Release{Tag: "v1.0.0", Ver: "1.0.0", Assets: []Asset{
		{Name: AssetName("1.0.0", "linux", "amd64"), URL: "http://ignored"},
	}}
	_, err := Install(context.Background(), Options{Dest: filepath.Join(t.TempDir(), "vmbench")}, release)
	if err == nil || !strings.Contains(err.Error(), "no checksums.txt") {
		t.Errorf("error = %v, want missing checksums.txt", err)
	}
}

func TestInstallNoMatchingAsset(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	options := Options{APIBase: server.URL, GOOS: "plan9", GOARCH: "amd64", Dest: filepath.Join(t.TempDir(), "vmbench")}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	_, err = Install(context.Background(), options, release)
	if err == nil || !strings.Contains(err.Error(), "no asset") {
		t.Errorf("error = %v, want no matching asset", err)
	}
	if !strings.Contains(err.Error(), server.linuxName) {
		t.Errorf("error %q should list available assets", err)
	}
}

func TestInstallTruncatedArchive(t *testing.T) {
	fx := buildFixture(t)
	fx.linuxArchive = []byte("definitely not gzip")
	fx.checksums = fx.checksumsFor(AssetName("9.9.9", "linux", "amd64"), AssetName("9.9.9", "windows", "amd64"))
	server := newUpdateServer(t, fx)
	dest := filepath.Join(t.TempDir(), "vmbench")
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: dest}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	_, err = Install(context.Background(), options, release)
	if err == nil || !strings.Contains(err.Error(), "read release archive") {
		t.Errorf("error = %v, want archive decode failure", err)
	}
}

func TestInstallSizeCap(t *testing.T) {
	fx := buildFixture(t)
	server := newUpdateServer(t, fx)
	dest := filepath.Join(t.TempDir(), "vmbench")
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: dest, MaxArchiveBytes: 4}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	_, err = Install(context.Background(), options, release)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want size cap", err)
	}
}

func TestInstallNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits do not block writes")
	}
	server := newUpdateServer(t, buildFixture(t))
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := filepath.Join(dir, "vmbench")
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: dest}
	release, err := Check(context.Background(), options)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	_, err = Install(context.Background(), options, release)
	if !strings.Contains(fmt.Sprint(err), ErrTargetNotWritable.Error()) {
		t.Errorf("error = %v, want %v", err, ErrTargetNotWritable)
	}
}

func TestInstallContextCancelled(t *testing.T) {
	server := newUpdateServer(t, buildFixture(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	options := Options{APIBase: server.URL, GOOS: "linux", GOARCH: "amd64", Dest: filepath.Join(t.TempDir(), "vmbench")}
	if _, err := Check(ctx, options); err == nil {
		t.Fatal("Check with cancelled context should fail")
	}
}
