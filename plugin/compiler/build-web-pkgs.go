package bldr_plugin_compiler

import (
	"context"
	"path/filepath"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/bldr/util/npm"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
	web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"
	"github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/sirupsen/logrus"
)

// BuildDirectWebPkgs builds the BldrExternal web packages directly using esbuild.
//
// This is used when web packages are declared but no esbuild/vite sub-manifests
// exist to build them (e.g. in the saucer flow).
func BuildDirectWebPkgs(
	ctx context.Context,
	le *logrus.Entry,
	distSourcePath string,
	workingPath string,
	outAssetsPath string,
	isRelease bool,
) (web_pkg.WebPkgRefSlice, error) {
	// Create a temporary directory for bun install.
	buildPkgsDir := filepath.Join(workingPath, "build", "web-pkgs")
	if err := fsutil.CleanCreateDir(buildPkgsDir); err != nil {
		return nil, err
	}

	// Copy dist/deps/package.json for installing npm dependencies.
	if err := fsutil.CopyFile(
		filepath.Join(buildPkgsDir, "package.json"),
		filepath.Join(distSourcePath, "dist/deps/package.json"),
		0o644,
	); err != nil {
		return nil, err
	}

	// Run bun install to get react, react-dom, etc.
	le.Debug("installing npm deps for web packages")
	cmd, err := npm.BunInstall(ctx, le, workingPath, "--cwd", buildPkgsDir)
	if err != nil {
		return nil, err
	}
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return nil, err
	}

	// Get web package refs with resolved source paths.
	refs := web_pkg_external.GetBldrDistWebPkgRefs(buildPkgsDir, distSourcePath)

	// Build web packages with esbuild.
	le.Debug("building web packages with esbuild")
	outWebPkgsPath := filepath.Join(outAssetsPath, bldr_plugin.PluginAssetsWebPkgsDir)
	_, _, err = web_pkg_esbuild.BuildWebPkgsEsbuild(
		ctx,
		le,
		buildPkgsDir,
		refs,
		outWebPkgsPath,
		bldr_plugin.PluginWebPkgHttpPrefix,
		isRelease,
	)
	if err != nil {
		return nil, err
	}

	return refs, nil
}
