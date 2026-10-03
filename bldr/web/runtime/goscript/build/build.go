//go:build !js

package web_runtime_goscript_build

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr"
	bldr_buildbudget "github.com/s4wave/spacewave/bldr/util/buildbudget"
	bldr_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

const (
	webRuntimeGoScriptDir = "web/runtime/goscript"

	GoScriptSharedWebPkgID       = "@s4wave/goscript-shared"
	goScriptBundleReportFilename = "plugin-goscript-bundle-report.json"
)

// GoScriptMinify selects how Rolldown compacts a GoScript bundle.
type GoScriptMinify int

const (
	// GoScriptMinifyNone leaves the bundle readable.
	GoScriptMinifyNone GoScriptMinify = iota
	// GoScriptMinifyMangle mangles names and removes whitespace but skips the
	// compress pass, which dominates minify time on large bundles.
	GoScriptMinifyMangle
	// GoScriptMinifyFull also runs the compress pass.
	GoScriptMinifyFull
)

// goScriptMinifyFrom maps a minify flag to the full or no minify level.
func goScriptMinifyFrom(minify bool) GoScriptMinify {
	if minify {
		return GoScriptMinifyFull
	}
	return GoScriptMinifyNone
}

// GoScriptSharedImportMap maps a local @goscript import to the provider URL
// that serves the shared module.
type GoScriptSharedImportMap map[string]string

// GoScriptSharedBundleOptions configures shared provider publication and
// consumer externalization.
type GoScriptSharedBundleOptions struct {
	WebPkgID string
	Enabled  bool
}

type goScriptBundleReport struct {
	SchemaVersion    int                        `json:"schemaVersion"`
	OutputPath       string                     `json:"outputPath"`
	OutputBytes      int64                      `json:"outputBytes"`
	TotalOutputBytes int64                      `json:"totalOutputBytes"`
	OutputFileCount  int                        `json:"outputFileCount"`
	OutputFiles      []goScriptBundleOutputFile `json:"outputFiles"`
	Minify           bool                       `json:"minify"`
	Sourcemaps       bool                       `json:"sourcemaps"`
	CodeSplitting    bool                       `json:"codeSplitting"`
	InputCount       int                        `json:"inputCount"`
	InputPaths       []string                   `json:"inputPaths"`
}

type goScriptBundleOutputFile struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// BuildWebGoScriptPluginScript builds the web plugin runtime entrypoint script.
func BuildWebGoScriptPluginScript(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outPath,
	mainPackagePath string,
	minify,
	sourcemaps,
	codeSplitting bool,
) ([]string, error) {
	return BuildWebGoScriptPluginScriptWithOptions(
		ctx,
		le,
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		mainPackagePath,
		goScriptMinifyFrom(minify),
		sourcemaps,
		codeSplitting,
		GoScriptSharedBundleOptions{},
	)
}

// BuildWebGoScriptPluginScriptWithOptions builds the web plugin runtime entrypoint script.
func BuildWebGoScriptPluginScriptWithOptions(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outPath,
	mainPackagePath string,
	minify GoScriptMinify,
	sourcemaps,
	codeSplitting bool,
	sharedOptions GoScriptSharedBundleOptions,
) ([]string, error) {
	return buildWebGoScriptPluginScript(
		ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, outPath,
		mainPackagePath, "plugin-goscript.ts", minify, sourcemaps,
		codeSplitting, sharedOptions,
	)
}

// BuildWebGoScriptCloudflarePluginScript builds the Cloudflare Workers plugin
// runtime entrypoint script. Uses the Worker host runtime instead of the
// browser MessagePort runtime.
func BuildWebGoScriptCloudflarePluginScript(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outPath,
	mainPackagePath string,
	minify GoScriptMinify,
	sourcemaps,
	codeSplitting bool,
	sharedOptions GoScriptSharedBundleOptions,
) ([]string, error) {
	return buildWebGoScriptPluginScript(
		ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, outPath,
		mainPackagePath, "plugin-goscript-cloudflare.ts", minify, sourcemaps,
		codeSplitting, sharedOptions,
	)
}

