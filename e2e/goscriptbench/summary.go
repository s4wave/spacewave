//go:build !js

package goscriptbench

import (
	"math"
	"slices"

	"github.com/pkg/errors"
)

// Summary records nearest-rank statistics derived from retained source rows.
type Summary struct {
	// method names the percentile algorithm
	Method string
	// sampleCount is the number of retained source rows
	SampleCount int
	// medianDisplayReadyMs is the nearest-rank p50
	MedianDisplayReadyMs float64
	// p95DisplayReadyMs is the nearest-rank p95
	P95DisplayReadyMs float64
}

// SummarizeSamples derives nearest-rank p50 and p95 without reordering source rows.
func SummarizeSamples(samples []Sample) (Summary, error) {
	if len(samples) == 0 {
		return Summary{}, errors.New("cannot summarize an empty sample population")
	}
	values := make([]float64, len(samples))
	for idx, sample := range samples {
		if math.IsNaN(sample.DisplayReadyMs) || math.IsInf(sample.DisplayReadyMs, 0) {
			return Summary{}, errors.Errorf("sample %q has a non-finite displayReadyMs", sample.ID)
		}
		values[idx] = sample.DisplayReadyMs
	}
	slices.Sort(values)
	return Summary{
		Method:               SummaryMethodNearestRank,
		SampleCount:          len(samples),
		MedianDisplayReadyMs: nearestRank(values, 0.50),
		P95DisplayReadyMs:    nearestRank(values, 0.95),
	}, nil
}

// Validate checks that the summary equals the retained source rows.
func (s Summary) Validate(samples []Sample) error {
	derived, err := SummarizeSamples(samples)
	if err != nil {
		return err
	}
	if s.Method != derived.Method || s.SampleCount != derived.SampleCount {
		return errors.New("summary method or sample count differs from retained rows")
	}
	if s.MedianDisplayReadyMs != derived.MedianDisplayReadyMs || s.P95DisplayReadyMs != derived.P95DisplayReadyMs {
		return errors.New("summary statistics differ from retained rows")
	}
	return nil
}

func nearestRank(sorted []float64, percentile float64) float64 {
	rank := min(max(int(math.Ceil(percentile*float64(len(sorted)))), 1), len(sorted))
	return sorted[rank-1]
}
