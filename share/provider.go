package share

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider names accepted by --provider.
const (
	ProviderDpaste   = "dpaste"
	ProviderZerox0   = "0x0"
	ProviderPasteRS  = "paste_rs"
	ProviderCustom   = "custom"
	DefaultProviders = ProviderDpaste
)

// uploadTimeout bounds a single provider attempt. There is no in-provider
// retry: a duplicate paste is worse than a failed one, so only the fallback
// chain tries again (against a different provider).
const uploadTimeout = 30 * time.Second

// maxResponseBytes caps how much of a provider response is read.
const maxResponseBytes = 1024 * 1024

// uploadFilename is the filename used by multipart providers. Paste services
// keep no metadata beyond it; the payload itself carries the provenance.
const uploadFilename = "vmbench-report.txt"

// providerSpec describes one paste provider adapter.
type providerSpec struct {
	// endpoint is the default upload URL; "custom" takes it from options.
	endpoint string
	// upload performs one attempt and returns the paste URL.
	upload func(ctx context.Context, client *http.Client, endpoint, userAgent string, payload []byte) (string, error)
}

var providerSpecs = map[string]providerSpec{
	ProviderDpaste:  {endpoint: "https://dpaste.org/api/", upload: uploadDpaste},
	ProviderZerox0:  {endpoint: "https://0x0.st", upload: uploadMultipart},
	ProviderPasteRS: {endpoint: "https://paste.rs", upload: uploadRawBodyURL},
	ProviderCustom:  {endpoint: "", upload: uploadRawBodyURL},
}

// ParseProviders splits a comma-separated provider chain and validates every
// name plus the custom endpoint rule before anything is uploaded.
func ParseProviders(value, customEndpoint string) ([]string, error) {
	names := []string{}
	for _, field := range strings.Split(value, ",") {
		name := strings.ToLower(strings.TrimSpace(field))
		if name == "" {
			continue
		}
		if _, ok := providerSpecs[name]; !ok {
			return nil, fmt.Errorf("invalid share provider %q (available: %s, %s, %s, %s)",
				name, ProviderDpaste, ProviderZerox0, ProviderPasteRS, ProviderCustom)
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		names = []string{DefaultProviders}
	}
	for _, name := range names {
		if name == ProviderCustom && strings.TrimSpace(customEndpoint) == "" {
			return nil, fmt.Errorf("share provider %q requires --share-endpoint", ProviderCustom)
		}
	}
	return names, nil
}

// UploadOptions configures one Upload call.
type UploadOptions struct {
	Providers []string     // ordered fallback chain; empty means dpaste
	Endpoint  string       // upload URL for the "custom" provider
	UserAgent string       // sent to every provider
	Client    *http.Client // injectable for tests; default has uploadTimeout
	// Endpoints overrides provider upload URLs (tests point providers at
	// httptest servers). The https-only rule still applies.
	Endpoints map[string]string
}

// Attempt records one failed provider try, for stderr reporting.
type Attempt struct {
	Provider string
	Err      error
}

// Upload sends the payload through the provider chain and returns the first
// successful paste URL. Fallback only happens on network errors and 5xx
// responses: any other failure (4xx, unparseable or oversized response,
// non-https endpoint) is terminal so a misbehaving provider cannot turn one
// requested upload into several. The payload is uploaded successfully at
// most once.
func Upload(ctx context.Context, payload []byte, opts UploadOptions) (string, []Attempt, error) {
	names := opts.Providers
	if len(names) == 0 {
		names = []string{DefaultProviders}
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: uploadTimeout}
	}
	userAgent := opts.UserAgent
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "vmbench"
	}

	attempts := []Attempt{}
	for _, name := range names {
		spec := providerSpecs[name]
		endpoint := strings.TrimSpace(opts.Endpoint)
		if override, ok := opts.Endpoints[name]; ok && strings.TrimSpace(override) != "" {
			endpoint = strings.TrimSpace(override)
		} else if endpoint == "" {
			endpoint = spec.endpoint
		}
		pasteURL, err := spec.upload(ctx, client, endpoint, userAgent, payload)
		if err == nil {
			return pasteURL, attempts, nil
		}
		attempts = append(attempts, Attempt{Provider: name, Err: err})
		if !fallbackable(err) {
			return "", attempts, err
		}
	}
	if len(attempts) == 1 {
		return "", attempts, attempts[0].Err
	}
	return "", attempts, fmt.Errorf("all %d share providers failed", len(attempts))
}

// fallbackable reports whether moving on to the next provider makes sense.
func fallbackable(err error) bool {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		return httpErr.Status >= 500
	}
	// Network errors (dial, timeout, TLS) are *url.Error; everything else
	// (bad status handling aside) is treated as terminal.
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// httpError marks a non-2xx provider response.
type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("share: provider returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("share: provider returned HTTP %d: %s", e.Status, e.Body)
}

// uploadDpaste posts to the dpaste.org API and reads the JSON response.
func uploadDpaste(ctx context.Context, client *http.Client, endpoint, userAgent string, payload []byte) (string, error) {
	form := url.Values{}
	form.Set("content", string(payload))
	form.Set("lexer", "text")
	form.Set("format", "json")
	form.Set("expiry_days", "365")
	req, err := newUploadRequest(ctx, http.MethodPost, endpoint, userAgent, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	var decoded struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("share: decoding dpaste response: %w", err)
	}
	return parsePasteURL(decoded.URL)
}

// uploadRawBodyURL posts the payload as the whole request body (paste.rs and
// custom endpoints) and reads the paste URL from the response body.
func uploadRawBodyURL(ctx context.Context, client *http.Client, endpoint, userAgent string, payload []byte) (string, error) {
	req, err := newUploadRequest(ctx, http.MethodPost, endpoint, userAgent, "text/plain; charset=utf-8", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	return parsePasteURL(string(body))
}

// uploadMultipart posts the payload as a multipart file field.
func uploadMultipart(ctx context.Context, client *http.Client, endpoint, userAgent string, payload []byte) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", uploadFilename)
	if err != nil {
		return "", fmt.Errorf("share: building multipart form: %w", err)
	}
	if _, err := part.Write(payload); err != nil {
		return "", fmt.Errorf("share: building multipart form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("share: building multipart form: %w", err)
	}
	req, err := newUploadRequest(ctx, http.MethodPost, endpoint, userAgent, writer.FormDataContentType(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	return parsePasteURL(string(body))
}

// newUploadRequest builds the POST, enforcing the https-only rule for every
// provider including custom endpoints.
func newUploadRequest(ctx context.Context, method, endpoint, userAgent, contentType string, body io.Reader) (*http.Request, error) {
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, fmt.Errorf("share: provider endpoint must be https: %s", endpoint)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("share: building request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// readResponse consumes the response with the shared cap and turns any
// non-2xx status into an httpError.
func readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("share: reading provider response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("share: provider response exceeds %d bytes", int64(maxResponseBytes))
	}
	return body, nil
}

// parsePasteURL validates that the provider answered with a URL and returns
// its first line.
func parsePasteURL(body string) (string, error) {
	line := strings.TrimSpace(body)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if line == "" {
		return "", fmt.Errorf("share: provider returned an empty paste URL")
	}
	parsed, err := url.Parse(line)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("share: provider response is not a paste URL: %s", truncateText(line, 120))
	}
	return line, nil
}

// truncateText shortens provider evidence in error messages.
func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
