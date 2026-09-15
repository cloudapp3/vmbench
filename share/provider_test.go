package share

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// newTLSServer starts an httptest TLS server plus a client that trusts it.
// Upload enforces https endpoints, so every fake provider runs over TLS.
func newTLSServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	return srv, srv.Client()
}

func TestUploadDpastePostsFormAndParsesJSON(t *testing.T) {
	var got struct{ method, ua, contentType, content, lexer, format, expiry string }
	srv, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.ua = r.Header.Get("User-Agent")
		got.contentType = r.Header.Get("Content-Type")
		got.content = r.PostFormValue("content")
		got.lexer = r.PostFormValue("lexer")
		got.format = r.PostFormValue("format")
		got.expiry = r.PostFormValue("expiry_days")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"url":"https://example.com/p/abc1"}`)
	})
	pasteURL, attempts, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderDpaste},
		UserAgent: "vmbench/test",
		Client:    client,
		Endpoints: map[string]string{ProviderDpaste: srv.URL},
	})
	if err != nil || len(attempts) != 0 {
		t.Fatalf("Upload = %q, %v, %v", pasteURL, attempts, err)
	}
	if pasteURL != "https://example.com/p/abc1" {
		t.Fatalf("pasteURL = %q", pasteURL)
	}
	if got.method != http.MethodPost {
		t.Errorf("method = %s, want POST", got.method)
	}
	if got.ua != "vmbench/test" {
		t.Errorf("User-Agent = %q", got.ua)
	}
	if !strings.Contains(got.contentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q", got.contentType)
	}
	for field, want := range map[string]string{
		"content": "payload", "lexer": "text", "format": "json", "expiry_days": "365",
	} {
		if got := map[string]string{"content": got.content, "lexer": got.lexer, "format": got.format, "expiry_days": got.expiry}[field]; got != want {
			t.Errorf("form field %s = %q, want %q", field, got, want)
		}
	}
}

func TestUploadZerox0PostsMultipartFile(t *testing.T) {
	var gotFilename, gotBody, gotContentType string
	srv, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("missing multipart file field: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		defer file.Close()
		gotFilename = header.Filename
		body, _ := io.ReadAll(file)
		gotBody = string(body)
		io.WriteString(w, "https://example.com/f/1\n")
	})
	pasteURL, _, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderZerox0},
		Client:    client,
		Endpoints: map[string]string{ProviderZerox0: srv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pasteURL != "https://example.com/f/1" {
		t.Fatalf("pasteURL = %q", pasteURL)
	}
	if gotFilename != uploadFilename {
		t.Errorf("filename = %q, want %q", gotFilename, uploadFilename)
	}
	if gotBody != "payload" {
		t.Errorf("file body = %q, want payload verbatim", gotBody)
	}
	if !strings.Contains(gotContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q", gotContentType)
	}
}

func TestUploadRawBodyProvidersPostPayload(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider string
		useFlag  bool // custom takes the endpoint from UploadOptions.Endpoint
	}{
		{name: "paste_rs", provider: ProviderPasteRS},
		{name: "custom", provider: ProviderCustom, useFlag: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody, gotContentType string
			srv, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
				gotContentType = r.Header.Get("Content-Type")
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				io.WriteString(w, "https://example.com/raw/9")
			})
			opts := UploadOptions{
				Providers: []string{tt.provider},
				Client:    client,
				Endpoints: map[string]string{tt.provider: srv.URL},
			}
			if tt.useFlag {
				opts.Endpoint = srv.URL
			}
			pasteURL, _, err := Upload(context.Background(), []byte("payload"), opts)
			if err != nil {
				t.Fatal(err)
			}
			if pasteURL != "https://example.com/raw/9" {
				t.Fatalf("pasteURL = %q", pasteURL)
			}
			if gotBody != "payload" {
				t.Errorf("body = %q, want payload verbatim", gotBody)
			}
			if !strings.Contains(gotContentType, "text/plain") {
				t.Errorf("Content-Type = %q", gotContentType)
			}
		})
	}
}

func TestUploadFallsBackOn5xx(t *testing.T) {
	var secondHits atomic.Int32
	broken, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusBadGateway)
	})
	alive, _ := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		io.WriteString(w, "https://example.com/ok")
	})
	pasteURL, attempts, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderDpaste, ProviderPasteRS},
		Client:    client,
		Endpoints: map[string]string{ProviderDpaste: broken.URL, ProviderPasteRS: alive.URL},
	})
	if err != nil {
		t.Fatalf("fallback upload failed: %v (attempts %+v)", err, attempts)
	}
	if pasteURL != "https://example.com/ok" {
		t.Fatalf("pasteURL = %q", pasteURL)
	}
	if len(attempts) != 1 || attempts[0].Provider != ProviderDpaste {
		t.Fatalf("attempts = %+v, want one failed dpaste try", attempts)
	}
	if secondHits.Load() != 1 {
		t.Fatalf("second provider hits = %d, want 1", secondHits.Load())
	}
}

func TestUploadFallsBackOnNetworkError(t *testing.T) {
	// Reserve then release a port: dialing it is a genuine connection error.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadEndpoint := "https://" + listener.Addr().String()
	listener.Close()

	alive, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "https://example.com/ok")
	})
	pasteURL, attempts, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderDpaste, ProviderPasteRS},
		Client:    client,
		Endpoints: map[string]string{ProviderDpaste: deadEndpoint, ProviderPasteRS: alive.URL},
	})
	if err != nil {
		t.Fatalf("network fallback failed: %v (attempts %+v)", err, attempts)
	}
	if pasteURL != "https://example.com/ok" {
		t.Fatalf("pasteURL = %q", pasteURL)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %+v, want the dead endpoint recorded once", attempts)
	}
}

func TestUpload4xxIsTerminal(t *testing.T) {
	var secondHits atomic.Int32
	rejecting, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
	})
	alive, _ := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		io.WriteString(w, "https://example.com/ok")
	})
	pasteURL, attempts, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderDpaste, ProviderPasteRS},
		Client:    client,
		Endpoints: map[string]string{ProviderDpaste: rejecting.URL, ProviderPasteRS: alive.URL},
	})
	if err == nil {
		t.Fatalf("4xx upload unexpectedly succeeded: %q", pasteURL)
	}
	if !strings.Contains(err.Error(), "413") {
		t.Fatalf("error = %v, want the 4xx status surfaced", err)
	}
	if len(attempts) != 1 || secondHits.Load() != 0 {
		t.Fatalf("4xx must be terminal: attempts %+v, second provider hits %d", attempts, secondHits.Load())
	}
}

func TestUploadSuccessStopsChain(t *testing.T) {
	var secondHits atomic.Int32
	first, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"url":"https://example.com/first"}`)
	})
	second, _ := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		io.WriteString(w, "https://example.com/second")
	})
	pasteURL, _, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderDpaste, ProviderPasteRS},
		Client:    client,
		Endpoints: map[string]string{ProviderDpaste: first.URL, ProviderPasteRS: second.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pasteURL != "https://example.com/first" || secondHits.Load() != 0 {
		t.Fatalf("success must stop the chain: url %q, second hits %d", pasteURL, secondHits.Load())
	}
}