func buildWebGoScriptPluginScript(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot, workDir, goScriptOutputRoot, outPath, mainPackagePath,
	runtimeFile string,
	minify GoScriptMinify,
	sourcemaps, codeSplitting bool,
	sharedOptions GoScriptSharedBundleOptions,
) ([]string, error) {
	// Require a main package before preparing the plugin entrypoint.
	if strings.TrimSpace(mainPackagePath) == "" {
		return nil, errors.New("plugin-goscript: main package path cannot be empty")
	}

	// Create the work directory for the plugin entrypoint.
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript entrypoint work dir")
	}

	// Resolve the runtime and main module imports for the plugin entrypoint.
	pluginJsDir := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir)
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	runtimeImport, err := relativeImportPath(workDir, filepath.Join(pluginJsDir, runtimeFile))
	if err != nil {
		return nil, err
	}
	mainImport := "@goscript/" + strings.Trim(mainPackagePath, "/") + "/plugin.gs.js"

	// The entrypoint forwards the runtime env, which carries the document's
	// storage selection, so the plugin's os.Getenv sees it.
	entrypoint := "import runGoScriptPlugin from " + strconv.Quote(runtimeImport) + "\n" +
		"import { main as pluginMain } from " + strconv.Quote(mainImport) + "\n\n" +
		"export default function main(api, _abortSignal, env) {\n" +
		"  return runGoScriptPlugin(api, () => Promise.resolve(pluginMain), env)\n" +
		"}\n"
	if codeSplitting {
		entrypoint = "import runGoScriptPlugin from " + strconv.Quote(runtimeImport) + "\n\n" +
			"export default function main(api, _abortSignal, env) {\n" +
			"  return runGoScriptPlugin(api, async () => (await import(" + strconv.Quote(mainImport) + ")).main, env)\n" +
			"}\n"
	}
	if err := os.WriteFile(entrypointPath, []byte(entrypoint), 0o644); err != nil {
		return nil, errors.Wrap(err, "write goscript entrypoint")
	}

	return runRolldownGoScriptBundle(ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, entrypointPath, outPath, minify, sourcemaps, codeSplitting, sharedOptions)
}

// BuildWebGoScriptRuntimeScript builds the browser shell runtime entrypoint.
func BuildWebGoScriptRuntimeScript(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outPath,
	mainPackagePath string,
	minify,
	sourcemaps,
	codeSplitting bool,
) ([]string, error) {
	// Require a main package before preparing the browser runtime entrypoint.
	if strings.TrimSpace(mainPackagePath) == "" {
		return nil, errors.New("runtime-goscript: main package path cannot be empty")
	}

	// Create the work directory for the browser runtime entrypoint.
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript runtime work dir")
	}

	// Resolve the runtime and main module imports for the browser entrypoint.
	runtimeJsDir := filepath.Join(bldrDistRoot, "web/entrypoint/browser")
	entrypointPath := filepath.Join(workDir, "runtime-goscript-entrypoint.ts")
	runtimeImport, err := relativeImportPath(workDir, filepath.Join(runtimeJsDir, "runtime-goscript.ts"))
	if err != nil {
		return nil, err
	}
	mainImport := "@goscript/" + strings.Trim(mainPackagePath, "/") + "/main.gs.js"

	// Write the browser entrypoint with the selected main module loading policy.
	entrypoint := "import runGoScriptRuntime from " + strconv.Quote(runtimeImport) + "\n" +
		"import { main as distMain } from " + strconv.Quote(mainImport) + "\n\n" +
		"runGoScriptRuntime(() => Promise.resolve(distMain))\n"
	if codeSplitting {
		entrypoint = "import runGoScriptRuntime from " + strconv.Quote(runtimeImport) + "\n\n" +
			"runGoScriptRuntime(async () => (await import(" + strconv.Quote(mainImport) + ")).main)\n"
	}
	if err := os.WriteFile(entrypointPath, []byte(entrypoint), 0o644); err != nil {
		return nil, errors.Wrap(err, "write goscript runtime entrypoint")
	}

	return runRolldownGoScriptBundle(ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, entrypointPath, outPath, goScriptMinifyFrom(minify), sourcemaps, codeSplitting, GoScriptSharedBundleOptions{})
}

