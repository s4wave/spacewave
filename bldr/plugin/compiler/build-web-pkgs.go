//go:build !js

package bldr_plugin_compiler

import (
	"context"
	"path/filepath"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/util/npm"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	web_pkg_vite "github.com/s4wave/spacewave/bldr/web/pkg/vite"
	"github.com/sirupsen/logrus"
)

// BuildDirectWebPkgs builds the BldrExternal web packages directly using Vite.
//
// This is used when web packages are declared but no esbuild/vite sub-manifests
// exist to build them (e.g. in the saucer flow).
// Returns the web pkg refs, source files, and import map entries mapping
// logical specifiers to hashed output paths.
func BuildDirectWebPkgs(
	ctx context.Context,
	le *logrus.Entry,
	distSourcePath string,
	sourcePath string,
	workingPath string,
	outAssetsPath string,
	isRelease bool,
) (web_pkg.WebPkgRefSlice, []string, []web_pkg_vite.ImportMapEntry, error) {
	// Install dist deps (cached: skips if package.json unchanged).
	buildPkgsDir := filepath.Join(workingPath, "build", "web-pkgs")
	if err := npm.EnsureBunInstall(ctx, le, workingPath, filepath.Join(distSourcePath, "dist/deps/package.json"), buildPkgsDir); err != nil {
		return nil, nil, nil, err
	}

	// Get web package refs with resolved source paths.
	refs := web_pkg_external.GetBldrDistWebPkgRefs(buildPkgsDir, distSourcePath)

	// Build web packages with Vite via a one-shot process.
	le.Debug("building web packages with vite")
	outWebPkgsPath := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
	viteWorkingPath := filepath.Join(workingPath, "vite-web-pkgs")

	var importMapEntries []web_pkg_vite.ImportMapEntry
	var srcFiles []string
	err := web_pkg_vite.RunOneShot(ctx, le, distSourcePath, sourcePath, viteWorkingPath, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
		_, builtSrcFiles, entries, buildErr := web_pkg_vite.BuildWebPkgsVite(
			ctx,
			le,
			sourcePath,
			refs,
			outWebPkgsPath,
			bldr_plugin.PluginWebPkgHttpPrefix,
			isRelease,
			client,
			filepath.Join(viteWorkingPath, "cache"),
		)
		srcFiles = builtSrcFiles
		importMapEntries = entries
		return buildErr
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return refs, srcFiles, importMapEntries, nil
}