func TestUploadRejectsOversizedResponse(t *testing.T) {
	verbose, client := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(make([]byte, maxResponseBytes+1)); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	_, _, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderDpaste},
		Client:    client,
		Endpoints: map[string]string{ProviderDpaste: verbose.URL},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want the response-size guard", err)
	}
}

func TestUploadRejectsHTTPEndpoint(t *testing.T) {
	_, _, err := Upload(context.Background(), []byte("payload"), UploadOptions{
		Providers: []string{ProviderCustom},
		Endpoint:  "http://insecure.example/upload",
	})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("error = %v, want the https-only rule enforced", err)
	}
}

func TestParsePasteURL(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "plain", body: "https://example.com/p/1", want: "https://example.com/p/1"},
		{name: "first line with trailing noise", body: "https://example.com/p/1\nextra lines", want: "https://example.com/p/1"},
		{name: "surrounding whitespace", body: "  https://example.com/p/1  \n", want: "https://example.com/p/1"},
		{name: "empty", body: "   \n", wantErr: true},
		{name: "not a url", body: "paste accepted, thanks", wantErr: true},
		{name: "wrong scheme", body: "ftp://example.com/p/1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePasteURL(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsePasteURL(%q) = %q, want error", tt.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePasteURL(%q) error = %v", tt.body, err)
			}
			if got != tt.want {
				t.Fatalf("parsePasteURL(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestParseProviders(t *testing.T) {
	if got, err := ParseProviders("", ""); err != nil || len(got) != 1 || got[0] != DefaultProviders {
		t.Fatalf("ParseProviders(\"\") = %v, %v; want [dpaste]", got, err)
	}
	got, err := ParseProviders(" dpaste , 0X0 ,", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != ProviderDpaste || got[1] != ProviderZerox0 {
		t.Fatalf("ParseProviders chain = %v, want [dpaste 0x0]", got)
	}
	if _, err := ParseProviders("bogus", ""); err == nil || !strings.Contains(err.Error(), "dpaste") {
		t.Fatalf("unknown provider error = %v, want the available list", err)
	}
	if _, err := ParseProviders("custom", ""); err == nil || !strings.Contains(err.Error(), "share-endpoint") {
		t.Fatalf("custom without endpoint error = %v", err)
	}
	if _, err := ParseProviders("custom", "https://paste.internal/upload"); err != nil {
		t.Fatalf("custom with endpoint rejected: %v", err)
	}
	// custom mid-chain still validates the endpoint before any upload.
	if _, err := ParseProviders("dpaste,custom", ""); err == nil {
		t.Fatal("custom mid-chain without endpoint accepted")
	}
}

// TestFallbackableClassification pins the fallback contract at the unit level:
// only network errors and 5xx responses move on to the next provider.
func TestFallbackableClassification(t *testing.T) {
	networkErr := &url.Error{Op: "Post", URL: "https://paste.example/", Err: &net.OpError{Op: "dial", Err: errors.New("refused")}}
	if !fallbackable(networkErr) {
		t.Fatal("network error must be fallbackable")
	}
	if !fallbackable(&httpError{Status: 500, Body: "boom"}) || !fallbackable(&httpError{Status: 503}) {
		t.Fatal("5xx must be fallbackable")
	}
	if fallbackable(&httpError{Status: 404}) || fallbackable(&httpError{Status: 429}) {
		t.Fatal("4xx must be terminal")
	}
	if fallbackable(errors.New("share: decoding response")) {
		t.Fatal("unparseable response must be terminal")
	}
}
