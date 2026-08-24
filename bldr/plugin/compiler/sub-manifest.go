package bldr_plugin_compiler

import (
	"context"
	"path/filepath"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// BuildAndCheckoutSubManifest builds a sub-manifest with the given builder
// config and checks out the results into the assets directory.
// It returns the web package references, source files, and output metadata
// parsed from the sub-manifest input metadata.
func BuildAndCheckoutSubManifest[OutputMetaT any](
	ctx context.Context,
	le *logrus.Entry,
	host bldr_manifest_builder.BuildManifestHost,
	buildWorld world.Engine,
	outAssetsPath string,
	subManifestID string,
	builderProto *configset_proto.ControllerConfig,
	assetsSubdir string,
	parseMeta func(metadata []byte) (web_pkg.WebPkgRefSlice, OutputMetaT, error),
) (web_pkg.WebPkgRefSlice, []string, OutputMetaT, error) {
	// build the manifest for this sub-manifest bundle
	le.Debugf("waiting for %s sub-manifest", subManifestID)
	var zeroOutputMeta OutputMetaT
	subManifestPromise, err := host.BuildSubManifest(ctx, subManifestID, &bldr_project.ManifestConfig{
		Builder: builderProto,
	})
	if err != nil {
		return nil, nil, zeroOutputMeta, errors.Wrapf(err, "failed to start %s sub-manifest build", subManifestID)
	}

	// wait for the result
	subManifestResult, err := subManifestPromise.Await(ctx)
	if err != nil {
		return nil, nil, zeroOutputMeta, errors.Wrapf(err, "%s sub-manifest build failed", subManifestID)
	}
	subManifestInput := subManifestResult.GetInputManifest()

	// parse out the input manifest meta
	webPkgRefs, outputMeta, err := parseMeta(subManifestInput.GetMetadata())
	if err != nil {
		return nil, nil, zeroOutputMeta, err
	}

	// extract source files
	var srcFiles []string
	for _, inputFile := range subManifestInput.GetFiles() {
		srcFiles = append(srcFiles, inputFile.GetPath())
	}

	// sync the latest sub-manifest contents into our assets directory
	le.Debugf("%s sub-manifest build complete, checking out assets", subManifestID)
	outAssetsSubPath := filepath.Join(outAssetsPath, assetsSubdir)
	_, err = bldr_manifest_world.CheckoutManifest(
		ctx,
		le,
		buildWorld.AccessWorldState,
		subManifestResult.GetManifestRef().GetManifestRef(),
		"", // No dist path for the sub-manifest
		outAssetsSubPath,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		nil, // No dist filter for the sub-manifest
		nil,
	)
	if err != nil {
		return nil, nil, zeroOutputMeta, errors.Wrapf(err, "unable to extract %s sub-manifest", subManifestID)
	}

	// move any web-pkgs to the correct dir. these functions ignore not-exist source dirs
	webPkgsDir := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
	outAssetsSubWebPkgsDir := filepath.Join(outAssetsSubPath, bldr_plugin.PluginAssetsWebPkgsDir)
	if err := fsutil.CopyRecursive(webPkgsDir, outAssetsSubWebPkgsDir, nil); err != nil {
		return nil, nil, zeroOutputMeta, err
	}
	if err := fsutil.CleanDir(outAssetsSubWebPkgsDir); err != nil {
		return nil, nil, zeroOutputMeta, err
	}

	return webPkgRefs, srcFiles, outputMeta, nil
}
