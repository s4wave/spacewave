//go:build !js

package web_plugin_browser_build

import (
	"context"
	"path"
	"path/filepath"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	bldr "github.com/s4wave/spacewave/bldr"
	"github.com/s4wave/spacewave/bldr/util/npm"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

// webPluginBrowserPkg is the repo sub-dir for the browser plugin entrypoint.
const webPluginBrowserPkg = "web/plugin/browser"

// BuildWebPluginBrowserEntrypoint builds the .mjs web browser plugin shim.
//
// builds to outFile
func BuildWebPluginBrowserEntrypoint(ctx context.Context, le *logrus.Entry, bldrDistRoot, outFile string, minify, sourcemaps bool) error {
	outFilename := filepath.Base(outFile)
	le.Infof("building %v", outFilename)

	depsDir := filepath.Join(filepath.Dir(outFile), "runtime-deps")
	if err := npm.EnsureBunInstall(ctx, le, filepath.Dir(outFile), bldr.ResolveDistSourcePath(bldrDistRoot, "dist", "deps", "package.json"), depsDir); err != nil {
		return err
	}
	if err := npm.EnsureNodeModulesLink(bldrDistRoot, depsDir); err != nil {
		return err
	}

	opts := entrypoint_browser_bundle.BrowserBuildOpts(bldrDistRoot, minify, sourcemaps)
	opts.NodePaths = append(opts.NodePaths, filepath.Join(depsDir, "node_modules"))
	opts.EntryPoints = []string{path.Join(webPluginBrowserPkg, "web-plugin-browser.ts")}
	opts.Outfile = outFile
	opts.Write = true

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return err
	}

	// build complete
	return nil
}