// BuildWebGoScriptSharedProviderScript builds the shared GoScript provider web package.
func BuildWebGoScriptSharedProviderScript(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outWebPkgPath,
	webPkgID string,
	minify GoScriptMinify,
	sourcemaps bool,
) (GoScriptSharedImportMap, []string, error) {
	// Require a web package ID before preparing the shared provider.
	if strings.TrimSpace(webPkgID) == "" {
		return nil, nil, errors.New("goscript shared provider: web pkg id cannot be empty")
	}

	// Create the work directory for shared provider entrypoints.
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, nil, errors.Wrap(err, "create goscript shared provider work dir")
	}

	// Generate shared module entrypoints and require at least one module.
	entrypoints, importMap, err := writeGoScriptSharedProviderEntrypoints(workDir, goScriptOutputRoot, webPkgID)
	if err != nil {
		return nil, nil, err
	}
	if len(entrypoints) == 0 {
		return nil, nil, errors.New("goscript shared provider: no shared modules found")
	}
	return runRolldownGoScriptSharedProvider(ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, outWebPkgPath, entrypoints, importMap, minify, sourcemaps)
}

func runRolldownGoScriptBundle(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	entrypointPath,
	outPath string,
	minify GoScriptMinify,
	sourcemaps,
	codeSplitting bool,
	sharedOptions GoScriptSharedBundleOptions,
) ([]string, error) {
	// Prepare the output directory for the GoScript bundle.
	le.Infof("building plugin-goscript-entrypoint.ts with Rolldown/Oxc to %v", filepath.Base(outPath))
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript bundle output dir")
	}

	// Configure Rolldown for the browser bundle and shared imports.
	request := &bldr_rolldown.BuildRequest{
		WorkingDir:         workDir,
		SourceRoot:         resolveGoScriptSourceRoot(bldrDistRoot),
		OutputRoot:         filepath.Dir(outPath),
		BldrDistRoot:       bldrDistRoot,
		Format:             "es",
		Platform:           "browser",
		Target:             "es2024",
		EntryFileNames:     filepath.Base(outPath),
		ChunkFileNames:     "chunks/[name]-[hash].mjs",
		AssetFileNames:     "assets/[name]-[hash][extname]",
		CodeSplitting:      codeSplitting,
		Sourcemap:          goScriptSourceMapPolicy(sourcemaps, codeSplitting),
		Minify:             minify != GoScriptMinifyNone,
		MinifySkipCompress: minify == GoScriptMinifyMangle,
		TreeShaking:        true,
		Banner:             entrypoint_browser_bundle.DefaultBanner()["js"],
		Defines: map[string]string{
			"BLDR_IS_BROWSER": "true",
			"BLDR_IS_PLUGIN":  "true",
		},
		Entrypoints: []*bldr_rolldown.Entrypoint{{
			Name:      "entrypoint",
			InputPath: entrypointPath,
		}},
		Goscript: &bldr_rolldown.GoScriptPolicy{
			OutputRoot:            goScriptOutputRoot,
			SharedExternalImports: sharedOptions.Enabled,
			SharedImportUrlPrefix: sharedImportURLPrefix(sharedWebPkgID(sharedOptions.WebPkgID)),
		},
	}

	// Reserve the build budget for the GoScript bundle.
	budget, err := bldr_buildbudget.Default()
	if err != nil {
		return nil, err
	}
	permit, err := budget.Acquire(ctx, bldr_buildbudget.GoScriptBundleWeight)
	if err != nil {
		return nil, err
	}
	defer permit.Release()

	// Build the GoScript bundle with Rolldown.
	stateDir := filepath.Join(workDir, "..", "..", "bun")
	result, err := bldr_rolldown.Build(ctx, le, stateDir, bldrDistRoot, request)
	if err != nil {
		return nil, err
	}

	// Record the GoScript bundle inputs and output sizes.
	if err := writeGoScriptBundleReport(GoScriptBundleReportPath(workDir), outPath, result.Inputs, minify != GoScriptMinifyNone, sourcemaps, codeSplitting); err != nil {
		return nil, err
	}
	return result.Inputs, nil
}

