//go:build !js

package web_plugin_browser_build

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
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
	sourceMap := "none"
	if sourcemaps {
		sourceMap = "external"
	}
	outputRoot := filepath.Dir(outFile)
	entrypointName := strings.TrimSuffix(outFilename, filepath.Ext(outFilename))
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		outputRoot,
		bldrDistRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   outputRoot,
			SourceRoot:   bldrDistRoot,
			OutputRoot:   outputRoot,
			BldrDistRoot: bldrDistRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name: entrypointName,
				InputPath: bldr.ResolveDistSourcePath(
					bldrDistRoot,
					webPluginBrowserPkg,
					"web-plugin-browser.ts",
				),
			}},
			Format:         "es",
			Platform:       "browser",
			Target:         "es2024",
			EntryFileNames: outFilename,
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      sourceMap,
			Minify:         minify,
			TreeShaking:    true,
			Banner:         entrypoint_browser_bundle.DefaultBanner()["js"],
			Defines:        map[string]string{"BLDR_IS_BROWSER": "true"},
			Loaders: map[string]string{
				".wasm": "asset", ".woff": "asset", ".woff2": "asset",
				".png": "asset", ".jpg": "asset", ".jpeg": "asset",
				".svg": "asset", ".gif": "asset",
			},
		},
	)
	if err != nil {
		return err
	}
	if result.GetEntrypointOutputs()[entrypointName] != outFilename {
		return errors.Errorf("browser plugin output is %q, expected %q", result.GetEntrypointOutputs()[entrypointName], outFilename)
	}

	// build complete
	return nil
}
