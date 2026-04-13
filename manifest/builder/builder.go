package bldr_manifest_builder

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"

	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/promise"
)

// ControllerConfig is a configuration for a manifest Builder controller.
type ControllerConfig interface {
	// Config is the base config interface.
	config.Config
}

// Controller is a manifest builder controller.
//
// The controller builds and writes the manifest and contents to the configured
// world engine. It should watch for changes and re-build.
type Controller interface {
	controller.Controller

	// BuildManifest attempts to compile the manifest once.
	//
	// prevResult contains the previous successful BuilderResult to be used for caching.
	// prevResult will be nil if the build has not completed successfully before.
	BuildManifest(
		ctx context.Context,
		args *BuildManifestArgs,
		host BuildManifestHost,
	) (*BuilderResult, error)

	// SupportsStartupManifestCache returns true if startup cache reuse is safe.
	SupportsStartupManifestCache() bool

	// GetSupportedPlatforms returns the base platform IDs this compiler supports.
	// Used by the build system to select the appropriate platform for a target.
	// Returns values like "desktop" or "js".
	GetSupportedPlatforms() []string
}

// BuildManifestHost is the host API available to a BuildManifest routine.
type BuildManifestHost interface {
	// BuildSubManifest builds a sub-manifest and returns a compilation promise.
	//
	// subManifestID must be a valid manifest-id.
	// in development mode the compiler will watch for changes to the sub-manifest.
	//
	// once a value is returned from the Promise any change to the sub-manifest
	// will restart parent BuildManifest attempt (implementing hot reloading).
	BuildSubManifest(
		ctx context.Context,
		subManifestID string,
		subManifestConfig *bldr_project.ManifestConfig,
	) (promise.PromiseLike[*BuilderResult], error)
}

// NewInputManifest constructs a new input manifest with a list of files.
func NewInputManifest(paths []string, meta []byte) *InputManifest {
	manifest := &InputManifest{Metadata: meta}
	seenPaths := make(map[string]struct{})
	for _, inputPath := range paths {
		cleanPath := path.Clean(inputPath)
		if _, ok := seenPaths[cleanPath]; ok {
			continue
		}
		seenPaths[cleanPath] = struct{}{}
		manifest.Files = append(manifest.Files, &InputManifest_File{Path: cleanPath})
	}
	return manifest
}

// SortFiles sorts the files field on the input manifest.
func (m *InputManifest) SortFiles() {
	if m != nil {
		slices.SortFunc(m.Files, func(a, b *InputManifest_File) int {
			return strings.Compare(a.GetPath(), b.GetPath())
		})
	}
}

// NewEnvStartupInput constructs an environment-variable startup input.
func NewEnvStartupInput(key, value string) *InputManifest_StartupInput {
	return &InputManifest_StartupInput{
		Kind:        InputManifest_StartupInputKind_ENV_VAR,
		Key:         key,
		StringValue: value,
	}
}

// NewControllerConfigDigestStartupInput constructs a controller-config digest startup input.
func NewControllerConfigDigestStartupInput(digest []byte) *InputManifest_StartupInput {
	return &InputManifest_StartupInput{
		Kind:       InputManifest_StartupInputKind_CONTROLLER_CONFIG_DIGEST,
		Key:        "controller-config",
		BytesValue: digest,
	}
}

// AddStartupInput adds a startup input if a duplicate does not already exist.
func (m *InputManifest) AddStartupInput(input *InputManifest_StartupInput) {
	if m == nil || input == nil {
		return
	}
	for _, existing := range m.StartupInputs {
		if existing.GetKind() != input.GetKind() {
			continue
		}
		if existing.GetKey() != input.GetKey() {
			continue
		}
		if existing.GetStringValue() != input.GetStringValue() {
			continue
		}
		if !bytes.Equal(existing.GetBytesValue(), input.GetBytesValue()) {
			continue
		}
		return
	}
	m.StartupInputs = append(m.StartupInputs, input)
}

// SortStartupInputs sorts startup inputs deterministically.
func (m *InputManifest) SortStartupInputs() {
	if m == nil {
		return
	}
	slices.SortFunc(m.StartupInputs, func(a, b *InputManifest_StartupInput) int {
		if d := int(a.GetKind()) - int(b.GetKind()); d != 0 {
			return d
		}
		if d := strings.Compare(a.GetKey(), b.GetKey()); d != 0 {
			return d
		}
		if d := strings.Compare(a.GetStringValue(), b.GetStringValue()); d != 0 {
			return d
		}
		return bytes.Compare(a.GetBytesValue(), b.GetBytesValue())
	})
}