func runRolldownGoScriptSharedProvider(
	ctx context.Context,
	le *logrus.Entry,
	bldrDistRoot,
	workDir,
	goScriptOutputRoot,
	outWebPkgPath string,
	entrypoints map[string]string,
	importMap GoScriptSharedImportMap,
	minify GoScriptMinify,
	sourcemaps bool,
) (GoScriptSharedImportMap, []string, error) {
	// Prepare the output directory for the shared GoScript provider.
	le.Infof("building shared GoScript provider with Rolldown/Oxc to %v", outWebPkgPath)
	if err := os.MkdirAll(outWebPkgPath, 0o755); err != nil {
		return nil, nil, errors.Wrap(err, "create goscript shared provider output dir")
	}

	// Order shared module entrypoints for a deterministic provider build.
	entrypointNames := make([]string, 0, len(entrypoints))
	for name := range entrypoints {
		entrypointNames = append(entrypointNames, name)
	}
	slices.Sort(entrypointNames)
	rolldownEntrypoints := make([]*bldr_rolldown.Entrypoint, 0, len(entrypointNames))
	for _, name := range entrypointNames {
		rolldownEntrypoints = append(rolldownEntrypoints, &bldr_rolldown.Entrypoint{
			Name:      name,
			InputPath: entrypoints[name],
		})
	}

	// Configure Rolldown to emit the shared provider modules and chunks.
	request := &bldr_rolldown.BuildRequest{
		WorkingDir:         workDir,
		SourceRoot:         resolveGoScriptSourceRoot(bldrDistRoot),
		OutputRoot:         outWebPkgPath,
		BldrDistRoot:       bldrDistRoot,
		Format:             "es",
		Platform:           "browser",
		Target:             "es2024",
		EntryFileNames:     "[name].mjs",
		ChunkFileNames:     "chunks/[name]-[hash].mjs",
		AssetFileNames:     "assets/[name]-[hash][extname]",
		CodeSplitting:      true,
		Sourcemap:          goScriptSourceMapPolicy(sourcemaps, true),
		Minify:             minify != GoScriptMinifyNone,
		MinifySkipCompress: minify == GoScriptMinifyMangle,
		TreeShaking:        true,
		Banner:             entrypoint_browser_bundle.DefaultBanner()["js"],
		Defines: map[string]string{
			"BLDR_IS_BROWSER": "true",
			"BLDR_IS_PLUGIN":  "true",
		},
		Entrypoints: rolldownEntrypoints,
		Goscript: &bldr_rolldown.GoScriptPolicy{
			OutputRoot: goScriptOutputRoot,
		},
	}

	// Build the shared provider modules with Rolldown.
	stateDir := filepath.Join(workDir, "..", "..", "bun")
	result, err := bldr_rolldown.Build(ctx, le, stateDir, bldrDistRoot, request)
	if err != nil {
		return nil, nil, err
	}

	// Record the shared provider inputs and output sizes.
	if err := writeGoScriptBundleDirectoryReport(GoScriptBundleReportPath(workDir), outWebPkgPath, result.Inputs, minify != GoScriptMinifyNone, sourcemaps, true); err != nil {
		return nil, nil, err
	}
	return importMap, result.Inputs, nil
}

func goScriptSourceMapPolicy(enabled, codeSplitting bool) string {
	if !enabled {
		return "none"
	}
	if codeSplitting {
		return "external"
	}
	return "both"
}

func writeGoScriptSharedProviderEntrypoints(workDir, goScriptOutputRoot, webPkgID string) (map[string]string, GoScriptSharedImportMap, error) {
	// Collect shared provider entrypoints and their public import URLs.
	goScriptRoot := filepath.Join(goScriptOutputRoot, "@goscript")
	entryRoot := filepath.Join(workDir, "goscript-shared-entrypoints")
	entrypoints := make(map[string]string)
	importMap := make(GoScriptSharedImportMap)
	err := filepath.WalkDir(goScriptRoot, func(filePath string, entry os.DirEntry, err error) error {
		// Propagate directory walk failures and skip non-TypeScript entries.
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") {
			return nil
		}

		// Identify shared modules by their path under the GoScript output root.
		rel, err := filepath.Rel(goScriptRoot, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isSharedGoScriptRel(rel) {
			return nil
		}

		// Write a re-export entrypoint for the shared GoScript module.
		moduleRel := strings.TrimSuffix(rel, ".ts") + ".js"
		entryName := strings.TrimSuffix(rel, ".ts")
		entryPath := filepath.Join(entryRoot, filepath.FromSlash(rel))
		importSource := "@goscript/" + moduleRel
		if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(entryPath, []byte("export * from "+strconv.Quote(importSource)+"\n"), 0o644); err != nil {
			return err
		}

		// Register the shared module entrypoint and its provider URL.
		entrypoints[entryName] = entryPath
		importMap[importSource] = sharedImportURL(webPkgID, moduleRel)
		return nil
	})

	// Treat a missing GoScript output root as an empty shared provider.
	if os.IsNotExist(err) {
		return entrypoints, importMap, nil
	}
	return entrypoints, importMap, err
}

