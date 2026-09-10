// Package score computes a deterministic assessment from a vmbench report.
//
// Evaluate is a pure function: the same report bytes evaluated against the
// same baseline set produce byte-identical output. No network, clock, or
// filesystem access happens during evaluation, and no LLM ever participates
// in the scoring path. Assessment coverage fields disclose exactly which
// expected metrics were measured so sparse reports never pretend to be
// complete.
package score

import (
	"fmt"
	"sort"
	"strings"
)

// AssessmentSchemaVersion is the schema version of Assessment documents.
const AssessmentSchemaVersion = 1

// ReportKindAssessment is the report_kind value of Assessment documents.
const ReportKindAssessment = "assessment"

// DefaultGenerator labels assessments produced by the score CLI.
const DefaultGenerator = "vmbench score"

// Options tunes one Evaluate call. A nil Baseline selects the baseline set
// embedded in the binary.
type Options struct {
	Baseline        *BaselineSet
	BaselineSource  string // defaults to "embedded"
	RequireRevision string // fail closed when the baseline revision differs
	Generator       string
}

// Assessment is the deterministic evaluation result of one report.
type Assessment struct {
	SchemaVersion int               `json:"schema_version"`
	ReportKind    string            `json:"report_kind"`
	Generator     string            `json:"generator,omitempty"`
	Source        SourceRef         `json:"source"`
	Baseline      BaselineRef       `json:"baseline"`
	Composite     *CompositeResult  `json:"composite,omitempty"`
	Dimensions    []DimensionResult `json:"dimensions"`
	Profiles      []ProfileResult   `json:"profiles"`
	Coverage      CoverageSummary   `json:"coverage"`
	Warnings      []string          `json:"warnings,omitempty"`
}

// SourceRef identifies the evaluated report.
type SourceRef struct {
	Kind string `json:"kind"`
}

// BaselineRef identifies the baseline set used for evaluation.
type BaselineRef struct {
	Revision string `json:"revision"`
	Source   string `json:"source"`
}

// CompositeResult is the weighted composite index. It is nil when the report
// cannot support a meaningful composite (CPU absent or fewer than two
// performance dimensions measured).
type CompositeResult struct {
	Index    float64  `json:"index"`
	Rating   string   `json:"rating"`
	Status   string   `json:"status"` // "complete" or "partial"
	Basis    []string `json:"basis"`
	Excluded []string `json:"excluded,omitempty"`
}

// DimensionResult is one dimension's aggregated index.
type DimensionResult struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"` // "scored", "no_data", "not_applicable"
	Index       float64        `json:"index,omitempty"`
	Rating      string         `json:"rating,omitempty"`
	Confidence  float64        `json:"confidence,omitempty"`
	CoveragePct float64        `json:"coverage_pct,omitempty"`
	Metrics     []MetricResult `json:"metrics,omitempty"`
	Missing     []string       `json:"missing,omitempty"`
}

// Metric states. "absent" covers expected metrics with no reading; optional
// metrics that were not measured are "absent" without entering coverage.
const (
	MetricOK           = "ok"
	MetricAbsent       = "absent"
	MetricError        = "error"
	MetricUnitMismatch = "unit_mismatch"
)

// MetricResult is one metric's reading and normalized score.
type MetricResult struct {
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	Workload   string  `json:"workload,omitempty"`
	Value      float64 `json:"value,omitempty"`
	Unit       string  `json:"unit,omitempty"`
	Normalized float64 `json:"normalized,omitempty"`
	Weight     float64 `json:"weight,omitempty"`
	Class      string  `json:"class,omitempty"`
	Note       string  `json:"note,omitempty"`
}

// Dimension applicability states.
const (
	DimensionScored        = "scored"
	DimensionNoData        = "no_data"
	DimensionNotApplicable = "not_applicable"
)

// ProfileResult is one scenario profile's reweighted view.
type ProfileResult struct {
	ID      string   `json:"id"`
	Index   float64  `json:"index,omitempty"`
	Rating  string   `json:"rating,omitempty"`
	Fit     string   `json:"fit"`
	Veto    *VetoHit `json:"veto,omitempty"`
	Missing []string `json:"missing,omitempty"`
}

// Profile fit verdicts.
const (
	FitRecommended          = "recommended"
	FitConditional          = "conditional"
	FitUnsuitable           = "unsuitable"
	FitInsufficientEvidence = "insufficient_evidence"
)

