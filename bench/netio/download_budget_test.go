package netio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type downloadRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn downloadRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestProbeDownloadEnforcesCatalogTrafficBudget(t *testing.T) {
	const budget = int64(1024)
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: downloadRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Accept-Encoding"); got != "identity" {
			t.Errorf("Accept-Encoding = %q, want identity", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(make([]byte, 4096))),
			Request:    r,
		}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = originalClient })

	result, err := ProbeDownload(context.Background(), SpeedNode{
		Name:         "budgeted",
		TestURL:      "http://download.example/test.bin",
		TrafficBytes: budget,
	})
	if err != nil {
		t.Fatalf("ProbeDownload() error = %v", err)
	}
	if result.Bytes != budget {
		t.Fatalf("ProbeDownload() bytes = %d, want budget %d", result.Bytes, budget)
	}
}
