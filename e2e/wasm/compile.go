//go:build !js

package wasm

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	"github.com/sirupsen/logrus"
)

// CompiledScripts maps base filenames to their served URL paths.
// e.g. "navigate-hash.ts" -> "/e2e/navigate-hash.mjs"
type CompiledScripts map[string]string

// CompileTestScripts discovers and bundles *.ts files as ESM modules.
func CompileTestScripts(dir, outDir string) (CompiledScripts, error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}
	return CompileTestScriptsFor(dir, outDir, repoRoot, filepath.Join(repoRoot, "vendor"))
}

// CompileTestScriptsFor bundles *.ts files with explicit source aliases.
// alphaRoot is the Spacewave source root and vendorDir is the caller's Go
// vendor directory, which may belong to a downstream repository.
func CompileTestScriptsFor(dir, outDir, alphaRoot, vendorDir string) (CompiledScripts, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.ts"))
	if err != nil {
		return nil, errors.Wrap(err, "glob ts files")
	}
	if len(matches) == 0 {
		return CompiledScripts{}, nil
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create output dir")
	}

	entrypoints := make([]*bldr_web_bundler_rolldown.Entrypoint, 0, len(matches))
	scripts := make(CompiledScripts, len(matches))
	for _, inputPath := range matches {
		name := filepath.Base(inputPath)
		entrypointName := strings.TrimSuffix(name, ".ts")
		absoluteInputPath, err := filepath.Abs(inputPath)
		if err != nil {
			return nil, errors.Wrapf(err, "resolve %s", name)
		}
		entrypoints = append(entrypoints, &bldr_web_bundler_rolldown.Entrypoint{
			Name:      entrypointName,
			InputPath: absoluteInputPath,
		})
		scripts[name] = "/e2e/" + entrypointName + ".mjs"
	}

	spacewaveVendor := filepath.Join(vendorDir, "github.com", "s4wave", "spacewave")
	result, err := bldr_web_bundler_rolldown.Build(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		outDir,
		alphaRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:     outDir,
			SourceRoot:     alphaRoot,
			OutputRoot:     outDir,
			BldrDistRoot:   alphaRoot,
			Entrypoints:    entrypoints,
			Format:         "es",
			Platform:       "browser",
			Target:         "es2022",
			CodeSplitting:  true,
			EntryFileNames: "[name].mjs",
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      "none",
			TreeShaking:    true,
			External:       BuildExternalList(),
			Aliases: map[string]string{
				"@s4wave/sdk":     filepath.Join(alphaRoot, "sdk", "index.ts"),
				"@s4wave/app":     filepath.Join(alphaRoot, "app"),
				"@s4wave/web":     filepath.Join(alphaRoot, "web"),
				"@aptre/bldr-sdk": filepath.Join(spacewaveVendor, "bldr", "sdk", "plugin.ts"),
			},
			PrefixAliases: map[string]string{
				"@go/":             vendorDir,
				"@s4wave/sdk/":     filepath.Join(alphaRoot, "sdk"),
				"@s4wave/core/":    filepath.Join(alphaRoot, "core"),
				"@s4wave/app/":     filepath.Join(alphaRoot, "app"),
				"@s4wave/web/":     filepath.Join(alphaRoot, "web"),
				"@aptre/bldr-sdk/": filepath.Join(spacewaveVendor, "bldr", "sdk"),
			},
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "bundle e2e scripts")
	}
	for _, entrypoint := range entrypoints {
		if got := result.GetEntrypointOutputs()[entrypoint.GetName()]; got != entrypoint.GetName()+".mjs" {
			return nil, errors.Errorf("e2e output for %q is %q", entrypoint.GetName(), got)
		}
	}
	return scripts, nil
}

// BuildExternalList returns packages resolved by the app import map.
func BuildExternalList() []string {
	external := make([]string, 0, len(web_pkg_external.BldrExternal))
	for _, pkg := range web_pkg_external.BldrExternal {
		if pkg == "@aptre/protobuf-es-lite" {
			continue
		}
		external = append(external, pkg)
	}
	return external
}
