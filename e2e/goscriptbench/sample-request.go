//go:build !js

package goscriptbench

// SampleKind identifies a warm-up, retained, or diagnostic action.
type SampleKind string

const (
	// SampleKindWarmup identifies the discarded untraced warm-up.
	SampleKindWarmup SampleKind = "warmup"
	// SampleKindRetained identifies one untraced source row.
	SampleKindRetained SampleKind = "retained"
	// SampleKindDiagnostic identifies the separately traced diagnostic.
	SampleKindDiagnostic SampleKind = "diagnostic"
)

// SampleRequest describes one action the runner asks a workload to perform.
type SampleRequest struct {
	// kind identifies the sample population
	Kind SampleKind
	// number is one-based within the sample population
	Number int
	// trace enables diagnostic tracing for this action
	Trace bool
}
