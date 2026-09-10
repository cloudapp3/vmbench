package nodecatalog

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain keeps this test binary hermetic: the default mirror chain points
// at an address that refuses connections instantly, so no test ever touches
// the network unless it overrides the chain with withRemote.
func TestMain(m *testing.M) {
	DefaultManifestURLs = "http://127.0.0.1:1/nodes.json"
	os.Exit(m.Run())
}

// withRemote points the mirror chain at manifestURL for one test and
// restores the TestMain stub afterwards.
func withRemote(t *testing.T, manifestURL string) {
	t.Helper()
	oldURLs := DefaultManifestURLs
	DefaultManifestURLs = manifestURL
	t.Cleanup(func() { DefaultManifestURLs = oldURLs })
}

// remoteServer serves raw bytes at /nodes.json and counts requests.
func remoteServer(t *testing.T, raw []byte) (*httptest.Server, *int64) {
	t.Helper()
	hits := new(int64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nodes.json" {
			*hits++
			_, _ = w.Write(raw)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server, hits
}

func autoLoad(t *testing.T, cachePath string, revision string) Loaded {
	t.Helper()
	loaded, err := Load(LoadOptions{Source: SourceAuto, CachePath: cachePath, Revision: revision})
	if err != nil {
		t.Fatalf("Load(auto) error = %v", err)
	}
	return loaded
}

func TestLoadAutoFetchesAndCaches(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("fetched-r9", time.Now().Add(time.Hour)))
	server, hits := remoteServer(t, raw)
	withRemote(t, server.URL+"/nodes.json")

	cache := filepath.Join(t.TempDir(), "nodes.json")
	loaded := autoLoad(t, cache, "")
	if loaded.Manifest.Revision != "fetched-r9" || loaded.Source != SourceRemote || loaded.Path != "" {
		t.Fatalf("Load(auto) = %+v", loaded)
	}
	if loaded.Warning != "" {
		t.Fatalf("unexpected warning %q", loaded.Warning)
	}
	written, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(raw) {
		t.Fatal("cached bytes differ from fetched document")
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(cache); err == nil && info.Mode().Perm() != 0o600 {
			t.Fatalf("cache mode = %o, want 600", info.Mode().Perm())
		}
	}
	if *hits == 0 {
		t.Fatal("manifest was never requested")
	}
}

func TestLoadAutoFallsBackToCacheSilentlyWhenOffline(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("remote-dead", time.Now().Add(time.Hour)))
	server, _ := remoteServer(t, raw)
	url := server.URL + "/nodes.json"
	server.Close()

	cacheRaw := mustManifestJSON(t, testManifest("cached-r5", time.Now().Add(time.Hour)))
	cache := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(cache, cacheRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	withRemote(t, url)

	loaded := autoLoad(t, cache, "")
	if loaded.Source != SourceAuto || loaded.Manifest.Revision != "cached-r5" {
		t.Fatalf("Load(auto offline) = %+v", loaded)
	}
	if loaded.Warning != "" {
		t.Fatalf("offline fallback must be silent, got warning %q", loaded.Warning)
	}
}

func TestLoadAutoFallsBackToEmbeddedSilentlyOnNetworkError(t *testing.T) {
	withRemote(t, "http://127.0.0.1:1/nodes.json")
	cache := filepath.Join(t.TempDir(), "nodes.json") // does not exist

	loaded := autoLoad(t, cache, "")
	if loaded.Source != SourceEmbedded {
		t.Fatalf("source = %q, want embedded", loaded.Source)
	}
	if loaded.Warning != "" {
		t.Fatalf("unreachable chain must be silent, got warning %q", loaded.Warning)
	}
}

func TestLoadAutoSkipsInvalidSchemaSilently(t *testing.T) {
	server, _ := remoteServer(t, []byte("not json"))
	withRemote(t, server.URL+"/nodes.json")

	cache := filepath.Join(t.TempDir(), "nodes.json")
	loaded := autoLoad(t, cache, "")
	if loaded.Source != SourceEmbedded {
		t.Fatalf("source = %q, want embedded fallback", loaded.Source)
	}
	if loaded.Warning != "" {
		t.Fatalf("schema-invalid remote must be silent, got %q", loaded.Warning)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("invalid bytes must not be cached")
	}
}

