//go:build !js

package goscriptbench

import "context"

// Workload supplies setup, restart, measurement, and validation behavior to a Runner.
type Workload interface {
	Setup(ctx context.Context) (RunMetadata, error)
	Restart(ctx context.Context, request SampleRequest) error
	Measure(ctx context.Context, request SampleRequest) (Sample, error)
	Validate(ctx context.Context, request SampleRequest, sample Sample) error
}
