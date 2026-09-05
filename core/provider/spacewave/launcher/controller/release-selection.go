//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	spacewave_release "github.com/s4wave/spacewave/core/release"
)

// SelectReleaseManifests resolves this application's desktop and optional CLI.
// A nil CLI reference means CLI updates are explicitly disabled. Unconfigured
// identifiers retain the Spacewave release contract used by existing launchers.
func (c *Config) SelectReleaseManifests(metadata *spacewave_release.ReleaseMetadata, platformID string) (*bldr_manifest.ManifestRef, *bldr_manifest.ManifestRef, error) {
	// Refuse another application's metadata even when its manifest names match.
	if projectID := c.GetProjectId(); projectID != "" && metadata.GetProjectId() != projectID {
		return nil, nil, errors.New("release metadata project does not match launcher configuration")
	}

	// Select exactly one desktop entrypoint for the running platform.
	manifestID := c.GetEntrypointManifestId()
	if manifestID == "" {
		manifestID = nativeEntrypointManifestID
	}
	native, err := selectReleaseManifestRefByID(metadata, platformID, manifestID, "native")
	if err != nil {
		return nil, nil, err
	}
	if c.GetDisableCliUpdate() {
		return native, nil, nil
	}

	// Keep the companion mandatory unless this application explicitly omits it.
	cliID := c.GetCliManifestId()
	if cliID == "" {
		cliID = cliEntrypointManifestID
	}
	cli, err := selectReleaseManifestRefByID(metadata, platformID, cliID, "cli")
	if err != nil {
		return nil, nil, err
	}
	return native, cli, nil
}
