package bldr_plugin_compiler

import (
	"context"
	"path/filepath"

	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_web_bundler_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// viteSubManifestID is the ID used for the vite sub-manifest.
	viteSubManifestID = "vite"
)

// ViteAssetSubdir is the sub-directory for vite assets within the assets dir.
var ViteAssetSubdir = "v"

// BuildAndCheckoutViteSubManifest builds the vite sub-manifest and checks out the results.
// It returns the web package references, source files, and vite output metadata extracted from the sub-manifest.
// The caller is responsible for constructing and validating the viteBuilderProto.
func BuildAndCheckoutViteSubManifest(
	ctx context.Context,
	le *logrus.Entry,
	host bldr_manifest_builder.BuildManifestHost,
	buildWorld world.Engine,
	outAssetsPath string,
	viteBuilderProto *configset_proto.ControllerConfig,
) (web_pkg.WebPkgRefSlice, []string, []*bldr_web_bundler_vite.ViteOutputMeta, error) {
	// build the manifest for this vite bundle
	le.Debug("waiting for vite sub-manifest")
	subManifestPromise, err := host.BuildSubManifest(ctx, viteSubManifestID, &bldr_project.ManifestConfig{
		Builder: viteBuilderProto,
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start vite sub-manifest build")
	}

	// wait for the result
	subManifestResult, err := subManifestPromise.Await(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "vite sub-manifest build failed")
	}

	// parse out the input manifest meta
	subManifestInput := subManifestResult.GetInputManifest()
	subManifestInputMeta := &bldr_web_bundler_vite_compiler.InputManifestMeta{}
	if err := subManifestInputMeta.UnmarshalVT(subManifestInput.GetMetadata()); err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to parse vite sub-manifest input metadata")
	}

	// extract variables
	viteWebPkgRefs := subManifestInputMeta.GetWebPkgRefs()
	viteOutputMeta := subManifestInputMeta.GetViteOutputs()
	var srcFiles []string
	for _, inputFile := range subManifestInput.GetFiles() {
		srcFiles = append(srcFiles, inputFile.GetPath())
	}

	// sync the latest sub-manifest contents into our assets directory
	le.Debug("vite sub-manifest build complete, checking out assets")
	outAssetsVitePath := filepath.Join(outAssetsPath, ViteAssetSubdir)
	_, err = bldr_manifest_world.CheckoutManifest(
		ctx,
		le,
		buildWorld.AccessWorldState,
		subManifestResult.GetManifestRef().GetManifestRef(),
		"", // No dist path for vite sub-manifest
		outAssetsVitePath,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		nil,
		nil,
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to extract vite sub-manifest")
	}

	// move any web-pkgs to the correct dir. these functions ignore not-exist source dirs
	webPkgsDir := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
	outAssetsViteWebPkgsDir := filepath.Join(outAssetsVitePath, bldr_plugin.PluginAssetsWebPkgsDir)
	if err := fsutil.CopyRecursive(webPkgsDir, outAssetsViteWebPkgsDir, nil); err != nil {
		return nil, nil, nil, err
	}
	if err := fsutil.CleanDir(outAssetsViteWebPkgsDir); err != nil {
		return nil, nil, nil, err
	}

	return viteWebPkgRefs, srcFiles, viteOutputMeta, nil
}
