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
	bldr_web_bundler_esbuild "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	bldr_web_bundler_esbuild_compiler "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/compiler"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

const (
	// esbuildSubManifestID is the ID used for the esbuild sub-manifest.
	esbuildSubManifestID = "esbuild"
)

// EsbuildAssetSubdir is the sub-directory for esbuild assets within the assets dir.
var EsbuildAssetSubdir = "esb"

// BuildAndCheckoutEsbuildSubManifest builds the esbuild sub-manifest and checks out the results.
// It returns the web package references, source files, and esbuild output metadata extracted from the sub-manifest.
// The caller is responsible for constructing and validating the esbuildBuilderProto.
func BuildAndCheckoutEsbuildSubManifest(
	ctx context.Context,
	le *logrus.Entry,
	host bldr_manifest_builder.BuildManifestHost,
	buildWorld world.Engine,
	outAssetsPath string,
	esbuildBuilderProto *configset_proto.ControllerConfig,
) (web_pkg.WebPkgRefSlice, []string, []*bldr_web_bundler_esbuild.EsbuildOutputMeta, error) {
	// build the manifest for this esbuild bundle
	le.Debug("waiting for esbuild sub-manifest")
	subManifestPromise, err := host.BuildSubManifest(ctx, esbuildSubManifestID, &bldr_project.ManifestConfig{
		Builder: esbuildBuilderProto,
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start esbuild sub-manifest build")
	}

	// wait for the result
	subManifestResult, err := subManifestPromise.Await(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "esbuild sub-manifest build failed")
	}

	// parse out the input manifest meta
	subManifestInput := subManifestResult.GetInputManifest()
	subManifestInputMeta := &bldr_web_bundler_esbuild_compiler.InputManifestMeta{}
	if err := subManifestInputMeta.UnmarshalVT(subManifestInput.GetMetadata()); err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to parse esbuild sub-manifest input metadata")
	}

	// extract variables
	webPkgRefs := subManifestInputMeta.GetWebPkgRefs()
	esbuildOutputMeta := subManifestInputMeta.GetEsbuildOutputs()
	var srcFiles []string
	for _, inputFile := range subManifestInput.GetFiles() {
		srcFiles = append(srcFiles, inputFile.GetPath())
	}

	// sync the latest sub-manifest contents into our assets directory
	le.Debug("esbuild sub-manifest build complete, checking out assets")
	outAssetsEsbuildPath := filepath.Join(outAssetsPath, EsbuildAssetSubdir)
	_, err = bldr_manifest_world.CheckoutManifest(
		ctx,
		le,
		buildWorld.AccessWorldState,
		subManifestResult.GetManifestRef().GetManifestRef(),
		"", // No dist path for esbuild sub-manifest
		outAssetsEsbuildPath,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		nil, // No dist filter for esbuild sub-manifest
		nil,
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to extract esbuild sub-manifest")
	}

	// move any web-pkgs to the correct dir. these functions ignore not-exist source dirs
	webPkgsDir := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
	outAssetsEsbuildWebPkgsDir := filepath.Join(outAssetsEsbuildPath, bldr_plugin.PluginAssetsWebPkgsDir)
	if err := fsutil.CopyRecursive(webPkgsDir, outAssetsEsbuildWebPkgsDir, nil); err != nil {
		return nil, nil, nil, err
	}
	if err := fsutil.CleanDir(outAssetsEsbuildWebPkgsDir); err != nil {
		return nil, nil, nil, err
	}

	return webPkgRefs, srcFiles, esbuildOutputMeta, nil
}
