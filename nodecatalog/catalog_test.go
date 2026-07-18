package nodecatalog

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbeddedCatalogCoverage(t *testing.T) {
	manifest, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded() error = %v", err)
	}
	if manifest.Revision != "2026-07-13.1" {
		t.Fatalf("Revision = %q", manifest.Revision)
	}
	if got := len(manifest.Select(Filter{Kind: KindDownload})); got != 15 {
		t.Fatalf("download nodes = %d, want 15", got)
	}
	for _, test := range []struct {
		filter Filter
		label  string
	}{
		{Filter{Kind: KindRoute, City: "Chengdu"}, "Chengdu"},
		{Filter{Kind: KindPing, Carrier: "CERNET"}, "CERNET"},
		{Filter{Kind: KindRoute, Carrier: "CSTNET"}, "CSTNET"},
		{Filter{Kind: KindPing, IPFamily: "v6"}, "IPv6 ping"},
	} {
		if got := manifest.Select(test.filter); len(got) == 0 {
			t.Fatalf("embedded catalog has no %s nodes", test.label)
		}
	}

	manifest.Nodes[0].Name = "mutated"
	again, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if again.Nodes[0].Name == "mutated" {
		t.Fatal("Embedded() returned shared mutable nodes")
	}
}

func TestDecodeRejectsUnknownAndTrailingJSON(t *testing.T) {
	raw := mustManifestJSON(t, testManifest("r1", time.Now().Add(time.Hour)))
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	object["unexpected"] = true
	withUnknown, _ := json.Marshal(object)
	if _, err := Decode(withUnknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Decode(unknown) error = %v", err)
	}
	if _, err := Decode(append(raw, []byte(` {}`)...)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("Decode(trailing) error = %v", err)
	}
}

func TestLoadAutoAndRevisionPin(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "nodes.json")
	data := mustManifestJSON(t, testManifest("cached-r2", time.Now().Add(time.Hour)))
	if err := os.WriteFile(cache, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(LoadOptions{Source: SourceAuto, CachePath: cache, Revision: "cached-r2"})
	if err != nil {
		t.Fatalf("Load(auto) error = %v", err)
	}
	if loaded.Manifest.Revision != "cached-r2" || loaded.Source != SourceAuto || loaded.Path != cache {
		t.Fatalf("Load(auto) = %+v", loaded)
	}
	if _, err := Load(LoadOptions{Source: cache, Revision: "other"}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("Load(revision mismatch) error = %v", err)
	}
	explicit, err := Load(LoadOptions{Source: cache})
	if err != nil || explicit.Source != SourcePath || explicit.Path != cache {
		t.Fatalf("Load(path) = %+v, %v", explicit, err)
	}
	if err := os.WriteFile(cache, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	fallback, err := Load(LoadOptions{Source: SourceAuto, CachePath: cache})
	if err != nil {
		t.Fatalf("Load(auto fallback) error = %v", err)
	}
	if fallback.Source != SourceEmbedded || !strings.Contains(fallback.Warning, "ignored") {
		t.Fatalf("fallback = %+v", fallback)
	}
}

func TestSignedUpdateWritesAtomicPrivateCacheAndReportsExpiry(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testManifest("signed-r3", time.Now().Add(-time.Hour))
	manifest.GeneratedAt = time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	raw := mustManifestJSON(t, manifest)
	signature := ed25519.Sign(privateKey, raw)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nodes.json":
			_, _ = w.Write(raw)
		case "/nodes.sig":
			_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(signature)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "cache", "nodes.json")
	loaded, err := Update(context.Background(), UpdateOptions{
		ManifestURL:  server.URL + "/nodes.json",
		SignatureURL: server.URL + "/nodes.sig",
		PublicKey:    publicKey,
		Destination:  destination,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if loaded.Manifest.Revision != "signed-r3" || !strings.Contains(loaded.Warning, "expired") {
		t.Fatalf("Update() loaded = %+v", loaded)
	}
	written, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(raw) {
		t.Fatal("cached bytes differ from signed document")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("cache mode = %o, want 600", got)
		}
	}
}

func TestSignedUpdateRejectsTamperingWithoutReplacingCache(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw := mustManifestJSON(t, testManifest("tampered", time.Now().Add(time.Hour)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Update(context.Background(), UpdateOptions{
		ManifestURL: server.URL,
		Signature:   make([]byte, ed25519.SignatureSize),
		PublicKey:   publicKey,
		Destination: destination,
	})
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("Update(tampered) error = %v", err)
	}
	data, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "existing" {
		t.Fatalf("cache was replaced with %q", data)
	}
}

func TestCheckHealthUsesHEADWithoutDownloadingBody(t *testing.T) {
	var heads atomic.Int32
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gets.Add(1)
		_, _ = w.Write(make([]byte, 1<<20))
	}))
	defer server.Close()
	manifest := testManifest("health", time.Now().Add(time.Hour))
	manifest.Nodes[0].URL = server.URL + "/100MB.bin"
	manifest.Nodes[0].Endpoint = strings.TrimPrefix(server.URL, "http://")
	results := CheckHealth(context.Background(), manifest, HealthOptions{Timeout: time.Second})
	if len(results) != 1 || results[0].Status != "ok" || results[0].HTTPStatus != http.StatusMethodNotAllowed {
		t.Fatalf("CheckHealth() = %+v", results)
	}
	if results[0].LatencyMs <= 0 {
		t.Fatalf("latency_ms = %f, want > 0", results[0].LatencyMs)
	}
	if heads.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("requests HEAD=%d GET=%d", heads.Load(), gets.Load())
	}
}

