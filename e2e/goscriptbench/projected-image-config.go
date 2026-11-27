//go:build !js

package goscriptbench

import "github.com/pkg/errors"

// ProjectedImageConfig identifies the source and engine for one workload run.
type ProjectedImageConfig struct {
	// RunID groups this result with other engine processes.
	RunID string
	// Engine identifies the selected Playwright browser.
	Engine string
	// SpacewaveRevision identifies the Spacewave source tree.
	SpacewaveRevision string
	// GoScriptRevision identifies the GoScript source tree.
	GoScriptRevision string
	// UnavailableFields lists optional timing fields omitted by the engine.
	UnavailableFields []string
	// BrowserCPUProfile enables optional same-window Chromium profiling.
	BrowserCPUProfile bool
}

// Validate checks the workload identity before browser setup begins.
func (c ProjectedImageConfig) Validate() error {
	// Require a safe run ID and a supported Playwright engine.
	if !validArtifactID(c.RunID) {
		return errors.New("run ID must be a safe artifact path component")
	}
	switch c.Engine {
	case "chromium", "firefox", "webkit":
	default:
		return errors.New("engine must be chromium, firefox, or webkit")
	}
	if c.BrowserCPUProfile && c.Engine != "chromium" {
		return errors.New("browser CPU profile requires Chromium")
	}

	// Require source revisions and valid optional-field names.
	if c.SpacewaveRevision == "" || c.GoScriptRevision == "" {
		return errors.New("Spacewave and GoScript revisions are required")
	}
	if err := validateUnavailableFields(c.UnavailableFields); err != nil {
		return err
	}
	return nil
}
