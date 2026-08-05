//go:build !js

package goscriptbench

import (
	"context"

	"github.com/pkg/errors"
)

// Runner executes fixed benchmark populations and publishes validated artifacts.
type Runner struct {
	// publisher atomically exposes complete per-engine results
	publisher *ArtifactPublisher
}

// NewRunner constructs a runner rooted at outputRoot.
func NewRunner(outputRoot string) (*Runner, error) {
	publisher, err := NewArtifactPublisher(outputRoot)
	if err != nil {
		return nil, err
	}
	return &Runner{publisher: publisher}, nil
}

// Run executes one workload and returns its published engine directory.
func (r *Runner) Run(ctx context.Context, workload Workload) (string, error) {
	if r == nil || r.publisher == nil {
		return "", errors.New("runner is not initialized")
	}
	if workload == nil {
		return "", errors.New("workload is required")
	}

	// Establish the fixture, run identity, and state boundary before sampling.
	metadata, err := workload.Setup(ctx)
	if err != nil {
		return "", errors.Wrap(err, "setup workload")
	}
	if err := metadata.Validate(); err != nil {
		return "", errors.Wrap(err, "validate workload metadata")
	}

	// Execute one complete untraced warm-up and discard it from the distribution.
	warmupMeasurement, err := r.runSample(ctx, workload, metadata, SampleRequest{
		Kind:   SampleKindWarmup,
		Number: 1,
	})
	if err != nil {
		return "", err
	}

	// Retain ten complete untraced source rows in execution order.
	samples := make([]Sample, 0, RetainedSampleCount)
	for number := 1; number <= RetainedSampleCount; number++ {
		sample, err := r.runSample(ctx, workload, metadata, SampleRequest{
			Kind:   SampleKindRetained,
			Number: number,
		})
		if err != nil {
			return "", err
		}
		samples = append(samples, sample.Sample)
	}

	// Execute the traced diagnostic outside the retained population.
	diagnostic, err := r.runSample(ctx, workload, metadata, SampleRequest{
		Kind:   SampleKindDiagnostic,
		Number: 1,
		Trace:  true,
	})
	if err != nil {
		return "", err
	}

	// Name optional evidence, derive summaries, and publish one complete bundle.
	browserCPUProfileFile := ""
	if len(diagnostic.BrowserCPUProfile) != 0 {
		browserCPUProfileFile = artifactBrowserCPUProfileFile
	}

	summary, err := SummarizeSamples(samples)
	if err != nil {
		return "", errors.Wrap(err, "summarize retained samples")
	}
	return r.publisher.Publish(ArtifactBundle{
		Result: Artifact{
			SchemaVersion: artifactSchemaVersion,
			Metadata:      metadata,
			Sampling:      fixedSamplingPolicy(),
			Warmup:        warmupMeasurement.Sample,
			Samples:       samples,
			Summary:       summary,
		},
		Diagnostic: DiagnosticArtifact{
			SchemaVersion:         artifactSchemaVersion,
			RunID:                 metadata.RunID,
			Engine:                metadata.Engine,
			Sample:                diagnostic.Sample,
			RuntimeTraceFile:      artifactRuntimeTraceFile,
			BrowserCPUProfileFile: browserCPUProfileFile,
		},
		RuntimeTrace:      diagnostic.RuntimeTrace,
		BrowserCPUProfile: diagnostic.BrowserCPUProfile,
	})
}

func (r *Runner) runSample(
	ctx context.Context,
	workload Workload,
	metadata RunMetadata,
	request SampleRequest,
) (Measurement, error) {
	if err := ctx.Err(); err != nil {
		return Measurement{}, err
	}
	if err := workload.Restart(ctx, request); err != nil {
		return Measurement{}, errors.Wrapf(err, "restart workload for %s sample %d", request.Kind, request.Number)
	}
	measurement, err := workload.Measure(ctx, request)
	if err != nil {
		return Measurement{}, errors.Wrapf(err, "measure %s sample %d", request.Kind, request.Number)
	}
	if err := workload.Validate(ctx, request, measurement.Sample); err != nil {
		return Measurement{}, errors.Wrapf(err, "workload validation failed for %s sample %d", request.Kind, request.Number)
	}
	if err := measurement.Validate(request, metadata); err != nil {
		return Measurement{}, errors.Wrapf(err, "validate %s sample %d", request.Kind, request.Number)
	}
	return measurement, nil
}
