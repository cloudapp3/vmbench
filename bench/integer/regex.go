package integer

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultRegexLines = 100_000

// RegexWorkload benchmarks complex regular expression matching over synthetic logs.
type RegexWorkload struct {
	pattern       *regexp.Regexp
	lines         []string
	expectedCount int
	lastCount     int
}

// NewRegexWorkload returns the default regex workload.
func NewRegexWorkload() bench.Workload {
	return newRegexWorkload(defaultRegexLines)
}

func newRegexWorkload(lines int) *RegexWorkload {
	pattern := regexp.MustCompile(`(?i)^(GET|POST|PUT) /api/v\d+/(items|orders|users)(/[a-z0-9-]+)? status=(200|201|204|404) trace=[a-f0-9]{16}$`)
	out := make([]string, 0, lines)
	matches := 0
	for idx := 0; idx < lines; idx++ {
		var line string
		if idx%3 == 0 {
			line = fmt.Sprintf("GET /api/v1/items/%d status=200 trace=%016x", idx, idx*17+3)
		} else if idx%5 == 0 {
			line = fmt.Sprintf("POST /api/v2/orders/%d status=201 trace=%016x", idx, idx*29+7)
		} else {
			line = fmt.Sprintf("DEBUG worker=%d path=/health status=500 trace=%x", idx, idx)
		}
		if pattern.MatchString(strings.TrimSpace(line)) {
			matches++
		}
		out = append(out, line)
	}
	return &RegexWorkload{pattern: pattern, lines: out, expectedCount: matches}
}

func (w *RegexWorkload) Name() string { return "Regex" }

func (w *RegexWorkload) Category() string { return bench.CategoryInteger }

func (w *RegexWorkload) Description() string {
	return "Matches 100k synthetic log lines against a complex API request regular expression."
}

func (*RegexWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *RegexWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastCount = 0
	return &cp
}

func (w *RegexWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	count := 0
	started := time.Now()
	for idx, line := range w.lines {
		if idx%2048 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		if w.pattern.MatchString(line) {
			count++
		}
	}
	w.lastCount = count
	common.ConsumeUint64(uint64(count))
	return time.Since(started), int64(len(w.lines)), nil
}

func (w *RegexWorkload) Validate() error {
	if w.lastCount != w.expectedCount {
		return errors.New("regex validation failed")
	}
	return nil
}

func (w *RegexWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "lines/s"
	}
	return float64(processed) / elapsed.Seconds(), "lines/s"
}
