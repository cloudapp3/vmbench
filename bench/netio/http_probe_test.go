package netio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeUploadRejectsHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	restoreURL, restoreSize := cfUploadURL, uploadSize
	cfUploadURL = server.URL
	uploadSize = 1024
	defer func() {
		cfUploadURL = restoreURL
		uploadSize = restoreSize
	}()

	_, err := ProbeUpload(context.Background())
	if err == nil {
		t.Fatal("ProbeUpload() error = nil, want HTTP status error")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("ProbeUpload() error = %q, want HTTP 403", err.Error())
	}
}

func TestProbeUploadSuccessUsesConfiguredSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.ContentLength != int64(uploadSize) {
			t.Errorf("ContentLength = %d, want %d", r.ContentLength, uploadSize)
		}
		if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("Content-Type = %q, want application/octet-stream", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upload body: %v", err)
		}
		if len(body) != uploadSize {
			t.Errorf("body size = %d, want %d", len(body), uploadSize)
		}
		for i, value := range body {
			if value != 0 {
				t.Errorf("body[%d] = %d, want zero-filled stream", i, value)
				break
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	restoreURL, restoreSize := cfUploadURL, uploadSize
	cfUploadURL = server.URL
	uploadSize = 2048
	defer func() {
		cfUploadURL = restoreURL
		uploadSize = restoreSize
	}()

	result, err := ProbeUpload(context.Background())
	if err != nil {
		t.Fatalf("ProbeUpload() error = %v", err)
	}
	if result.Bytes != int64(uploadSize) {
		t.Fatalf("Bytes = %d, want %d", result.Bytes, uploadSize)
	}
	if result.Elapsed <= 0 {
		t.Fatalf("Elapsed = %s, want > 0", result.Elapsed)
	}
}

func TestProbeMultiDownloadRejectsHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	restoreURL := cfDownloadURL
	cfDownloadURL = server.URL
	defer func() { cfDownloadURL = restoreURL }()

	_, err := ProbeMultiDownload(context.Background())
	if err == nil {
		t.Fatal("ProbeMultiDownload() error = nil, want HTTP status error")
	}
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("ProbeMultiDownload() error = %q, want HTTP 502", err.Error())
	}
}

func TestProbeMultiDownloadAggregatesSuccessfulWorkers(t *testing.T) {
	payload := []byte("0123456789abcdef")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	restoreURL := cfDownloadURL
	cfDownloadURL = server.URL
	defer func() { cfDownloadURL = restoreURL }()

	result, err := ProbeMultiDownload(context.Background())
	if err != nil {
		t.Fatalf("ProbeMultiDownload() error = %v", err)
	}
	wantBytes := int64(len(payload) * multiThreads)
	if result.TotalBytes != wantBytes {
		t.Fatalf("TotalBytes = %d, want %d", result.TotalBytes, wantBytes)
	}
	if result.Threads != multiThreads {
		t.Fatalf("Threads = %d, want %d", result.Threads, multiThreads)
	}
	if result.Elapsed <= 0 {
		t.Fatalf("Elapsed = %s, want > 0", result.Elapsed)
	}
	if got := result.ThroughputMiBPerSec(); got <= 0 {
		t.Fatalf("ThroughputMiBPerSec() = %f, want > 0", got)
	}
}

func TestProbeMultiDownloadReportsCopyErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "16")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	restoreURL := cfDownloadURL
	cfDownloadURL = server.URL
	defer func() { cfDownloadURL = restoreURL }()

	_, err := ProbeMultiDownload(context.Background())
	if err == nil {
		t.Fatal("ProbeMultiDownload() error = nil, want copy error")
	}
	if !strings.Contains(fmt.Sprint(err), "unexpected EOF") {
		t.Fatalf("ProbeMultiDownload() error = %q, want unexpected EOF", err.Error())
	}
}