func sharedWebPkgID(webPkgID string) string {
	if strings.TrimSpace(webPkgID) == "" {
		return GoScriptSharedWebPkgID
	}
	return webPkgID
}

func sharedImportURLPrefix(webPkgID string) string {
	return "/b/pkg/" + strings.Trim(webPkgID, "/") + "/"
}

func sharedImportURL(webPkgID, moduleRel string) string {
	return path.Join(sharedImportURLPrefix(webPkgID), strings.TrimSuffix(moduleRel, ".js")+".mjs")
}

func isSharedGoScriptRel(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	return rel != "" && !strings.HasPrefix(rel, "github.com/s4wave/")
}

// GoScriptBundleReportPath returns the build-private report path for a GoScript wrapper work directory.
func GoScriptBundleReportPath(workDir string) string {
	return filepath.Join(workDir, goScriptBundleReportFilename)
}

func resolveGoScriptSourceRoot(bldrDistRoot string) string {
	dir := bldrDistRoot
	for {
		// The extracted tool module vendors Bldr's dependencies only. Generated
		// application bindings resolve against the enclosing project module.
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && modfile.ModulePath(data) != bldr.DistGoMod {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return bldrDistRoot
		}
		dir = parent
	}
}

func relativeImportPath(fromDir, toPath string) (string, error) {
	// Resolve the module path relative to the importing directory.
	rel, err := filepath.Rel(fromDir, toPath)
	if err != nil {
		return "", err
	}

	// Format the relative path as a JavaScript import specifier.
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel, nil
}

func writeGoScriptBundleReport(reportPath, outPath string, inputPaths []string, minify, sourcemaps, codeSplitting bool) error {
	// Collect the entry output and any split GoScript chunks.
	outputPaths := []string{outPath}
	if codeSplitting {
		var err error
		outputPaths, err = scanGoScriptBundleOutputs(filepath.Dir(outPath))
		if err != nil {
			return err
		}
	}

	// Measure GoScript outputs and require the bundle entry in the report.
	outputFiles, totalBytes, err := statGoScriptBundleOutputs(outputPaths)
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(outputFiles, func(outputFile goScriptBundleOutputFile) bool {
		return outputFile.Path == outPath
	})
	if idx < 0 {
		return errors.Errorf("goscript bundle entry output missing from report: %s", outPath)
	}
	return writeGoScriptBundleReportFile(reportPath, goScriptBundleReport{
		OutputPath:       outPath,
		OutputBytes:      outputFiles[idx].Bytes,
		TotalOutputBytes: totalBytes,
		OutputFiles:      outputFiles,
		Minify:           minify,
		Sourcemaps:       sourcemaps,
		CodeSplitting:    codeSplitting,
		InputPaths:       inputPaths,
	})
}

func writeGoScriptBundleDirectoryReport(reportPath, outDir string, inputPaths []string, minify, sourcemaps, codeSplitting bool) error {
	// Collect and measure the shared provider output modules.
	outputPaths, err := scanGoScriptBundleOutputs(outDir)
	if err != nil {
		return err
	}
	outputFiles, totalBytes, err := statGoScriptBundleOutputs(outputPaths)
	if err != nil {
		return err
	}
	return writeGoScriptBundleReportFile(reportPath, goScriptBundleReport{
		OutputPath:       outDir,
		OutputBytes:      totalBytes,
		TotalOutputBytes: totalBytes,
		OutputFiles:      outputFiles,
		Minify:           minify,
		Sourcemaps:       sourcemaps,
		CodeSplitting:    codeSplitting,
		InputPaths:       inputPaths,
	})
}

