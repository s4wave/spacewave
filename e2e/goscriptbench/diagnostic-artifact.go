//go:build !js

package goscriptbench

import "github.com/pkg/errors"

// DiagnosticArtifact is the separately traced sample for one engine result.
type DiagnosticArtifact struct {
	// schemaVersion identifies the artifact format
	SchemaVersion int
	// runID links the diagnostic to its scalar result
	RunID string
	// engine links the diagnostic to its browser engine
	Engine string
	// sample is excluded from the retained scalar population
	Sample Sample
	// runtimeTraceFile names the required runtime trace
	RuntimeTraceFile string
	// browserCPUProfileFile names optional Chromium-only CPU evidence
	BrowserCPUProfileFile string
}

// Validate checks that the diagnostic is complete and traced.
func (d DiagnosticArtifact) Validate(metadata RunMetadata) error {
	if d.SchemaVersion != artifactSchemaVersion {
		return errors.Errorf("diagnostic schema version %d is unsupported", d.SchemaVersion)
	}
	if d.RunID != metadata.RunID || d.Engine != metadata.Engine {
		return errors.New("diagnostic identity differs from the scalar result")
	}
	if !d.Sample.Traced {
		return errors.New("diagnostic sample must be traced")
	}
	if d.RuntimeTraceFile != artifactRuntimeTraceFile {
		return errors.New("diagnostic runtime trace filename is invalid")
	}
	if d.BrowserCPUProfileFile != "" && d.Engine != "chromium" {
		return errors.New("diagnostic browser CPU profile requires Chromium")
	}
	if err := d.Sample.Validate(metadata); err != nil {
		return errors.Wrap(err, "validate diagnostic sample")
	}
	return nil
}
