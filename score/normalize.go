package score

import "math"

// normalizeLog maps a throughput-class value onto 0-100 on a logarithmic curve
// anchored at floor (0) and ceiling (100). Values at or below floor score 0,
// values at or above ceiling score 100.
func normalizeLog(value, floor, ceiling float64) float64 {
	if value <= 0 || floor <= 0 || ceiling <= floor {
		return 0
	}
	normalized := 100 * math.Log(value/floor) / math.Log(ceiling/floor)
	return clamp(normalized, 0, 100)
}

// normalizeBand maps a lower-is-better value onto 0-100 through the fixed
// piecewise-linear anchors (excellent,100) (good,70) (fair,40) (cutoff,0).
func normalizeBand(value float64, bands BandThresholds) float64 {
	switch {
	case value <= bands.Excellent:
		return 100
	case value > bands.Cutoff:
		return 0
	case value <= bands.Good:
		return interpolate(value, bands.Excellent, 100, bands.Good, 70)
	case value <= bands.Fair:
		return interpolate(value, bands.Good, 70, bands.Fair, 40)
	default:
		return interpolate(value, bands.Fair, 40, bands.Cutoff, 0)
	}
}

func interpolate(value, x0, y0, x1, y1 float64) float64 {
	if x1 <= x0 {
		return y0
	}
	return y0 + (value-x0)*(y1-y0)/(x1-x0)
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// iterationFactor discounts confidence for reports with few iterations.
func iterationFactor(iterations int) float64 {
	switch {
	case iterations >= 3:
		return 1.0
	case iterations == 2:
		return 0.85
	default:
		return 0.7
	}
}

// medianFloat returns the median of a slice without mutating it.
func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sortFloats(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func sortFloats(values []float64) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// coefficientOfVariation returns stddev/mean for a sample set, or 0 when the
// set is too small or flat to be meaningful.
func coefficientOfVariation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	if mean <= 0 {
		return 0
	}
	variance := 0.0
	for _, value := range values {
		variance += (value - mean) * (value - mean)
	}
	variance /= float64(len(values) - 1)
	return math.Sqrt(variance) / mean
}