func TestLoadAutoTriesNextMirrorAfterServerError(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("mirror-r2", time.Now().Add(time.Hour)))
	server, hits := remoteServer(t, raw)

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)

	withRemote(t, broken.URL+"/nodes.json "+server.URL+"/nodes.json")
	started := time.Now()
	loaded := autoLoad(t, filepath.Join(t.TempDir(), "nodes.json"), "")
	if loaded.Source != SourceRemote || loaded.Manifest.Revision != "mirror-r2" {
		t.Fatalf("Load(auto chain) = %+v", loaded)
	}
	if loaded.Warning != "" {
		t.Fatalf("earlier mirror failure must be silent once a later mirror wins, got %q", loaded.Warning)
	}
	if *hits != 1 {
		t.Fatalf("good mirror hits = %d, want 1", *hits)
	}
	if elapsed := time.Since(started); elapsed >= DefaultFetchTimeout {
		t.Fatalf("chain walk took %s, budget is %s", elapsed, DefaultFetchTimeout)
	}
}

func TestLoadAutoPinMismatchKeepsPinnedCache(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("fetched-new", time.Now().Add(time.Hour)))
	server, _ := remoteServer(t, raw)
	withRemote(t, server.URL+"/nodes.json")

	pinnedRaw := mustManifestJSON(t, testManifest("pinned-old", time.Now().Add(time.Hour)))
	cache := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(cache, pinnedRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := autoLoad(t, cache, "pinned-old")
	if loaded.Source != SourceAuto || loaded.Manifest.Revision != "pinned-old" {
		t.Fatalf("Load(auto pinned) = %+v", loaded)
	}
	if loaded.Warning != "" {
		t.Fatalf("pin mismatch must be silent, got %q", loaded.Warning)
	}
	written, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(pinnedRaw) {
		t.Fatal("non-matching remote revision must not clobber the pinned cache")
	}
}

func TestLoadAutoReportsExpiredRemote(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("expired-r1", time.Now().Add(-time.Minute)))
	server, _ := remoteServer(t, raw)
	withRemote(t, server.URL+"/nodes.json")

	loaded := autoLoad(t, filepath.Join(t.TempDir(), "nodes.json"), "")
	if loaded.Source != SourceRemote || !strings.Contains(loaded.Warning, "expired") {
		t.Fatalf("Load(auto expired) = %+v", loaded)
	}
}

func TestLoadAutoCacheWriteFailureWarns(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("writable-r1", time.Now().Add(time.Hour)))
	server, _ := remoteServer(t, raw)
	withRemote(t, server.URL+"/nodes.json")

	// A regular file where the cache directory should be makes MkdirAll
	// fail deterministically on every platform.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := autoLoad(t, filepath.Join(blocker, "nodes.json"), "")
	if loaded.Source != SourceRemote || loaded.Manifest.Revision != "writable-r1" {
		t.Fatalf("fetched manifest must win: %+v", loaded)
	}
	if !strings.Contains(loaded.Warning, "cached catalog not updated") {
		t.Fatalf("cache write failure must warn, got %q", loaded.Warning)
	}
}

func TestLoadAutoConcurrentLoadsAreSafe(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("concurrent-r1", time.Now().Add(time.Hour)))
	server, _ := remoteServer(t, raw)
	withRemote(t, server.URL+"/nodes.json")

	cache := filepath.Join(t.TempDir(), "nodes.json")
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			loaded, err := Load(LoadOptions{Source: SourceAuto, CachePath: cache})
			if err != nil || loaded.Source != SourceRemote {
				t.Errorf("concurrent Load = %+v, %v", loaded, err)
			}
		})
	}
	wg.Wait()
	written, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(written); err != nil {
		t.Fatalf("concurrent cache write left invalid bytes: %v", err)
	}
}
