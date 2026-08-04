//go:build !js

package goscriptbench

import "github.com/pkg/errors"

const (
	// WarmupSampleCount is the number of complete rows discarded before retention.
	WarmupSampleCount = 1
	// RetainedSampleCount is the fixed per-engine scalar population.
	RetainedSampleCount = 10
	// DiagnosticSampleCount is the separately traced sample population.
	DiagnosticSampleCount = 1
	// SummaryMethodNearestRank identifies the retained percentile algorithm.
	SummaryMethodNearestRank = "nearest-rank"
)

// SamplingPolicy records the fixed populations and summary method.
type SamplingPolicy struct {
	// warmupSamples is the discarded sample count
	WarmupSamples int
	// retainedSamples is the untraced scalar sample count
	RetainedSamples int
	// diagnosticSamples is the traced sample count
	DiagnosticSamples int
	// summaryMethod names the percentile algorithm
	SummaryMethod string
}

// Validate checks that the artifact uses the selected fixed sampling contract.
func (p SamplingPolicy) Validate() error {
	if p.WarmupSamples != WarmupSampleCount || p.RetainedSamples != RetainedSampleCount || p.DiagnosticSamples != DiagnosticSampleCount {
		return errors.New("artifact sample counts differ from the fixed sampling contract")
	}
	if p.SummaryMethod != SummaryMethodNearestRank {
		return errors.New("artifact summary method must be nearest-rank")
	}
	return nil
}

func fixedSamplingPolicy() SamplingPolicy {
	return SamplingPolicy{
		WarmupSamples:     WarmupSampleCount,
		RetainedSamples:   RetainedSampleCount,
		DiagnosticSamples: DiagnosticSampleCount,
		SummaryMethod:     SummaryMethodNearestRank,
	}
}
