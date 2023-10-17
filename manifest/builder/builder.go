package bldr_manifest_builder

import (
	"context"
	"path"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"golang.org/x/exp/slices"
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
	) (*BuilderResult, error)
}

// NewInputManifest constructs a new input manifest with a list of files.
func NewInputManifest(paths []string) *InputManifest {
	manifest := &InputManifest{}
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
func (i *InputManifest) SortFiles() {
	if i != nil {
		slices.SortFunc(i.Files, func(a, b *InputManifest_File) int {
			return strings.Compare(a.GetPath(), b.GetPath())
		})
	}
}
