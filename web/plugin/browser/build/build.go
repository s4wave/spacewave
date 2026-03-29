//go:build !js

package web_plugin_browser_build

import (
	"context"
	"path"
	"path/filepath"

	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/web/entrypoint/browser/bundle"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// webPluginBrowserPkg is the repo sub-dir for the browser plugin entrypoint.
const webPluginBrowserPkg = "web/plugin/browser"

// BuildWebPluginBrowserEntrypoint builds the .mjs web browser plugin shim.
//
// builds to outFile
func BuildWebPluginBrowserEntrypoint(ctx context.Context, le *logrus.Entry, bldrDistRoot, outFile string, minify bool) error {
	outFilename := filepath.Base(outFile)
	le.Infof("building %v", outFilename)

	opts := entrypoint_browser_bundle.BrowserBuildOpts(bldrDistRoot, minify)
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
