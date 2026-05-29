//go:build !js

package web_runtime_goscript_build

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

const webRuntimeGoScriptDir = "web/runtime/goscript"

// BuildWebGoScriptPluginScript builds the web plugin runtime entrypoint script.
func BuildWebGoScriptPluginScript(
	_ context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outPath,
	mainPackagePath string,
	minify,
	sourcemaps bool,
) ([]string, error) {
	if strings.TrimSpace(mainPackagePath) == "" {
		return nil, errors.New("plugin-goscript: main package path cannot be empty")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript entrypoint work dir")
	}

	pluginJsDir := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir)
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	runtimeImport, err := relativeImportPath(workDir, filepath.Join(pluginJsDir, "plugin-goscript.ts"))
	if err != nil {
		return nil, err
	}
	mainImport := "@goscript/" + strings.Trim(mainPackagePath, "/") + "/plugin.gs.js"
	entrypoint := "import runGoScriptPlugin from " + strconv.Quote(runtimeImport) + "\n" +
		"import { main as pluginMain } from " + strconv.Quote(mainImport) + "\n\n" +
		"export default async function main(api) {\n" +
		"  await runGoScriptPlugin(api, pluginMain)\n" +
		"}\n"
	if err := os.WriteFile(entrypointPath, []byte(entrypoint), 0o644); err != nil {
		return nil, errors.Wrap(err, "write goscript entrypoint")
	}

	le.Infof("building plugin-goscript-entrypoint.ts to %v", filepath.Base(outPath))

	opts := entrypoint_browser_bundle.BrowserBuildOpts(workDir, minify, sourcemaps)
	opts.EntryPoints = []string{"plugin-goscript-entrypoint.ts"}
	opts.Outfile = outPath
	opts.Define["BLDR_IS_PLUGIN"] = "true"
	opts.Metafile = true
	opts.Write = true
	opts.Conditions = append(opts.Conditions, "browser")
	opts.Plugins = append(opts.Plugins, goScriptImportResolverPlugin(goScriptOutputRoot))

	if sourcemaps {
		opts.Sourcemap = esbuild_api.SourceMapInlineAndExternal
	}

	res := esbuild_api.Build(opts)
	if err := bldr_esbuild_build.BuildResultToErr(res); err != nil {
		return nil, err
	}
	if err := buildResultUndefinedImportToErr(res.Warnings); err != nil {
		return nil, err
	}
	inputPaths, err := buildInputPathsFromMetafile(workDir, res.Metafile)
	if err != nil {
		return nil, err
	}
	return inputPaths, nil
}

func buildResultUndefinedImportToErr(warnings []esbuild_api.Message) error {
	for _, warning := range warnings {
		if !isUndefinedImportWarning(warning) {
			continue
		}
		if warning.Location != nil && warning.Location.File != "" {
			return errors.Errorf("undefined GoScript import in %s: %s", warning.Location.File, warning.Text)
		}
		return errors.Errorf("undefined GoScript import: %s", warning.Text)
	}
	return nil
}

func isUndefinedImportWarning(warning esbuild_api.Message) bool {
	return warning.ID == "import-is-undefined" ||
		strings.Contains(warning.Text, "will always be undefined because there is no matching export")
}

func relativeImportPath(fromDir, toPath string) (string, error) {
	rel, err := filepath.Rel(fromDir, toPath)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel, nil
}

func goScriptImportResolverPlugin(outputRoot string) esbuild_api.Plugin {
	return esbuild_api.Plugin{
		Name: "goscript-import-resolver",
		Setup: func(build esbuild_api.PluginBuild) {
			build.OnResolve(esbuild_api.OnResolveOptions{Filter: `^@goscript/.+`}, func(args esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
				rel := strings.TrimPrefix(args.Path, "@goscript/")
				return resolveGoScriptImport(outputRoot, rel)
			})
			build.OnResolve(esbuild_api.OnResolveOptions{Filter: `^\.\.?/.+\.js$`}, func(args esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
				if args.ResolveDir == "" {
					return esbuild_api.OnResolveResult{}, nil
				}
				path := filepath.Join(args.ResolveDir, filepath.FromSlash(args.Path))
				if resolved := existingTypeScriptSibling(path); resolved != "" {
					return esbuild_api.OnResolveResult{Path: resolved}, nil
				}
				return esbuild_api.OnResolveResult{}, nil
			})
		},
	}
}

func resolveGoScriptImport(outputRoot, rel string) (esbuild_api.OnResolveResult, error) {
	path := filepath.Join(outputRoot, "@goscript", filepath.FromSlash(rel))
	if resolved := existingTypeScriptSibling(path); resolved != "" {
		return esbuild_api.OnResolveResult{Path: resolved}, nil
	}
	if _, err := os.Stat(path); err == nil {
		return esbuild_api.OnResolveResult{Path: path}, nil
	}
	return esbuild_api.OnResolveResult{}, nil
}

func existingTypeScriptSibling(path string) string {
	if before, ok := strings.CutSuffix(path, ".js"); ok {
		tsPath := before + ".ts"
		if _, err := os.Stat(tsPath); err == nil {
			return tsPath
		}
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func buildInputPathsFromMetafile(absWorkingDir, metafile string) ([]string, error) {
	if metafile == "" {
		return nil, nil
	}
	var parser fastjson.Parser
	value, err := parser.Parse(metafile)
	if err != nil {
		return nil, errors.Wrap(err, "parse esbuild metafile")
	}
	inputs := value.GetObject("inputs")
	if inputs == nil {
		return nil, nil
	}

	seen := make(map[string]struct{}, inputs.Len())
	inputPaths := make([]string, 0, inputs.Len())
	inputs.Visit(func(key []byte, _ *fastjson.Value) {
		inputPath := string(key)
		if inputPath == "" || strings.HasPrefix(inputPath, "<") {
			return
		}
		if !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(absWorkingDir, inputPath)
		}
		inputPath = filepath.Clean(inputPath)
		if _, ok := seen[inputPath]; ok {
			return
		}
		if _, err := os.Stat(inputPath); err != nil {
			return
		}
		seen[inputPath] = struct{}{}
		inputPaths = append(inputPaths, inputPath)
	})
	slices.Sort(inputPaths)
	return inputPaths, nil
}
