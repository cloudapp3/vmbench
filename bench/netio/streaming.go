package netio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

// MediaServiceResult stores one media unlock result.
type MediaServiceResult struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Region  string `json:"region,omitempty"`
	Message string `json:"message,omitempty"`
}

// MediaSummary stores aggregate media counts.
type MediaSummary struct {
	Available int `json:"available,omitempty"`
	Blocked   int `json:"blocked,omitempty"`
	Unknown   int `json:"unknown,omitempty"`
}

// MediaResult stores structured media unlock results.
type MediaResult struct {
	Items   []MediaServiceResult `json:"items,omitempty"`
	Summary MediaSummary         `json:"summary"`
}

// ProbeMedia runs all built-in media unlock checks.
func ProbeMedia(ctx context.Context) (*MediaResult, error) {
	services := defaultServices()
	type indexed struct {
		idx  int
		item MediaServiceResult
	}
	ch := make(chan indexed, len(services))
	for i, svc := range services {
		go func(idx int, s streamingService) {
			svcCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			r := s.Check(svcCtx)
			ch <- indexed{idx: idx, item: MediaServiceResult{ID: s.ID, Title: s.Title, Status: r.Status, Region: r.Region, Message: r.Message}}
		}(i, svc)
	}
	result := &MediaResult{Items: make([]MediaServiceResult, len(services))}
	for range services {
		r := <-ch
		result.Items[r.idx] = r.item
	}
	for _, item := range result.Items {
		switch item.Status {
		case "available":
			result.Summary.Available++
		case "blocked":
			result.Summary.Blocked++
		default:
			result.Summary.Unknown++
		}
	}
	return result, nil
}

type streamingService struct {
	ID    string
	Title string
	Check func(ctx context.Context) streamingResult
}

type streamingResult struct {
	Status  string
	Region  string
	Message string
}

// streamingUnlockWorkload detects streaming service unlock status.
type streamingUnlockWorkload struct {
	detail  string
	count   int
	total   int
	elapsed time.Duration
}

// NewStreamingUnlockWorkload creates a streaming unlock detection benchmark.
func NewStreamingUnlockWorkload() bench.Workload {
	return &streamingUnlockWorkload{}
}

func (w *streamingUnlockWorkload) Name() string     { return "Net Streaming Unlock" }
func (w *streamingUnlockWorkload) Category() string { return bench.CategoryNetwork }
func (w *streamingUnlockWorkload) Description() string {
	return "Netflix / Disney+ / YouTube / ChatGPT / TikTok unlock detection"
}
func (w *streamingUnlockWorkload) Validate() error  { return nil }
func (w *streamingUnlockWorkload) SkipWarmup() bool { return true }
func (w *streamingUnlockWorkload) MaxIterations() int {
	return 1
}

func (w *streamingUnlockWorkload) Throughput(int64, time.Duration) (float64, string) {
	return float64(w.count), "unlocked"
}

func (w *streamingUnlockWorkload) Detail() string { return w.detail }

func (w *streamingUnlockWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.detail != "" {
		return w.elapsed, int64(w.count), nil
	}
	start := time.Now()
	result, err := ProbeMedia(ctx)
	w.elapsed = time.Since(start)
	if err != nil {
		return 0, 0, err
	}
	w.total = len(result.Items)
	w.count = result.Summary.Available
	parts := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		switch item.Status {
		case "available":
			if item.Region != "" {
				parts = append(parts, fmt.Sprintf("%s:%s", item.Title, item.Region))
			} else {
				parts = append(parts, fmt.Sprintf("%s:Yes", item.Title))
			}
		case "blocked":
			parts = append(parts, fmt.Sprintf("%s:No", item.Title))
		default:
			parts = append(parts, fmt.Sprintf("%s:?", item.Title))
		}
	}
	w.detail = strings.Join(parts, " | ")
	return w.elapsed, int64(w.count), nil
}

func defaultServices() []streamingService {
	return []streamingService{
		{ID: "netflix", Title: "Netflix", Check: checkNetflix},
		{ID: "youtube", Title: "YouTube Premium", Check: checkYouTube},
		{ID: "disney_plus", Title: "Disney+", Check: checkDisneyPlus},
		{ID: "chatgpt", Title: "ChatGPT", Check: checkChatGPT},
		{ID: "tiktok", Title: "TikTok", Check: checkTikTok},
		{ID: "prime", Title: "Prime Video", Check: checkPrimeVideo},
	}
}

var ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func httpGet(ctx context.Context, url string) (string, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), resp.Header, nil
}

func checkNetflix(ctx context.Context) streamingResult {
	body, _, err := httpGet(ctx, "https://www.netflix.com/title/81280792")
	if err != nil {
		return streamingResult{Status: "unknown", Message: err.Error()}
	}
	if strings.Contains(body, "not available") || strings.Contains(body, "Missing") {
		return streamingResult{Status: "blocked", Message: "geo-blocked"}
	}
	if strings.Contains(body, "page-title") || strings.Contains(body, "title") {
		return streamingResult{Status: "available", Region: detectNetflixRegion(body)}
	}
	return streamingResult{Status: "unknown", Message: "unexpected response"}
}

func detectNetflixRegion(body string) string {
	re := regexp.MustCompile(`"currentCountry":"([A-Z]+)"`)
	m := re.FindStringSubmatch(body)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func checkYouTube(ctx context.Context) streamingResult {
	body, _, err := httpGet(ctx, "https://www.youtube.com/premium")
	if err != nil {
		return streamingResult{Status: "unknown", Message: err.Error()}
	}
	if strings.Contains(body, "not available in your country") {
		return streamingResult{Status: "blocked", Message: "not available"}
	}
	re := regexp.MustCompile(`"GL"\s*:\s*"([A-Z]+)"`)
	m := re.FindStringSubmatch(body)
	if len(m) > 1 {
		return streamingResult{Status: "available", Region: m[1]}
	}
	if strings.Contains(body, "Premium") {
		return streamingResult{Status: "available"}
	}
	return streamingResult{Status: "unknown"}
}

func checkDisneyPlus(ctx context.Context) streamingResult {
	_, headers, err := httpGet(ctx, "https://www.disneyplus.com")
	if err != nil {
		return streamingResult{Status: "unknown", Message: err.Error()}
	}
	region := headers.Get("X-Region")
	if region == "" {
		region = headers.Get("Region")
	}
	loc := headers.Get("Location")
	if strings.Contains(loc, "unavailable") || strings.Contains(loc, "preview.disneyplus.com/unavailable") {
		return streamingResult{Status: "blocked", Message: "redirected to unavailable"}
	}
	if region != "" {
		return streamingResult{Status: "available", Region: region}
	}
	return streamingResult{Status: "available"}
}

func checkChatGPT(ctx context.Context) streamingResult {
	body, _, err := httpGet(ctx, "https://chatgpt.com/cdn-cgi/trace")
	if err != nil {
		return streamingResult{Status: "unknown", Message: err.Error()}
	}
	loc := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "loc=") {
			loc = strings.TrimPrefix(line, "loc=")
		}
	}
	if strings.Contains(body, "h=chatgpt.com") {
		if loc != "" {
			return streamingResult{Status: "available", Region: loc}
		}
		return streamingResult{Status: "available"}
	}
	return streamingResult{Status: "blocked", Message: "access denied"}
}

func checkTikTok(ctx context.Context) streamingResult {
	body, _, err := httpGet(ctx, "https://www.tiktok.com/")
	if err != nil {
		return streamingResult{Status: "unknown", Message: err.Error()}
	}
	re := regexp.MustCompile(`data-region="([A-Z]+)"`)
	m := re.FindStringSubmatch(body)
	if len(m) > 1 {
		return streamingResult{Status: "available", Region: m[1]}
	}
	if strings.Contains(body, "tiktok") {
		return streamingResult{Status: "available"}
	}
	return streamingResult{Status: "unknown"}
}

func checkPrimeVideo(ctx context.Context) streamingResult {
	body, headers, err := httpGet(ctx, "https://www.primevideo.com")
	if err != nil {
		return streamingResult{Status: "unknown", Message: err.Error()}
	}
	region := headers.Get("Content-Language")
	if strings.Contains(body, "prime") || strings.Contains(body, "video") {
		if region != "" {
			return streamingResult{Status: "available", Region: strings.ToUpper(region)}
		}
		return streamingResult{Status: "available"}
	}
	return streamingResult{Status: "blocked"}
}
