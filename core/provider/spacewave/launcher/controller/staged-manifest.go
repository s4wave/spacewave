//go:build !js

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// stagedManifestFilename is the sidecar file inside the staging directory that
// records what was last extracted and codesign-verified.
const stagedManifestFilename = "manifest.binpb"

// writeStagedManifest marshals the StagedManifest and writes it into stagingDir
// atomically via a temp file + rename. The manifest must only be written after
// the extracted payload has passed all signature gates so that a later boot
// can trust it as freshness evidence.
func writeStagedManifest(stagingDir string, m *spacewave_launcher.StagedManifest) error {
	if stagingDir == "" {
		return errors.New("staging dir not set")
	}
	if m == nil {
		return errors.New("nil staged manifest")
	}
	data, err := m.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal staged manifest")
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return errors.Wrap(err, "create staging dir")
	}
	tmp := filepath.Join(stagingDir, stagedManifestFilename+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return errors.Wrap(err, "write staged manifest tmp")
	}
	final := filepath.Join(stagingDir, stagedManifestFilename)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return errors.Wrap(err, "rename staged manifest")
	}
	return nil
}

// readStagedManifest reads and decodes the sidecar manifest from stagingDir.
// Returns (nil, nil) when the manifest file does not exist; the caller treats
// that as "nothing staged". Other errors (malformed proto, unreadable) return
// an error and the caller wipes the staging tree.
func readStagedManifest(stagingDir string) (*spacewave_launcher.StagedManifest, error) {
	if stagingDir == "" {
		return nil, errors.New("staging dir not set")
	}
	path := filepath.Join(stagingDir, stagedManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "read staged manifest")
	}
	m := &spacewave_launcher.StagedManifest{}
	if err := m.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal staged manifest")
	}
	return m, nil
}

// removeStagedManifest deletes the sidecar manifest, ignoring "not found".
// Used when the staging tree is wiped due to a stale or failed staged version.
func removeStagedManifest(stagingDir string) error {
	if stagingDir == "" {
		return nil
	}
	path := filepath.Join(stagingDir, stagedManifestFilename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove staged manifest")
	}
	return nil
}
