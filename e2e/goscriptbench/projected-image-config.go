//go:build !js

package goscriptbench

import "github.com/pkg/errors"

// ProjectedImageConfig identifies the source and engine for one workload run.
type ProjectedImageConfig struct {
	// runID groups this result with other engine processes
	RunID string
	// engine names the selected Playwright browser
	Engine string
	// spacewaveRevision identifies the Spacewave source tree
	SpacewaveRevision string
	// goScriptRevision identifies the GoScript source tree
	GoScriptRevision string
	// unavailableFields names optional timing fields omitted by the engine
	UnavailableFields []string
	// browserCPUProfile enables optional same-window Chromium profiling
	BrowserCPUProfile bool
}

// Validate checks the workload identity before browser setup begins.
func (c ProjectedImageConfig) Validate() error {
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
	if c.SpacewaveRevision == "" || c.GoScriptRevision == "" {
		return errors.New("Spacewave and GoScript revisions are required")
	}
	if err := validateUnavailableFields(c.UnavailableFields); err != nil {
		return err
	}
	return nil
}
