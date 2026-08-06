//go:build !js

package entrypoint_browser_bundle

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	"github.com/sirupsen/logrus"
)

const (
	rolldownBrowserCompilerID = "rolldown-one-shot"
	viteBrowserCompilerID     = "vite-one-shot"
	rendererBrowserCompilerID = "rolldown-vite-router"
)

var browserAssetLoaders = map[string]string{
	".wasm":  "asset",
	".woff":  "asset",
	".woff2": "asset",
	".png":   "asset",
	".jpg":   "asset",
	".jpeg":  "asset",
	".svg":   "asset",
	".gif":   "asset",
}

type browserScriptSpec struct {
	name           string
	inputPath      string
	entryFileNames string
	format         string
	globalName     string
	minify         bool
	sourcemaps     bool
	devMode        bool
}

func browserScriptRequest(bldrDistRoot, buildDir string, spec browserScriptSpec) *bldr_web_bundler_rolldown.BuildRequest {
	sourcemap := "none"
	if spec.sourcemaps {
		sourcemap = "inline"
	}
	return &bldr_web_bundler_rolldown.BuildRequest{
		WorkingDir:   buildDir,
		SourceRoot:   bldrDistRoot,
		OutputRoot:   buildDir,
		BldrDistRoot: bldrDistRoot,
		Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
			Name:      spec.name,
			InputPath: bldr.ResolveDistSourcePath(bldrDistRoot, spec.inputPath),
		}},
		Format:         spec.format,
		GlobalName:     spec.globalName,
		Platform:       "browser",
		Target:         "es2024",
		EntryFileNames: spec.entryFileNames,
		ChunkFileNames: "[name]-[hash].mjs",
		AssetFileNames: "[name]-[hash][extname]",
		Sourcemap:      sourcemap,
		Minify:         spec.minify,
		TreeShaking:    true,
		Banner:         DefaultBanner()["js"],
		Defines: map[string]string{
			"BLDR_IS_BROWSER": "true",
			"BLDR_DEBUG":      strconv.FormatBool(spec.devMode),
		},
		Loaders: browserAssetLoaders,
	}
}

func buildBrowserScript(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	buildDir string,
	spec browserScriptSpec,
) (*bldr_web_bundler_rolldown.BuildResult, error) {
	if stateDir == "" {
		stateDir = filepath.Join(buildDir, ".bun")
	}
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		browserScriptRequest(bldrDistRoot, buildDir, spec),
	)
	if err != nil {
		return nil, err
	}
	if result.GetEntrypointOutputs()[spec.name] == "" {
		return nil, errors.Errorf("%s build produced no entrypoint output", spec.name)
	}
	return result, nil
}

func resultOutputPaths(result *bldr_web_bundler_rolldown.BuildResult) []string {
	paths := make([]string, 0, len(result.GetOutputs()))
	for _, output := range result.GetOutputs() {
		paths = append(paths, filepath.Clean(output.GetPath()))
	}
	return uniqueStrings(paths)
}

func directRendererRequest(
	bldrDistRoot,
	buildDir string,
	opts ConfigFreeRendererOpts,
) *bldr_web_bundler_rolldown.BuildRequest {
	sourcemap := "none"
	if opts.Sourcemaps {
		sourcemap = "external"
	}
	external := slices.Clone(web_pkg_external.BldrExternal)
	external = append(external, "tailwindcss")
	external = append(external, opts.ExtraExternal...)
	return &bldr_web_bundler_rolldown.BuildRequest{
		WorkingDir:   buildDir,
		SourceRoot:   bldrDistRoot,
		OutputRoot:   opts.OutputDir,
		BldrDistRoot: bldrDistRoot,
		Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
			Name:      "entrypoint",
			InputPath: bldr.ResolveDistSourcePath(bldrDistRoot, "web/entrypoint/entrypoint.tsx"),
		}},
		Format:          "es",
		Platform:        "browser",
		Target:          "es2024",
		EntryFileNames:  "entrypoint.mjs",
		ChunkFileNames:  "[name]-[hash].mjs",
		AssetFileNames:  "[name]-[hash][extname]",
		PublicPath:      opts.PublicPath,
		Sourcemap:       sourcemap,
		Minify:          opts.Minify,
		TreeShaking:     true,
		Banner:          DefaultBanner()["js"],
		Defines:         opts.Defines,
		External:        external,
		Loaders:         browserAssetLoaders,
		RouteCssImports: true,
		CleanOutputDir:  false,
	}
}

// BuildRenderer routes JavaScript-only graphs through direct Rolldown and
// CSS-bearing graphs through config-free Vite.
func BuildRenderer(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	buildDir string,
	opts ConfigFreeRendererOpts,
) (*RendererResult, error) {
	outputDirRel, err := rendererOutputDirRelative(buildDir, opts.OutputDir)
	if err != nil {
		return nil, err
	}
	request := directRendererRequest(bldrDistRoot, buildDir, opts)
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		request,
	)
	if err != nil {
		return nil, err
	}
	if result.GetHasCssImports() {
		return BuildConfigFreeRenderer(ctx, le, stateDir, bldrDistRoot, buildDir, opts)
	}
	entrypoint := result.GetEntrypointOutputs()["entrypoint"]
	if entrypoint == "" {
		return nil, errors.New("renderer build produced no entrypoint.mjs role")
	}

	outputs := make([]string, 0, len(result.GetOutputs()))
	for _, output := range result.GetOutputs() {
		outputs = append(outputs, filepath.Join(outputDirRel, output.GetPath()))
	}
	return &RendererResult{
		InputFiles:  result.GetInputs(),
		OutputFiles: uniqueStrings(outputs),
		JSPath:      filepath.Join(outputDirRel, entrypoint),
	}, nil
}
