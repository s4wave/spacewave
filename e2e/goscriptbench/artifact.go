//go:build !js

package goscriptbench

import "github.com/pkg/errors"

const artifactSchemaVersion = 1

// Artifact is one engine's untraced scalar result.
type Artifact struct {
	// schemaVersion identifies the artifact format
	SchemaVersion int
	// metadata identifies the engine, fixture, revisions, and state boundary
	Metadata RunMetadata
	// sampling records fixed population sizes and summary method
	Sampling SamplingPolicy
	// warmup is the complete row excluded from the scalar population
	Warmup Sample
	// samples are the retained source rows in execution order
	Samples []Sample
	// summary is derived only from samples
	Summary Summary
}

// Validate checks that the scalar result is complete and internally consistent.
func (a Artifact) Validate() error {
	if a.SchemaVersion != artifactSchemaVersion {
		return errors.Errorf("artifact schema version %d is unsupported", a.SchemaVersion)
	}
	if err := a.Metadata.Validate(); err != nil {
		return errors.Wrap(err, "validate run metadata")
	}
	if err := a.Sampling.Validate(); err != nil {
		return errors.Wrap(err, "validate sampling policy")
	}
	if len(a.Samples) != a.Sampling.RetainedSamples {
		return errors.Errorf("artifact has %d retained samples, expected %d", len(a.Samples), a.Sampling.RetainedSamples)
	}
	if a.Warmup.Traced {
		return errors.New("discarded warm-up cannot be traced")
	}
	if err := a.Warmup.Validate(a.Metadata); err != nil {
		return errors.Wrap(err, "validate discarded warm-up")
	}
	for idx, sample := range a.Samples {
		if sample.Traced {
			return errors.Errorf("retained sample %q cannot be traced", sample.ID)
		}
		if err := sample.Validate(a.Metadata); err != nil {
			return errors.Wrapf(err, "validate retained sample %d", idx+1)
		}
	}
	if err := a.Summary.Validate(a.Samples); err != nil {
		return errors.Wrap(err, "validate summary")
	}
	return nil
}
