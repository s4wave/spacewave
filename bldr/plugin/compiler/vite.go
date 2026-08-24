package bldr_plugin_compiler

import (
	"context"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_web_bundler_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/s4wave/spacewave/db/world"
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
	return BuildAndCheckoutSubManifest(
		ctx,
		le,
		host,
		buildWorld,
		outAssetsPath,
		viteSubManifestID,
		viteBuilderProto,
		ViteAssetSubdir,
		func(metadata []byte) (web_pkg.WebPkgRefSlice, []*bldr_web_bundler_vite.ViteOutputMeta, error) {
			subManifestInputMeta := &bldr_web_bundler_vite_compiler.InputManifestMeta{}
			if err := subManifestInputMeta.UnmarshalVT(metadata); err != nil {
				return nil, nil, errors.Wrap(err, "unable to parse vite sub-manifest input metadata")
			}
			return subManifestInputMeta.GetWebPkgRefs(), subManifestInputMeta.GetViteOutputs(), nil
		},
	)
}
