package bldr_project_controller

import (
	"os"
	"path/filepath"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	"github.com/pkg/errors"
)

// manifestStartupBuildStateDirName is the startup build-state directory name.
const manifestStartupBuildStateDirName = "manifest-startup-build-state"

// NewManifestStartupBuildState constructs a new startup build-state object.
func NewManifestStartupBuildState(
	manifestBuilderConfig *ManifestBuilderConfig,
	builderResult *bldr_manifest_builder.BuilderResult,
) *ManifestStartupBuildState {
	return &ManifestStartupBuildState{
		ManifestBuilderConfig: manifestBuilderConfig,
		BuilderResult:         builderResult,
	}
}

// Validate validates the startup build state.
func (m *ManifestStartupBuildState) Validate() error {
	if err := m.GetManifestBuilderConfig().Validate(); err != nil {
		return errors.Wrap(err, "manifest_builder_config")
	}
	if err := m.GetBuilderResult().Validate(); err != nil {
		return errors.Wrap(err, "builder_result")
	}
	meta := m.GetBuilderResult().GetManifest().GetMeta()
	conf := m.GetManifestBuilderConfig()
	if meta.GetManifestId() != conf.GetManifestId() {
		return errors.Errorf(
			"builder_result.manifest.meta.manifest_id must match manifest_builder_config: %q != %q",
			meta.GetManifestId(),
			conf.GetManifestId(),
		)
	}
	if meta.GetBuildType() != conf.GetBuildType() {
		return errors.Errorf(
			"builder_result.manifest.meta.build_type must match manifest_builder_config: %q != %q",
			meta.GetBuildType(),
			conf.GetBuildType(),
		)
	}
	if meta.GetPlatformId() != conf.GetPlatformId() {
		return errors.Errorf(
			"builder_result.manifest.meta.platform_id must match manifest_builder_config: %q != %q",
			meta.GetPlatformId(),
			conf.GetPlatformId(),
		)
	}
	return nil
}

// GetStatePath returns the startup build-state path under the working path.
func (m *ManifestStartupBuildState) GetStatePath(workingPath string) (string, error) {
	return getManifestStartupBuildStatePath(workingPath, m.GetManifestBuilderConfig())
}

// WriteFile writes the startup build state under the working path.
func (m *ManifestStartupBuildState) WriteFile(workingPath string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	statePath, err := m.GetStatePath(workingPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	data, err := m.MarshalVT()
	if err != nil {
		return err
	}
	tempPath := statePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tempPath, statePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

// ReadManifestStartupBuildState reads the startup build state for a manifest
// build slot.
func ReadManifestStartupBuildState(
	workingPath string,
	manifestBuilderConfig *ManifestBuilderConfig,
) (*ManifestStartupBuildState, error) {
	statePath, err := getManifestStartupBuildStatePath(workingPath, manifestBuilderConfig)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	state := &ManifestStartupBuildState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, err
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	return state, nil
}

// RemoveManifestStartupBuildState removes the startup build state for a
// manifest build slot.
func RemoveManifestStartupBuildState(
	workingPath string,
	manifestBuilderConfig *ManifestBuilderConfig,
) error {
	statePath, err := getManifestStartupBuildStatePath(workingPath, manifestBuilderConfig)
	if err != nil {
		return err
	}
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RemoveAllManifestStartupBuildStates removes all persisted startup build
// states under the working path.
func RemoveAllManifestStartupBuildStates(workingPath string) error {
	rootPath, err := getManifestStartupBuildStateRoot(workingPath)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(rootPath); err != nil {
		return err
	}
	return nil
}

// getManifestStartupBuildStatePath builds the startup build-state path for a
// manifest build slot.
func getManifestStartupBuildStatePath(
	workingPath string,
	manifestBuilderConfig *ManifestBuilderConfig,
) (string, error) {
	rootPath, err := getManifestStartupBuildStateRoot(workingPath)
	if err != nil {
		return "", err
	}
	if err := manifestBuilderConfig.Validate(); err != nil {
		return "", errors.Wrap(err, "manifest_builder_config")
	}
	buildType := manifestBuilderConfig.GetBuildType()
	if buildType == "" {
		buildType = string(bldr_manifest.BuildType_DEV)
	}
	return filepath.Join(
		rootPath,
		manifestBuilderConfig.GetManifestId(),
		buildType,
		filepath.FromSlash(manifestBuilderConfig.GetPlatformId()),
		manifestBuilderConfig.MarshalB58()+".pb",
	), nil
}

// getManifestStartupBuildStateRoot builds the startup build-state root path.
func getManifestStartupBuildStateRoot(workingPath string) (string, error) {
	if workingPath == "" {
		return "", errors.Wrap(bldr_manifest.ErrEmptyPath, "working path")
	}
	if !filepath.IsAbs(workingPath) {
		return "", errors.New("working path must be absolute")
	}
	return filepath.Join(
		workingPath,
		"cache",
		manifestStartupBuildStateDirName,
	), nil
}
