//go:build !js

package entrypoint_browser_bundle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	web_pkg_vite "github.com/s4wave/spacewave/bldr/web/pkg/vite"
	"github.com/sirupsen/logrus"
)

// ConfigFreeRendererOpts describes one internal renderer build through Vite.
type ConfigFreeRendererOpts struct {
	OutputDir     string
	PublicPath    string
	Defines       map[string]string
	ExtraExternal []string
	Minify        bool
	Sourcemaps    bool
}

// RendererResult contains normalized renderer provenance and outputs.
type RendererResult struct {
	InputFiles  []string
	OutputFiles []string
	CSSPaths    []string
	JSPath      string
}

func rendererProjectRoot(bldrDistRoot string) string {
	root := filepath.Clean(bldrDistRoot)
	for {
		if info, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && !info.IsDir() {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return filepath.Clean(bldrDistRoot)
		}
		root = parent
	}
}

func configFreeRendererRequest(bldrDistRoot, workingPath string, opts ConfigFreeRendererOpts) *bldr_vite.BuildRequest {
	sourcemapMode := "none"
	if opts.Sourcemaps {
		sourcemapMode = "external"
	}
	external := slices.Clone(web_pkg_external.BldrExternal)
	external = append(external, "tailwindcss")
	external = append(external, opts.ExtraExternal...)
	return &bldr_vite.BuildRequest{
		Mode:           "production",
		RootDir:        bldrDistRoot,
		ProjectRoot:    rendererProjectRoot(bldrDistRoot),
		OutDir:         opts.OutputDir,
		CacheDir:       filepath.Join(workingPath, "cache"),
		DistDir:        bldrDistRoot,
		PublicPath:     opts.PublicPath,
		ExternalPkgs:   external,
		JsMinification: opts.Minify,
		Entrypoints: []*bldr_vite.ViteBuildRequestEntrypoint{{
			Name:      "entrypoint",
			InputPath: "web/entrypoint/entrypoint.tsx",
		}},
		Defines:        opts.Defines,
		FlatEntryNames: true,
		SourcemapMode:  sourcemapMode,
	}
}

func rendererOutputDirRelative(buildDir, outputDir string) (string, error) {
	outputDirRel, err := filepath.Rel(buildDir, outputDir)
	if err != nil {
		return "", errors.Wrap(err, "relativize renderer output directory")
	}
	if outputDirRel == ".." || strings.HasPrefix(outputDirRel, ".."+string(filepath.Separator)) || filepath.IsAbs(outputDirRel) {
		return "", errors.New("renderer output directory escapes build directory")
	}
	return outputDirRel, nil
}

// BuildConfigFreeRenderer builds an internal renderer without project Vite configuration.
func BuildConfigFreeRenderer(
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
	workingPath := filepath.Join(stateDir, "vite-renderer")
	request := configFreeRendererRequest(bldrDistRoot, workingPath, opts)
	var response *bldr_vite.BuildResponse
	err = web_pkg_vite.RunOneShot(
		ctx,
		le,
		bldrDistRoot,
		bldrDistRoot,
		workingPath,
		func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
			var err error
			response, err = client.Build(ctx, request)
			if err != nil {
				return errors.Wrap(err, "config-free Vite renderer build")
			}
			if !response.GetSuccess() {
				return errors.New("config-free Vite renderer build failed: " + response.GetError())
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(opts.OutputDir, ".vite", "manifest.json")
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "remove internal Vite manifest")
	}
	_ = os.Remove(filepath.Dir(manifestPath))
	var entrypoint *bldr_vite.EntrypointOutput
	for _, output := range response.GetEntrypointOutputs() {
		if output.GetJsOutput() == "entrypoint.mjs" {
			entrypoint = output
			break
		}
	}
	if entrypoint == nil {
		return nil, errors.Errorf("renderer build produced no entrypoint.mjs role among %d entrypoints", len(response.GetEntrypointOutputs()))
	}

	inputs := make([]string, 0, len(response.GetInputFiles()))
	for _, input := range response.GetInputFiles() {
		inputs = append(inputs, filepath.Join(bldrDistRoot, input))
	}
	outputs := make([]string, 0, len(response.GetOutputFiles()))
	for _, output := range response.GetOutputFiles() {
		outputs = append(outputs, filepath.Join(outputDirRel, output))
	}
	css := make([]string, 0, len(entrypoint.GetCssOutputs())+len(response.GetGlobalCssFiles()))
	for _, output := range append(slices.Clone(entrypoint.GetCssOutputs()), response.GetGlobalCssFiles()...) {
		css = append(css, filepath.Join(outputDirRel, output))
	}
	return &RendererResult{
		InputFiles:  uniqueStrings(inputs),
		OutputFiles: uniqueStrings(outputs),
		CSSPaths:    uniqueStrings(css),
		JSPath:      filepath.Join(outputDirRel, entrypoint.GetJsOutput()),
	}, nil
}
