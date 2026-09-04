package netio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/oneclickvirt/UnlockTests/executor"
	"github.com/oneclickvirt/UnlockTests/model"
)

// MediaServiceResult stores one media unlock result.
type MediaServiceResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	RawStatus  string `json:"raw_status,omitempty"`
	Region     string `json:"region,omitempty"`
	UnlockType string `json:"unlock_type,omitempty"`
	IPVersion  string `json:"ip_version,omitempty"`
	Message    string `json:"message,omitempty"`
}

// MediaSummary stores aggregate media counts.
type MediaSummary struct {
	Available  int `json:"available,omitempty"`
	Restricted int `json:"restricted,omitempty"`
	Blocked    int `json:"blocked,omitempty"`
	Unknown    int `json:"unknown,omitempty"`
}

// MediaResult stores structured media unlock results.
type MediaResult struct {
	Items     []MediaServiceResult `json:"items,omitempty"`
	Summary   MediaSummary         `json:"summary"`
	Set       string               `json:"set,omitempty"`
	IPVersion string               `json:"ip_version,omitempty"`
}

// MediaProbeOptions controls the UnlockTests run.
type MediaProbeOptions struct {
	// Set selects the region/platform set: all, globe, tw, hk, jp, kr, na,
	// sa, eu, afr, sea, oce, ai, or a comma-separated combination.
	Set string
	// IPVersion uses the suite convention v4/v6/dual and defaults to dual.
	IPVersion string
}

// DefaultMediaProbeOptions returns the full-platform, dual-stack defaults.
func DefaultMediaProbeOptions() MediaProbeOptions {
	return MediaProbeOptions{Set: "all", IPVersion: "dual"}
}

// mediaSetID normalizes a user-provided set into the canonical display form.
func mediaSetID(set string) string {
	s := strings.ToLower(strings.TrimSpace(set))
	if s == "" {
		return "all"
	}
	return s
}

// ValidateMediaSet reports whether a media set can be resolved.
func ValidateMediaSet(set string) error {
	_, err := executor.ParseRegionSelection(mediaSetID(set))
	return err
}

// normalizeMediaIPVersion maps suite IP versions onto UnlockTests values.
func normalizeMediaIPVersion(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "v4", "ipv4", "4":
		return "ipv4"
	case "v6", "ipv6", "6":
		return "ipv6"
	default:
		return "auto"
	}
}

var mediaIDSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func mediaIDFromName(name string) string {
	id := mediaIDSanitizer.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "_")
	return strings.Trim(id, "_")
}

// mapUnlockStatus converts UnlockTests statuses into the shared
// available/blocked/unknown vocabulary. RawStatus keeps the source value.
func mapUnlockStatus(raw string) string {
	switch raw {
	case model.StatusYes, model.StatusCDNRelay, model.StatusRestricted:
		return "available"
	case model.StatusNo, model.StatusBanned:
		return "blocked"
	default:
		// Unknown, Error, NetworkError, RateLimited, Timeout,
		// NoIPv6Support, DNSResolveFailed and future values stay inconclusive.
		return "unknown"
	}
}

// ProbeMedia runs media unlock checks through the UnlockTests library.
// A missing IP stack is not an error: per-item statuses carry the evidence.
func ProbeMedia(ctx context.Context, opts MediaProbeOptions) (*MediaResult, error) {
	set := mediaSetID(opts.Set)
	selection, err := executor.ParseRegionSelection(set)
	if err != nil {
		return nil, err
	}
	ipVersion := normalizeMediaIPVersion(opts.IPVersion)
	results, runErr := executor.RunStructured(ctx, executor.RunOptions{
		Selection:   selection,
		IPVersion:   ipVersion,
		Concurrency: executor.DefaultStructuredConcurrency,
	})
	if len(results) == 0 {
		if runErr != nil {
			return nil, runErr
		}
		return nil, fmt.Errorf("media unlock set %q produced no results", set)
	}
	result := &MediaResult{Set: set, IPVersion: ipVersion}
	result.Items = make([]MediaServiceResult, 0, len(results))
	for _, r := range results {
		item := MediaServiceResult{
			ID:         mediaIDFromName(r.Name),
			Title:      r.Name,
			Status:     mapUnlockStatus(r.Status),
			RawStatus:  r.Status,
			Region:     r.Region,
			UnlockType: r.UnlockType,
			IPVersion:  r.IPVersion,
		}
		if item.ID == "" {
			item.ID = "unknown"
		}
		switch {
		case r.Error != "":
			item.Message = r.Error
		case r.Info != "":
			item.Message = r.Info
		}
		switch item.Status {
		case "available":
			result.Summary.Available++
			if r.Status == model.StatusRestricted {
				result.Summary.Restricted++
			}
		case "blocked":
			result.Summary.Blocked++
		default:
			result.Summary.Unknown++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
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
	return "UnlockTests streaming / AI platform unlock detection"
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

// mediaDetailLimit caps the per-item detail line so a full-platform run does
// not produce a multi-kilobyte detail field.
const mediaDetailLimit = 60

func (w *streamingUnlockWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.detail != "" {
		return w.elapsed, int64(w.count), nil
	}
	start := time.Now()
	result, err := ProbeMedia(ctx, DefaultMediaProbeOptions())
	w.elapsed = time.Since(start)
	if err != nil {
		return 0, 0, err
	}
	w.total = len(result.Items)
	w.count = result.Summary.Available
	parts := make([]string, 0, mediaDetailLimit+1)
	for _, item := range result.Items {
		if len(parts) >= mediaDetailLimit {
			parts = append(parts, fmt.Sprintf("... +%d more", w.total-mediaDetailLimit))
			break
		}
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
	var sb strings.Builder
	fmt.Fprintf(&sb, "set %s · available %d · restricted %d · blocked %d · unknown %d",
		result.Set, result.Summary.Available, result.Summary.Restricted, result.Summary.Blocked, result.Summary.Unknown)
	if len(parts) > 0 {
		sb.WriteString(" | ")
		sb.WriteString(strings.Join(parts, " | "))
	}
	w.detail = sb.String()
	return w.elapsed, int64(w.count), nil
}
