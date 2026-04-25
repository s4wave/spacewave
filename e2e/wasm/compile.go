//go:build !js

package wasm

import (
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
)

// CompiledScripts maps base filenames to their served URL paths.
// e.g. "navigate-hash.ts" -> "/e2e/navigate-hash.mjs"
type CompiledScripts map[string]string

// CompileTestScripts discovers *.ts files in dir, compiles each to an ESM
// module via esbuild, writes the output to outDir, and returns a map of
// base filename to served URL path.
//
// Uses gitroot.FindRepoRoot() to locate the alpha repo root and vendor
// directory. For cross-repo usage where the alpha source is vendored,
// use CompileTestScriptsFor instead.
func CompileTestScripts(dir, outDir string) (CompiledScripts, error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}
	return CompileTestScriptsFor(dir, outDir, repoRoot, filepath.Join(repoRoot, "vendor"))
}

// CompileTestScriptsFor discovers *.ts files in dir, compiles each to an
// ESM module via esbuild, writes the output to outDir, and returns a map
// of base filename to served URL path.
//
// alphaRoot is the root of the alpha source tree (for @s4wave/* aliases).
// vendorDir is the Go vendor directory (for @go/* and @aptre/* aliases).
// For alpha itself, alphaRoot is the repo root and vendorDir is repo/vendor.
// For downstream repos that vendor alpha, alphaRoot is the vendored alpha
// path and vendorDir is the downstream repo's vendor directory.
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

	plugin := BuildResolverPlugin(alphaRoot, vendorDir)
	external := BuildExternalList()

	scripts := make(CompiledScripts, len(matches))
	for _, path := range matches {
		name := filepath.Base(path)
		outName := strings.TrimSuffix(name, ".ts") + ".mjs"
		outPath := filepath.Join(outDir, outName)
		if err := CompileOneScript(path, outPath, plugin, external); err != nil {
			return nil, errors.Wrapf(err, "compile %s", name)
		}
		scripts[name] = "/e2e/" + outName
	}
	return scripts, nil
}

// BuildExternalList returns the list of packages to externalize so the
// browser resolves them via the app's import map.
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

// CompileOneScript bundles a single TS file to an ESM module.
func CompileOneScript(path, outPath string, plugin esbuild.Plugin, external []string) error {
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{path},
		Bundle:      true,
		Format:      esbuild.FormatESModule,
		Target:      esbuild.ES2022,
		TreeShaking: esbuild.TreeShakingTrue,
		Platform:    esbuild.PlatformBrowser,
		Outfile:     outPath,
		Write:       true,
		Plugins:     []esbuild.Plugin{plugin},
		External:    external,
	})

	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Text
		}
		return errors.Errorf("esbuild: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// BuildResolverPlugin creates an esbuild plugin that resolves TypeScript
// path aliases (@go/*, @s4wave/*, @aptre/*) to the appropriate source and
// vendor directories.
//
// alphaRoot is the root of the alpha source tree (sdk/, core/, app/, web/).
// vendorDir is the Go vendor directory containing @go/* and @aptre/* deps.
func BuildResolverPlugin(alphaRoot, vendorDir string) esbuild.Plugin {
	return esbuild.Plugin{
		Name: "e2e-resolver",
		Setup: func(build esbuild.PluginBuild) {
			// @go/* -> vendor/*
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@go/`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				importPath := strings.TrimPrefix(args.Path, "@go/")
				return ResolveWithTsFallback(filepath.Join(vendorDir, importPath))
			})

			// @s4wave/sdk/* -> alphaRoot/sdk/*
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@s4wave/sdk`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				rest := strings.TrimPrefix(args.Path, "@s4wave/sdk")
				if rest == "" {
					return ResolveWithTsFallback(filepath.Join(alphaRoot, "sdk", "index.ts"))
				}
				return ResolveWithTsFallback(filepath.Join(alphaRoot, "sdk", strings.TrimPrefix(rest, "/")))
			})

			// @s4wave/core/* -> alphaRoot/core/*
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@s4wave/core/`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				rest := strings.TrimPrefix(args.Path, "@s4wave/core/")
				return ResolveWithTsFallback(filepath.Join(alphaRoot, "core", rest))
			})

			// @s4wave/app/* -> alphaRoot/app/*
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@s4wave/app`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				rest := strings.TrimPrefix(args.Path, "@s4wave/app")
				if rest == "" {
					return ResolveWithTsFallback(filepath.Join(alphaRoot, "app"))
				}
				return ResolveWithTsFallback(filepath.Join(alphaRoot, "app", strings.TrimPrefix(rest, "/")))
			})

			// @s4wave/web/* -> alphaRoot/web/*
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@s4wave/web`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				rest := strings.TrimPrefix(args.Path, "@s4wave/web")
				if rest == "" {
					return ResolveWithTsFallback(filepath.Join(alphaRoot, "web"))
				}
				return ResolveWithTsFallback(filepath.Join(alphaRoot, "web", strings.TrimPrefix(rest, "/")))
			})

			// @aptre/bldr-sdk -> vendor bldr SDK
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@aptre/bldr-sdk`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				rest := strings.TrimPrefix(args.Path, "@aptre/bldr-sdk")
				if rest == "" {
					rest = "/plugin.ts"
				}
				return ResolveWithTsFallback(filepath.Join(vendorDir, "github.com/s4wave/spacewave/bldr/sdk", rest))
			})

			// @aptre/bldr -> vendor bldr web (externalized, but resolve for type checking)
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@aptre/bldr$`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				return ResolveWithTsFallback(filepath.Join(vendorDir, "github.com/s4wave/spacewave/bldr/web/bldr/index.js"))
			})

			// @aptre/bldr-react -> vendor bldr-react (externalized, but resolve for type checking)
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^@aptre/bldr-react`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				return ResolveWithTsFallback(filepath.Join(vendorDir, "github.com/s4wave/spacewave/bldr/web/bldr-react/index.js"))
			})
		},
	}
}

// ResolveWithTsFallback resolves a path, trying .ts extension if .js was requested.
func ResolveWithTsFallback(resolved string) (esbuild.OnResolveResult, error) {
	if before, ok := strings.CutSuffix(resolved, ".js"); ok {
		tsPath := before + ".ts"
		if _, err := os.Stat(tsPath); err == nil {
			return esbuild.OnResolveResult{Path: tsPath}, nil
		}
	}
	if _, err := os.Stat(resolved); err == nil {
		return esbuild.OnResolveResult{Path: resolved}, nil
	}
	return esbuild.OnResolveResult{}, nil
}
