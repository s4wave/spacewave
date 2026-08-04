//go:build !js

package goscriptbench

import (
	"slices"

	"github.com/pkg/errors"
)

var optionalSampleFields = []string{
	"decodedBodySize",
	"responseEndMs",
	"responseStartMs",
	"transferSize",
}

// RunMetadata identifies one engine run and its measured state boundary.
type RunMetadata struct {
	// runID groups independent engine results from one matrix invocation
	RunID string
	// engine names the selected browser engine
	Engine string
	// engineVersion identifies the launched browser binary
	EngineVersion string
	// compiler names the Go-to-browser compiler
	Compiler string
	// spacewaveRevision identifies the Spacewave source tree
	SpacewaveRevision string
	// goScriptRevision identifies the GoScript source tree
	GoScriptRevision string
	// buildMode names the measured build shape
	BuildMode string
	// workerMode names the GoScript runtime worker shape
	WorkerMode string
	// storageBackend names the measured persistent storage backend
	StorageBackend string
	// runtimeState names the retained and recreated cache cell
	RuntimeState string
	// projectedURLTemplate identifies the measured request family
	ProjectedURLTemplate string
	// fixture identifies the measured bytes
	Fixture Fixture
	// state declares the boundary between samples
	State StateBoundary
	// unavailableFields names optional browser timing fields omitted by this engine
	UnavailableFields []string
}

// Validate checks that the run identity and state boundary are complete.
func (m RunMetadata) Validate() error {
	if !validArtifactID(m.RunID) {
		return errors.New("run ID must be a safe artifact path component")
	}
	if !validArtifactID(m.Engine) {
		return errors.New("engine must be a safe artifact path component")
	}
	if m.EngineVersion == "" || m.Compiler == "" {
		return errors.New("engine version and compiler are required")
	}
	if m.SpacewaveRevision == "" || m.GoScriptRevision == "" {
		return errors.New("Spacewave and GoScript revisions are required")
	}
	if m.BuildMode == "" || m.WorkerMode == "" || m.StorageBackend == "" {
		return errors.New("build, worker, and storage modes are required")
	}
	if m.RuntimeState == "" || m.ProjectedURLTemplate == "" {
		return errors.New("runtime state and projected URL template are required")
	}
	if err := m.Fixture.Validate(); err != nil {
		return errors.Wrap(err, "validate fixture")
	}
	if err := m.State.Validate(); err != nil {
		return errors.Wrap(err, "validate state boundary")
	}
	if m.UnavailableFields == nil {
		return errors.New("unavailable-field metadata is required")
	}
	for idx, field := range m.UnavailableFields {
		if !slices.Contains(optionalSampleFields, field) {
			return errors.Errorf("unavailable sample field %q is unknown", field)
		}
		if slices.Contains(m.UnavailableFields[:idx], field) {
			return errors.Errorf("unavailable sample field %q is duplicated", field)
		}
	}
	return nil
}

func (m RunMetadata) fieldUnavailable(field string) bool {
	return slices.Contains(m.UnavailableFields, field)
}

func validArtifactID(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for idx, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		if idx != 0 && (char == '-' || char == '_' || char == '.') {
			continue
		}
		return false
	}
	return true
}