// VetoHit records one triggered veto rule.
type VetoHit struct {
	Metric    string  `json:"metric"`
	Op        string  `json:"op"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Unit      string  `json:"unit,omitempty"`
	Cap       string  `json:"cap"`
}

// CoverageSummary discloses which expected metrics were actually measured.
type CoverageSummary struct {
	MetricPct         float64  `json:"metric_pct"`
	Present           []string `json:"present"`
	Missing           []string `json:"missing"`
	DimensionsPresent []string `json:"dimensions_present"`
	DimensionsMissing []string `json:"dimensions_missing"`
	Confidence        float64  `json:"confidence"`
}

// Composite withholding rules.
const (
	minPerformanceDimensions = 2
)

// Evaluate computes the assessment of one report document. data may be a run
// report, a checkup report, or a legacy suite report.
func Evaluate(data []byte, opts Options) (Assessment, error) {
	baseline := opts.Baseline
	if baseline == nil {
		embedded, err := EmbeddedBaseline()
		if err != nil {
			return Assessment{}, err
		}
		baseline = embedded
	}
	if opts.RequireRevision != "" && baseline.Revision != opts.RequireRevision {
		return Assessment{}, fmt.Errorf("score: baseline revision %q does not match required %q", baseline.Revision, opts.RequireRevision)
	}
	in, err := extractInput(data)
	if err != nil {
		return Assessment{}, err
	}

	evidence := extractEvidence(*baseline, in)
	assessment := Assessment{
		SchemaVersion: AssessmentSchemaVersion,
		ReportKind:    ReportKindAssessment,
		Generator:     opts.Generator,
		Source:        SourceRef{Kind: in.Kind},
		Baseline: BaselineRef{
			Revision: baseline.Revision,
			Source:   baselineSourceLabel(opts.BaselineSource),
		},
		Warnings: evidenceWarnings(evidence),
	}
	assessment.Dimensions = evaluateDimensions(*baseline, in, evidence)
	assessment.Composite = evaluateComposite(*baseline, assessment.Dimensions, &assessment.Warnings)
	assessment.Profiles = evaluateProfiles(*baseline, evidence)
	assessment.Coverage = summarizeCoverage(*baseline, evidence, assessment.Dimensions)
	assessment.Coverage.Confidence = percentToFraction(assessment.Coverage.MetricPct) * iterationFactor(in.Iterations)
	return assessment, nil
}

// metricEvidence is one metric's resolved state for this report.
type metricEvidence struct {
	Metric     MetricBaseline
	Applicable bool
	Present    bool
	Optional   bool
	Weighted   bool // weight > 0 and non-optional participation in coverage
	Consumed   bool // reading already substituted into its fallback target
	Value      float64
	Unit       string
	Normalized float64
	Status     string
	Note       string
}

// evidenceWarnings surfaces unit mismatches as top-level warnings so bad
// readings are visible even when the metric table is long.
func evidenceWarnings(evidence map[string]metricEvidence) []string {
	warnings := []string{}
	for _, id := range sortedMetricIDsFromEvidence(evidence) {
		state := evidence[id]
		if state.Status == MetricUnitMismatch {
			warnings = append(warnings, id+": "+state.Note)
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	return warnings
}

func sortedMetricIDsFromEvidence(evidence map[string]metricEvidence) []string {
	ids := make([]string, 0, len(evidence))
	for id := range evidence {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// extractEvidence resolves every baseline metric against the input once.
func extractEvidence(baseline BaselineSet, in input) map[string]metricEvidence {
	evidence := make(map[string]metricEvidence, len(baseline.Metrics))
	tools := toolSet(in.Tools)
	for _, id := range sortedMetricIDs(baseline.Metrics) {
		metric := baseline.Metrics[id]
		state := metricEvidence{
			Metric:     metric,
			Applicable: kindApplies(metric.RequiresKind, in.Kind) && platformApplies(metric.Platform, in.Platform),
			Optional:   metric.Optional,
			Status:     MetricAbsent,
		}
		if !state.Applicable {
			evidence[id] = state
			continue
		}
		if metric.RequiresTool != "" && len(tools) > 0 && !tools[metric.RequiresTool] {
			// The report did not select this tool, so the metric is out of
			// scope for this report even if stray rows exist.
			state.Applicable = false
			evidence[id] = state
			continue
		}
		value := in.resolveField(metric)
		switch {
		case value.Found:
			state.Present = true
			state.Status = MetricOK
			state.Value = value.Value
			state.Unit = value.Unit
			state.Note = value.Note
			state.Normalized = normalizeMetric(metric, value.Value)
		case strings.HasPrefix(value.Note, "workload error:"):
			state.Status = MetricError
			state.Note = value.Note
		case strings.HasPrefix(value.Note, "unit mismatch:"):
			state.Status = MetricUnitMismatch
			state.Note = value.Note
		}
		state.Weighted = metric.Weight > 0 && (state.Present || !metric.Optional)
		evidence[id] = state
	}
	applyFallbacks(baseline, evidence)
	return evidence
}

// applyFallbacks substitutes optional fallback readings for absent primaries.
// A consumed fallback keeps its report row visible but surrenders its own
// weight so one measurement is never counted twice.
func applyFallbacks(baseline BaselineSet, evidence map[string]metricEvidence) {
	for _, id := range sortedMetricIDs(baseline.Metrics) {
		state := evidence[id]
		if state.Present || !state.Applicable {
			continue
		}
		for _, fallbackID := range sortedMetricIDs(baseline.Metrics) {
			fallback := baseline.Metrics[fallbackID]
			if fallback.FallbackFor != id {
				continue
			}
			fallbackState := evidence[fallbackID]
			if !fallbackState.Present || fallbackState.Consumed {
				continue
			}
			state.Present = true
			state.Status = MetricOK
			state.Value = fallbackState.Value
			state.Unit = fallbackState.Unit
			state.Normalized = fallbackState.Normalized
			state.Note = "via " + fallbackID + " (" + fallback.Source.Workload + ")"
			evidence[id] = state
			fallbackState.Consumed = true
			fallbackState.Weighted = false
			fallbackState.Note = "consumed by " + id
			evidence[fallbackID] = fallbackState
			break
		}
	}
}

func evaluateDimensions(baseline BaselineSet, in input, evidence map[string]metricEvidence) []DimensionResult {
	factor := iterationFactor(in.Iterations)
	results := make([]DimensionResult, 0, len(baseline.Dimensions))
	for _, dimensionID := range baseline.sortedDimensionIDs() {
		result := DimensionResult{ID: dimensionID, Status: DimensionNotApplicable}
		metricIDs := baseline.metricIDsForDimension(dimensionID)
		applicable := false
		expectedWeight := 0.0
		presentWeight := 0.0
		weightedSum := 0.0
		for _, metricID := range metricIDs {
			state, ok := evidence[metricID]
			if !ok || !state.Applicable {
				continue
			}
			applicable = true
			metric := MetricResult{
				ID:     metricID,
				Status: state.Status,
				Note:   state.Note,
			}
			if state.Present {
				metric.Workload = state.Metric.Source.Workload
				metric.Value = state.Value
				metric.Unit = state.Unit
				metric.Normalized = state.Normalized
				metric.Class = state.Metric.Class
				if !state.Consumed {
					metric.Weight = state.Metric.Weight
				}
			}
			if state.Weighted {
				expectedWeight += state.Metric.Weight
				if state.Present {
					presentWeight += state.Metric.Weight
					weightedSum += state.Metric.Weight * state.Normalized
				} else {
					result.Missing = append(result.Missing, metricID)
				}
			}
			result.Metrics = append(result.Metrics, metric)
		}
		if !applicable {
			results = append(results, result)
			continue
		}
		if expectedWeight <= 0 || presentWeight <= 0 {
			result.Status = DimensionNoData
			result.CoveragePct = 0
			result.Confidence = 0
			results = append(results, result)
			continue
		}
		result.Status = DimensionScored
		result.Index = weightedSum / presentWeight
		result.Rating = baseline.rating(result.Index)
		result.CoveragePct = 100 * presentWeight / expectedWeight
		result.Confidence = percentToFraction(result.CoveragePct) * factor
		results = append(results, result)
	}
	return results
}

// evaluateComposite reweights dimensions that have data and withholds the
// composite when the report cannot support one.
func evaluateComposite(baseline BaselineSet, dimensions []DimensionResult, warnings *[]string) *CompositeResult {
	scores := make(map[string]float64, len(dimensions))
	var basis, excluded, noData []string
	cpuAvailable := false
	performanceAvailable := 0
	for _, dimension := range dimensions {
		if dimension.Status == DimensionNotApplicable {
			continue
		}
		if dimension.Status == DimensionNoData {
			noData = append(noData, dimension.ID)
			continue
		}
		scores[dimension.ID] = dimension.Index
		basis = append(basis, dimension.ID)
		if dimension.ID == "cpu" {
			cpuAvailable = true
		}
		if dimension.ID != "stability" {
			performanceAvailable++
		}
	}
	sort.Strings(basis)
	sort.Strings(noData)

	if !cpuAvailable {
		*warnings = append(*warnings, "composite withheld: cpu dimension has no data")
		return nil
	}
	if performanceAvailable < minPerformanceDimensions {
		*warnings = append(*warnings, fmt.Sprintf("composite withheld: only %d performance dimensions have data", performanceAvailable))
		return nil
	}

	weightSum := 0.0
	indexSum := 0.0
	for _, id := range basis {
		weight := baseline.Dimensions[id].Weight
		weightSum += weight
		indexSum += weight * scores[id]
	}
	status := "complete"
	if len(noData) > 0 {
		status = "partial"
		excluded = noData
		*warnings = append(*warnings, "composite is partial; reweighted without: "+strings.Join(noData, ", "))
	}
	index := indexSum / weightSum
	return &CompositeResult{
		Index:    index,
		Rating:   baseline.rating(index),
		Status:   status,
		Basis:    basis,
		Excluded: excluded,
	}
}

func evaluateProfiles(baseline BaselineSet, evidence map[string]metricEvidence) []ProfileResult {
	values := make(map[string]float64, len(evidence))
	for id, state := range evidence {
		if state.Present {
			values[id] = state.Value
		}
	}
	// Dimension scores are recomputed here (not reused from the composite)
	// so profiles keep working when the composite is withheld.
	dimensionScores := make(map[string]float64, len(baseline.Dimensions))
	for _, dimensionID := range baseline.sortedDimensionIDs() {
		weightSum := 0.0
		scoreSum := 0.0
		for _, metricID := range baseline.metricIDsForDimension(dimensionID) {
			state, ok := evidence[metricID]
			if !ok || !state.Applicable || !state.Present || state.Consumed || state.Metric.Weight <= 0 {
				continue
			}
			weightSum += state.Metric.Weight
			scoreSum += state.Metric.Weight * state.Normalized
		}
		if weightSum > 0 {
			dimensionScores[dimensionID] = scoreSum / weightSum
		}
	}

	results := make([]ProfileResult, 0, len(baseline.Profiles))
	for _, profileID := range baseline.sortedProfileIDs() {
		profile := baseline.Profiles[profileID]
		result := ProfileResult{ID: profileID, Fit: FitInsufficientEvidence}

		weightSum := 0.0
		indexSum := 0.0
		for _, dimensionID := range sortedDimensionIDsIn(profile.Weights) {
			weight, ok := profile.Weights[dimensionID]
			if !ok || weight <= 0 {
				continue
			}
			score, ok := dimensionScores[dimensionID]
			if !ok {
				continue
			}
			weightSum += weight
			indexSum += weight * score
		}

		veto := firstVetoHit(profile.Vetoes, values)
		var missing []string
		for _, rule := range profile.Vetoes {
			if _, ok := values[rule.Metric]; !ok {
				missing = append(missing, rule.Metric)
			}
		}
		sort.Strings(missing)
		result.Missing = missing

		if veto != nil {
			result.Veto = veto
			result.Fit = FitUnsuitable
			if weightSum > 0 {
				result.Index = indexSum / weightSum
				result.Rating = capRating(baseline.rating(result.Index), veto.Cap)
			}
			results = append(results, result)
			continue
		}
		if weightSum <= 0 {
			results = append(results, result)
			continue
		}
		result.Index = indexSum / weightSum
		result.Rating = baseline.rating(result.Index)
		if result.Index >= baseline.Grades.B {
			result.Fit = FitRecommended
		} else {
			result.Fit = FitConditional
		}
		results = append(results, result)
	}
	return results
}

// firstVetoHit returns the first triggered veto in rule order.
func firstVetoHit(rules []VetoRule, values map[string]float64) *VetoHit {
	for _, rule := range rules {
		value, ok := values[rule.Metric]
		if !ok {
			continue
		}
		triggered := (rule.Op == "gt" && value > rule.Value) || (rule.Op == "lt" && value < rule.Value)
		if !triggered {
			continue
		}
		cap := rule.Cap
		if cap == "" {
			cap = "C"
		}
		return &VetoHit{
			Metric:    rule.Metric,
			Op:        rule.Op,
			Value:     value,
			Threshold: rule.Value,
			Unit:      rule.Unit,
			Cap:       cap,
		}
	}
	return nil
}

func summarizeCoverage(baseline BaselineSet, evidence map[string]metricEvidence, dimensions []DimensionResult) CoverageSummary {
	summary := CoverageSummary{
		Present:           []string{},
		Missing:           []string{},
		DimensionsPresent: []string{},
		DimensionsMissing: []string{},
	}
	expectedWeight := 0.0
	presentWeight := 0.0
	for _, id := range sortedMetricIDs(baseline.Metrics) {
		state, ok := evidence[id]
		if !ok || !state.Applicable {
			continue
		}
		if state.Consumed {
			// The reading is represented by the metric it substituted for.
			continue
		}
		if state.Present {
			summary.Present = append(summary.Present, id)
		}
		if !state.Weighted {
			continue
		}
		expectedWeight += state.Metric.Weight
		if state.Present {
			presentWeight += state.Metric.Weight
		} else {
			summary.Missing = append(summary.Missing, id)
		}
	}
	if expectedWeight > 0 {
		summary.MetricPct = 100 * presentWeight / expectedWeight
	}
	for _, dimension := range dimensions {
		switch dimension.Status {
		case DimensionScored:
			summary.DimensionsPresent = append(summary.DimensionsPresent, dimension.ID)
		case DimensionNoData:
			summary.DimensionsMissing = append(summary.DimensionsMissing, dimension.ID)
		}
	}
	return summary
}

// kindApplies reports whether a metric's requires_kind matches the report.
func kindApplies(requiresKind, kind string) bool {
	switch requiresKind {
	case "", "any":
		return true
	default:
		return requiresKind == kind
	}
}

// platformApplies reports whether a metric's platform constraint matches.
// An unknown report platform keeps platform-constrained metrics inapplicable.
func platformApplies(metricPlatform, platform string) bool {
	if metricPlatform == "" {
		return true
	}
	return metricPlatform == platform
}

func normalizeMetric(metric MetricBaseline, value float64) float64 {
	switch metric.Class {
	case "log":
		return normalizeLog(value, metric.Floor, metric.Ceiling)
	case "band":
		if metric.Bands == nil {
			return 0
		}
		return normalizeBand(value, *metric.Bands)
	default:
		return 0
	}
}

// rating maps an index onto the baseline's grade letters.
func (b *BaselineSet) rating(index float64) string {
	switch {
	case index >= b.Grades.S:
		return "S"
	case index >= b.Grades.A:
		return "A"
	case index >= b.Grades.B:
		return "B"
	case index >= b.Grades.C:
		return "C"
	default:
		return "D"
	}
}

// capRating lowers a rating to the cap letter when it ranks above it.
func capRating(rating, cap string) string {
	if cap == "" {
		return rating
	}
	order := []string{"S", "A", "B", "C", "D"}
	rank := map[string]int{}
	for i, letter := range order {
		rank[letter] = i
	}
	r, okr := rank[rating]
	c, okc := rank[cap]
	if !okr || !okc {
		return rating
	}
	if r < c {
		return cap
	}
	return rating
}

func toolSet(tools []string) map[string]bool {
	if len(tools) == 0 {
		return map[string]bool{}
	}
	set := make(map[string]bool, len(tools))
	for _, tool := range tools {
		set[tool] = true
	}
	return set
}

func baselineSourceLabel(source string) string {
	if source == "" {
		return SourceEmbedded
	}
	return source
}

func percentToFraction(percent float64) float64 {
	return clamp(percent, 0, 100) / 100
}

func sortedMetricIDs(metrics map[string]MetricBaseline) []string {
	ids := make([]string, 0, len(metrics))
	for id := range metrics {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedDimensionIDsIn(weights map[string]float64) []string {
	ids := make([]string, 0, len(weights))
	for id := range weights {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
