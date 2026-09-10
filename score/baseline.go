package score

import (
	"fmt"
	"math"
	"sort"
)

// Canonical dimension and profile display order. Maps in the baseline JSON are
// unordered, so Evaluate sorts by these lists (extras appended alphabetically)
// to keep the output byte-identical for identical input.
var (
	dimensionOrder = []string{"cpu", "memory", "disk", "network", "stability"}
	profileOrder   = []string{"web", "build", "proxy", "storage"}
)

// GradeCutoffs stores the minimum index for each rating letter.
type GradeCutoffs struct {
	S float64 `json:"s"`
	A float64 `json:"a"`
	B float64 `json:"b"`
	C float64 `json:"c"`
}

// MetricSource points at the report location a metric is read from.
type MetricSource struct {
	// Workload is the exact workload name in a report, empty for sources that
	// are derived from report sections or samples instead of one workload row.
	Workload      string `json:"workload,omitempty"`
	Field         string `json:"field"`
	FallbackField string `json:"fallback_field,omitempty"`
}

// BandThresholds stores the fixed anchors of a lower-is-better metric band.
type BandThresholds struct {
	Excellent float64 `json:"excellent"`
	Good      float64 `json:"good"`
	Fair      float64 `json:"fair"`
	Cutoff    float64 `json:"cutoff"`
}

// MetricBaseline describes one scored metric and its normalization anchors.
type MetricBaseline struct {
	Dimension    string          `json:"dimension"`
	Weight       float64         `json:"weight"`
	Source       MetricSource    `json:"source"`
	Units        []string        `json:"units,omitempty"`
	Class        string          `json:"class"` // "log" or "band"
	Floor        float64         `json:"floor,omitempty"`
	Ceiling      float64         `json:"ceiling,omitempty"`
	Bands        *BandThresholds `json:"bands,omitempty"`
	RequiresTool string          `json:"requires_tool,omitempty"`
	RequiresKind string          `json:"requires_kind,omitempty"` // "any", "run", or "checkup"
	Platform     string          `json:"platform,omitempty"`      // empty = all platforms
	Optional     bool            `json:"optional,omitempty"`      // absence never lowers coverage
	VetoOnly     bool            `json:"veto_only,omitempty"`     // evidence for vetoes, never scored
	FallbackFor  string          `json:"fallback_for,omitempty"`  // substitutes this metric when absent
}

// DimensionBaseline stores the composite weight of one dimension.
type DimensionBaseline struct {
	Weight float64 `json:"weight"`
}

// VetoRule caps a profile rating when a metric crosses a hard limit.
type VetoRule struct {
	Metric string  `json:"metric"`
	Op     string  `json:"op"` // "gt" or "lt"
	Value  float64 `json:"value"`
	Unit   string  `json:"unit,omitempty"`
	Cap    string  `json:"cap,omitempty"` // rating letter, defaults to "C"
}

// ProfileBaseline stores one scenario profile reweighting and its vetoes.
type ProfileBaseline struct {
	Weights map[string]float64 `json:"weights"`
	Vetoes  []VetoRule         `json:"vetoes,omitempty"`
}

// BaselineSet is a versioned scoring baseline collection.
type BaselineSet struct {
	SchemaVersion int                          `json:"schema_version"`
	Revision      string                       `json:"revision"`
	Grades        GradeCutoffs                 `json:"grades"`
	Metrics       map[string]MetricBaseline    `json:"metrics"`
	Dimensions    map[string]DimensionBaseline `json:"dimensions"`
	Profiles      map[string]ProfileBaseline   `json:"profiles"`
}