func writeGoScriptBundleReportFile(reportPath string, report goScriptBundleReport) error {
	// Complete the GoScript report counts and write its JSON file.
	report.SchemaVersion = 1
	report.OutputFileCount = len(report.OutputFiles)
	report.InputCount = len(report.InputPaths)
	report.InputPaths = slices.Clone(report.InputPaths)
	if err := os.WriteFile(reportPath, marshalGoScriptBundleReport(report), 0o644); err != nil {
		return errors.Wrap(err, "write goscript bundle report")
	}
	return nil
}

// scanGoScriptBundleOutputs returns the sorted .mjs outputs under outDir.
func scanGoScriptBundleOutputs(outDir string) ([]string, error) {
	// Collect GoScript module outputs from the bundle directory.
	var outputPaths []string
	if err := filepath.WalkDir(outDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".mjs") {
			outputPaths = append(outputPaths, path)
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "scan goscript bundle outputs")
	}

	// Require module outputs and order their paths for the report.
	if len(outputPaths) == 0 {
		return nil, errors.Errorf("no goscript bundle outputs under %s", outDir)
	}
	slices.Sort(outputPaths)
	return outputPaths, nil
}

// statGoScriptBundleOutputs returns the size of each output and their total.
func statGoScriptBundleOutputs(outputPaths []string) ([]goScriptBundleOutputFile, int64, error) {
	outputFiles := make([]goScriptBundleOutputFile, 0, len(outputPaths))
	var totalBytes int64
	for _, outputPath := range outputPaths {
		info, err := os.Stat(outputPath)
		if err != nil {
			return nil, 0, errors.Wrap(err, "stat goscript bundle for report")
		}
		outputFiles = append(outputFiles, goScriptBundleOutputFile{
			Path:  outputPath,
			Bytes: info.Size(),
		})
		totalBytes += info.Size()
	}
	return outputFiles, totalBytes, nil
}

func marshalGoScriptBundleReport(report goScriptBundleReport) []byte {
	// Encode the GoScript report identity and output totals.
	var arena fastjson.Arena
	root := arena.NewObject()
	root.Set("schemaVersion", arena.NewNumberInt(report.SchemaVersion))
	root.Set("outputPath", arena.NewString(report.OutputPath))
	root.Set("outputBytes", arena.NewNumberString(strconv.FormatInt(report.OutputBytes, 10)))
	root.Set("totalOutputBytes", arena.NewNumberString(strconv.FormatInt(report.TotalOutputBytes, 10)))
	root.Set("outputFileCount", arena.NewNumberInt(report.OutputFileCount))

	// Encode the size and path of each GoScript output file.
	outputFiles := arena.NewArray()
	for idx, outputFile := range report.OutputFiles {
		outputFileValue := arena.NewObject()
		outputFileValue.Set("path", arena.NewString(outputFile.Path))
		outputFileValue.Set("bytes", arena.NewNumberString(strconv.FormatInt(outputFile.Bytes, 10)))
		outputFiles.SetArrayItem(idx, outputFileValue)
	}
	root.Set("outputFiles", outputFiles)

	// Encode the minification, sourcemap, and code splitting policies.
	if report.Minify {
		root.Set("minify", arena.NewTrue())
	} else {
		root.Set("minify", arena.NewFalse())
	}
	if report.Sourcemaps {
		root.Set("sourcemaps", arena.NewTrue())
	} else {
		root.Set("sourcemaps", arena.NewFalse())
	}
	if report.CodeSplitting {
		root.Set("codeSplitting", arena.NewTrue())
	} else {
		root.Set("codeSplitting", arena.NewFalse())
	}

	// Encode the GoScript input paths and serialize the report.
	root.Set("inputCount", arena.NewNumberInt(report.InputCount))
	inputPaths := arena.NewArray()
	for idx, inputPath := range report.InputPaths {
		inputPaths.SetArrayItem(idx, arena.NewString(inputPath))
	}
	root.Set("inputPaths", inputPaths)
	reportBytes := root.MarshalTo(nil)
	return append(reportBytes, '\n')
}
