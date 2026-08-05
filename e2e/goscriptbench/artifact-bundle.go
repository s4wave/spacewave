//go:build !js

package goscriptbench

import "github.com/pkg/errors"

// ArtifactBundle groups one scalar result with its separate diagnostic.
type ArtifactBundle struct {
	// result contains the untraced distribution
	Result Artifact
	// diagnostic contains the traced sample
	Diagnostic DiagnosticArtifact
	// runtimeTrace contains the separately captured diagnostic trace
	RuntimeTrace []byte
	// browserCPUProfile contains optional Chromium-only diagnostic evidence
	BrowserCPUProfile []byte
}

// Validate checks both artifacts and unique sample custody across them.
func (b ArtifactBundle) Validate() error {
	if err := b.Result.Validate(); err != nil {
		return errors.Wrap(err, "validate result artifact")
	}
	if err := b.Diagnostic.Validate(b.Result.Metadata); err != nil {
		return errors.Wrap(err, "validate diagnostic artifact")
	}
	if len(b.RuntimeTrace) == 0 {
		return errors.New("diagnostic runtime trace is empty")
	}
	if len(b.BrowserCPUProfile) == 0 && b.Diagnostic.BrowserCPUProfileFile != "" {
		return errors.New("diagnostic names a missing browser CPU profile")
	}
	if len(b.BrowserCPUProfile) != 0 && b.Diagnostic.BrowserCPUProfileFile != artifactBrowserCPUProfileFile {
		return errors.New("diagnostic browser CPU profile filename is invalid")
	}
	identities := make(map[string]struct{}, len(b.Result.Samples)+2)
	identities[b.Result.Warmup.ID] = struct{}{}
	for _, sample := range b.Result.Samples {
		if _, exists := identities[sample.ID]; exists {
			return errors.Errorf("sample identity %q is duplicated", sample.ID)
		}
		identities[sample.ID] = struct{}{}
	}
	if _, exists := identities[b.Diagnostic.Sample.ID]; exists {
		return errors.Errorf("sample identity %q is duplicated", b.Diagnostic.Sample.ID)
	}
	return nil
}