// Validate enforces the structural invariants of a baseline set.
func (b *BaselineSet) Validate() error {
	if b.SchemaVersion != 1 {
		return fmt.Errorf("score: unsupported baseline schema_version %d", b.SchemaVersion)
	}
	if b.Revision == "" {
		return fmt.Errorf("score: baseline revision is required")
	}
	if len(b.Metrics) == 0 || len(b.Dimensions) == 0 {
		return fmt.Errorf("score: baseline needs metrics and dimensions")
	}
	for id, metric := range b.Metrics {
		if _, ok := b.Dimensions[metric.Dimension]; !ok {
			return fmt.Errorf("score: metric %q references unknown dimension %q", id, metric.Dimension)
		}
		if metric.Weight <= 0 && !metric.VetoOnly {
			return fmt.Errorf("score: metric %q has non-positive weight", id)
		}
		switch metric.Class {
		case "log":
			if metric.Floor <= 0 || metric.Ceiling <= metric.Floor {
				return fmt.Errorf("score: metric %q needs 0 < floor < ceiling", id)
			}
		case "band":
			if metric.Bands == nil ||
				!(metric.Bands.Excellent < metric.Bands.Good &&
					metric.Bands.Good < metric.Bands.Fair &&
					metric.Bands.Fair < metric.Bands.Cutoff) {
				return fmt.Errorf("score: metric %q needs excellent < good < fair < cutoff", id)
			}
		default:
			return fmt.Errorf("score: metric %q has unknown class %q", id, metric.Class)
		}
		switch metric.RequiresKind {
		case "", "any", "run", "checkup":
		default:
			return fmt.Errorf("score: metric %q has unknown requires_kind %q", id, metric.RequiresKind)
		}
		if metric.FallbackFor != "" {
			if _, ok := b.Metrics[metric.FallbackFor]; !ok {
				return fmt.Errorf("score: metric %q falls back for unknown metric %q", id, metric.FallbackFor)
			}
		}
	}
	dimensionWeights := 0.0
	for id, dimension := range b.Dimensions {
		if dimension.Weight <= 0 || dimension.Weight > 1 {
			return fmt.Errorf("score: dimension %q weight %v out of range", id, dimension.Weight)
		}
		dimensionWeights += dimension.Weight
	}
	if math.Abs(dimensionWeights-1) > 1e-9 {
		return fmt.Errorf("score: dimension weights sum to %v, want 1", dimensionWeights)
	}
	for id, profile := range b.Profiles {
		weights := 0.0
		for dimension, weight := range profile.Weights {
			if _, ok := b.Dimensions[dimension]; !ok {
				return fmt.Errorf("score: profile %q references unknown dimension %q", id, dimension)
			}
			weights += weight
		}
		if math.Abs(weights-1) > 1e-9 {
			return fmt.Errorf("score: profile %q weights sum to %v, want 1", id, weights)
		}
		for _, veto := range profile.Vetoes {
			if _, ok := b.Metrics[veto.Metric]; !ok {
				return fmt.Errorf("score: profile %q veto references unknown metric %q", id, veto.Metric)
			}
			switch veto.Op {
			case "gt", "lt":
			default:
				return fmt.Errorf("score: profile %q veto has unknown op %q", id, veto.Op)
			}
		}
	}
	return nil
}

// sortedDimensionIDs returns baseline dimension ids in canonical display order.
func (b *BaselineSet) sortedDimensionIDs() []string {
	return sortedByCanonicalOrder(b.Dimensions, dimensionOrder)
}

// sortedProfileIDs returns baseline profile ids in canonical display order.
func (b *BaselineSet) sortedProfileIDs() []string {
	return sortedByCanonicalOrder(b.Profiles, profileOrder)
}

// metricIDsForDimension returns the metric ids of one dimension, sorted.
func (b *BaselineSet) metricIDsForDimension(dimension string) []string {
	ids := make([]string, 0)
	for id, metric := range b.Metrics {
		if metric.Dimension == dimension {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func sortedByCanonicalOrder[T any](values map[string]T, order []string) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	rank := make(map[string]int, len(order))
	for i, id := range order {
		rank[id] = i
	}
	sort.Slice(ids, func(i, j int) bool {
		ri, oki := rank[ids[i]]
		rj, okj := rank[ids[j]]
		if oki != okj {
			return oki
		}
		if oki && okj && ri != rj {
			return ri < rj
		}
		return ids[i] < ids[j]
	})
	return ids
}
