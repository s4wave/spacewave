//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

const electronRequireBanner = "const require = (await import('node:module')).createRequire(import.meta.url);const __filename = (await import('node:url')).fileURLToPath(import.meta.url);const __dirname = (await import('node:path')).dirname(__filename);"

func buildElectronScript(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	buildDir,
	name,
	inputPath,
	outputName,
	format string,
	minify,
	devMode,
	requireGlobals bool,
) error {
	sourcemap := "external"
	if minify {
		sourcemap = "none"
	}
	banner := entrypoint_browser_bundle.DefaultBanner()["js"]
	if requireGlobals {
		banner += "\n" + electronRequireBanner
	}
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   buildDir,
			SourceRoot:   bldrDistRoot,
			OutputRoot:   buildDir,
			BldrDistRoot: bldrDistRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      name,
				InputPath: bldr.ResolveDistSourcePath(bldrDistRoot, inputPath),
			}},
			Format:         format,
			Platform:       "node",
			Target:         "es2024",
			EntryFileNames: outputName,
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      sourcemap,
			Minify:         minify,
			TreeShaking:    true,
			Banner:         banner,
			Defines:        ElectronDefine(devMode),
			External:       []string{"electron"},
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
	if result.GetEntrypointOutputs()[name] != filepath.Clean(outputName) {
		return errors.Errorf("Electron %s output is %q", name, result.GetEntrypointOutputs()[name])
	}
	return nil
}