func TestCheckHealthDNSAndTCP(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP listener unavailable: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Revision:      "tcp-health",
		GeneratedAt:   time.Now().Add(-time.Hour),
		ExpiresAt:     time.Now().Add(time.Hour),
		Nodes: []Node{{
			ID: "local-route", Name: "local", Kind: KindRoutePing, Region: "test", City: "local",
			Carrier: "test", IPFamily: "v4", Protocol: "tcp", Endpoint: "localhost", Port: port, Source: "test",
		}},
	}
	results := CheckHealth(context.Background(), manifest, HealthOptions{Timeout: time.Second})
	if len(results) != 1 || results[0].Status != "ok" || results[0].Method != "DNS+TCP" {
		t.Fatalf("CheckHealth() = %+v", results)
	}
}

func TestParsePublicKeyAndSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey := base64.StdEncoding.EncodeToString(publicKey)
	parsed, err := ParsePublicKey([]byte(encodedKey))
	if err != nil || !publicKey.Equal(parsed) {
		t.Fatalf("ParsePublicKey() = %x, %v", parsed, err)
	}
	document := []byte("catalog")
	signature := ed25519.Sign(privateKey, document)
	if err := Verify(document, []byte(base64.StdEncoding.EncodeToString(signature)), parsed); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestParseRawKeyAndSignaturePreservesWhitespaceBytes(t *testing.T) {
	key := make([]byte, ed25519.PublicKeySize)
	key[0] = ' '
	key[len(key)-1] = '\n'
	parsedKey, err := ParsePublicKey(key)
	if err != nil || string(parsedKey) != string(key) {
		t.Fatalf("ParsePublicKey(raw whitespace) = %x, %v", parsedKey, err)
	}
	signature := make([]byte, ed25519.SignatureSize)
	signature[0] = '\t'
	signature[len(signature)-1] = '\r'
	parsedSignature, err := ParseSignature(signature)
	if err != nil || string(parsedSignature) != string(signature) {
		t.Fatalf("ParseSignature(raw whitespace) = %x, %v", parsedSignature, err)
	}
}

func testManifest(revision string, expiresAt time.Time) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		Revision:      revision,
		GeneratedAt:   expiresAt.Add(-time.Hour).UTC().Truncate(time.Second),
		ExpiresAt:     expiresAt.UTC().Truncate(time.Second),
		Nodes: []Node{{
			ID: "download-test", Name: "Test download", Kind: KindDownload, Region: "test", City: "test",
			Carrier: "test", ASN: 64512, IPFamily: "v4", Protocol: "http", Endpoint: "example.com",
			Port: 80, URL: "http://example.com/100MB.bin", TrafficBytes: 100_000_000, Source: "test",
		}},
	}
}

func mustManifestJSON(t *testing.T, manifest Manifest) []byte {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("test manifest invalid: %v", err)
	}
	return data
}

func ExampleLoad_revisionPin() {
	loaded, err := Load(LoadOptions{Source: SourceEmbedded, Revision: "2026-07-13.1"})
	fmt.Println(err == nil, loaded.Manifest.Revision)
	// Output: true 2026-07-13.1
}
