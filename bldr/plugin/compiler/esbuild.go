package bldr_plugin_compiler

import (
	"context"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_web_bundler_esbuild "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	bldr_web_bundler_esbuild_compiler "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/compiler"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
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
	return BuildAndCheckoutSubManifest(
		ctx,
		le,
		host,
		buildWorld,
		outAssetsPath,
		esbuildSubManifestID,
		esbuildBuilderProto,
		EsbuildAssetSubdir,
		func(metadata []byte) (web_pkg.WebPkgRefSlice, []*bldr_web_bundler_esbuild.EsbuildOutputMeta, error) {
			subManifestInputMeta := &bldr_web_bundler_esbuild_compiler.InputManifestMeta{}
			if err := subManifestInputMeta.UnmarshalVT(metadata); err != nil {
				return nil, nil, errors.Wrap(err, "unable to parse esbuild sub-manifest input metadata")
			}
			return subManifestInputMeta.GetWebPkgRefs(), subManifestInputMeta.GetEsbuildOutputs(), nil
		},
	)
}
