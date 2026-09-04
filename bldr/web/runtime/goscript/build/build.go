//go:build !js

package web_runtime_goscript_build

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

const (
	webRuntimeGoScriptDir = "web/runtime/goscript"

	GoScriptSharedWebPkgID       = "@s4wave/goscript-shared"
	goScriptBundleReportFilename = "plugin-goscript-bundle-report.json"
)

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
	SchemaVersion        int                        `json:"schemaVersion"`
	OutputPath           string                     `json:"outputPath"`
	OutputBytes          int64                      `json:"outputBytes"`
	OutputGzipBytes      int64                      `json:"outputGzipBytes"`
	TotalOutputBytes     int64                      `json:"totalOutputBytes"`
	TotalOutputGzipBytes int64                      `json:"totalOutputGzipBytes"`
	OutputFileCount      int                        `json:"outputFileCount"`
	OutputFiles          []goScriptBundleOutputFile `json:"outputFiles"`
	Minify               bool                       `json:"minify"`
	Sourcemaps           bool                       `json:"sourcemaps"`
	CodeSplitting        bool                       `json:"codeSplitting"`
	InputCount           int                        `json:"inputCount"`
	InputPaths           []string                   `json:"inputPaths"`
}

type goScriptBundleOutputFile struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	GzipBytes int64  `json:"gzipBytes"`
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
		minify,
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
	minify,
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
	minify,
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
	minify, sourcemaps, codeSplitting bool,
	sharedOptions GoScriptSharedBundleOptions,
) ([]string, error) {
	if strings.TrimSpace(mainPackagePath) == "" {
		return nil, errors.New("plugin-goscript: main package path cannot be empty")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript entrypoint work dir")
	}

	pluginJsDir := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir)
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	runtimeImport, err := relativeImportPath(workDir, filepath.Join(pluginJsDir, runtimeFile))
	if err != nil {
		return nil, err
	}
	mainImport := "@goscript/" + strings.Trim(mainPackagePath, "/") + "/plugin.gs.js"
	entrypoint := "import runGoScriptPlugin from " + strconv.Quote(runtimeImport) + "\n" +
		"import { main as pluginMain } from " + strconv.Quote(mainImport) + "\n\n" +
		"export default function main(api) {\n" +
		"  return runGoScriptPlugin(api, () => Promise.resolve(pluginMain))\n" +
		"}\n"
	if codeSplitting {
		entrypoint = "import runGoScriptPlugin from " + strconv.Quote(runtimeImport) + "\n\n" +
			"export default function main(api) {\n" +
			"  return runGoScriptPlugin(api, async () => (await import(" + strconv.Quote(mainImport) + ")).main)\n" +
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
	if strings.TrimSpace(mainPackagePath) == "" {
		return nil, errors.New("runtime-goscript: main package path cannot be empty")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript runtime work dir")
	}

	runtimeJsDir := filepath.Join(bldrDistRoot, "web/entrypoint/browser")
	entrypointPath := filepath.Join(workDir, "runtime-goscript-entrypoint.ts")
	runtimeImport, err := relativeImportPath(workDir, filepath.Join(runtimeJsDir, "runtime-goscript.ts"))
	if err != nil {
		return nil, err
	}
	mainImport := "@goscript/" + strings.Trim(mainPackagePath, "/") + "/main.gs.js"
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

	return runRolldownGoScriptBundle(ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, entrypointPath, outPath, minify, sourcemaps, codeSplitting, GoScriptSharedBundleOptions{})
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
	minify,
	sourcemaps bool,
) (GoScriptSharedImportMap, []string, error) {
	if strings.TrimSpace(webPkgID) == "" {
		return nil, nil, errors.New("goscript shared provider: web pkg id cannot be empty")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, nil, errors.Wrap(err, "create goscript shared provider work dir")
	}
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
	minify,
	sourcemaps,
	codeSplitting bool,
	sharedOptions GoScriptSharedBundleOptions,
) ([]string, error) {
	le.Infof("building plugin-goscript-entrypoint.ts with Rolldown/Oxc to %v", filepath.Base(outPath))
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, errors.Wrap(err, "create goscript bundle output dir")
	}

	request := &bldr_rolldown.BuildRequest{
		WorkingDir:     workDir,
		SourceRoot:     resolveGoScriptSourceRoot(bldrDistRoot),
		OutputRoot:     filepath.Dir(outPath),
		BldrDistRoot:   bldrDistRoot,
		Format:         "es",
		Platform:       "browser",
		Target:         "es2024",
		EntryFileNames: filepath.Base(outPath),
		ChunkFileNames: "chunks/[name]-[hash].mjs",
		AssetFileNames: "assets/[name]-[hash][extname]",
		CodeSplitting:  codeSplitting,
		Sourcemap:      goScriptSourceMapPolicy(sourcemaps, codeSplitting),
		Minify:         minify,
		TreeShaking:    true,
		Banner:         entrypoint_browser_bundle.DefaultBanner()["js"],
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
	stateDir := filepath.Join(workDir, "..", "..", "bun")
	result, err := bldr_rolldown.Build(ctx, le, stateDir, bldrDistRoot, request)
	if err != nil {
		return nil, err
	}
	if err := writeGoScriptBundleReport(GoScriptBundleReportPath(workDir), outPath, result.Inputs, minify, sourcemaps, codeSplitting); err != nil {
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
	minify,
	sourcemaps bool,
) (GoScriptSharedImportMap, []string, error) {
	le.Infof("building shared GoScript provider with Rolldown/Oxc to %v", outWebPkgPath)
	if err := os.MkdirAll(outWebPkgPath, 0o755); err != nil {
		return nil, nil, errors.Wrap(err, "create goscript shared provider output dir")
	}

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
	request := &bldr_rolldown.BuildRequest{
		WorkingDir:     workDir,
		SourceRoot:     resolveGoScriptSourceRoot(bldrDistRoot),
		OutputRoot:     outWebPkgPath,
		BldrDistRoot:   bldrDistRoot,
		Format:         "es",
		Platform:       "browser",
		Target:         "es2024",
		EntryFileNames: "[name].mjs",
		ChunkFileNames: "chunks/[name]-[hash].mjs",
		AssetFileNames: "assets/[name]-[hash][extname]",
		CodeSplitting:  true,
		Sourcemap:      goScriptSourceMapPolicy(sourcemaps, true),
		Minify:         minify,
		TreeShaking:    true,
		Banner:         entrypoint_browser_bundle.DefaultBanner()["js"],
		Defines: map[string]string{
			"BLDR_IS_BROWSER": "true",
			"BLDR_IS_PLUGIN":  "true",
		},
		Entrypoints: rolldownEntrypoints,
		Goscript: &bldr_rolldown.GoScriptPolicy{
			OutputRoot: goScriptOutputRoot,
		},
	}
	stateDir := filepath.Join(workDir, "..", "..", "bun")
	result, err := bldr_rolldown.Build(ctx, le, stateDir, bldrDistRoot, request)
	if err != nil {
		return nil, nil, err
	}
	if err := writeGoScriptBundleDirectoryReport(GoScriptBundleReportPath(workDir), outWebPkgPath, result.Inputs, minify, sourcemaps, true); err != nil {
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
	goScriptRoot := filepath.Join(goScriptOutputRoot, "@goscript")
	entryRoot := filepath.Join(workDir, "goscript-shared-entrypoints")
	entrypoints := make(map[string]string)
	importMap := make(GoScriptSharedImportMap)
	err := filepath.WalkDir(goScriptRoot, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") {
			return nil
		}
		rel, err := filepath.Rel(goScriptRoot, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isSharedGoScriptRel(rel) {
			return nil
		}
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
		entrypoints[entryName] = entryPath
		importMap[importSource] = sharedImportURL(webPkgID, moduleRel)
		return nil
	})
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
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
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

func writeGoScriptBundleReport(reportPath, outPath string, inputPaths []string, minify, sourcemaps, codeSplitting bool) error {
	outputFiles, err := readGoScriptBundleOutputFiles(outPath, codeSplitting)
	if err != nil {
		return err
	}
	var totalBytes, totalGzipBytes int64
	for _, outputFile := range outputFiles {
		totalBytes += outputFile.Bytes
		totalGzipBytes += outputFile.GzipBytes
	}
	var entryFile goScriptBundleOutputFile
	for _, outputFile := range outputFiles {
		if outputFile.Path == outPath {
			entryFile = outputFile
			break
		}
	}
	if entryFile.Path == "" {
		return errors.Errorf("goscript bundle entry output missing from report: %s", outPath)
	}
	reportBytes := marshalGoScriptBundleReport(goScriptBundleReport{
		SchemaVersion:        1,
		OutputPath:           outPath,
		OutputBytes:          entryFile.Bytes,
		OutputGzipBytes:      entryFile.GzipBytes,
		TotalOutputBytes:     totalBytes,
		TotalOutputGzipBytes: totalGzipBytes,
		OutputFileCount:      len(outputFiles),
		OutputFiles:          outputFiles,
		Minify:               minify,
		Sourcemaps:           sourcemaps,
		CodeSplitting:        codeSplitting,
		InputCount:           len(inputPaths),
		InputPaths:           slices.Clone(inputPaths),
	})
	if err := os.WriteFile(reportPath, reportBytes, 0o644); err != nil {
		return errors.Wrap(err, "write goscript bundle report")
	}
	return nil
}

func writeGoScriptBundleDirectoryReport(reportPath, outDir string, inputPaths []string, minify, sourcemaps, codeSplitting bool) error {
	outputFiles, err := readGoScriptBundleDirectoryOutputFiles(outDir)
	if err != nil {
		return err
	}
	var totalBytes, totalGzipBytes int64
	for _, outputFile := range outputFiles {
		totalBytes += outputFile.Bytes
		totalGzipBytes += outputFile.GzipBytes
	}
	reportBytes := marshalGoScriptBundleReport(goScriptBundleReport{
		SchemaVersion:        1,
		OutputPath:           outDir,
		OutputBytes:          totalBytes,
		OutputGzipBytes:      totalGzipBytes,
		TotalOutputBytes:     totalBytes,
		TotalOutputGzipBytes: totalGzipBytes,
		OutputFileCount:      len(outputFiles),
		OutputFiles:          outputFiles,
		Minify:               minify,
		Sourcemaps:           sourcemaps,
		CodeSplitting:        codeSplitting,
		InputCount:           len(inputPaths),
		InputPaths:           slices.Clone(inputPaths),
	})
	if err := os.WriteFile(reportPath, reportBytes, 0o644); err != nil {
		return errors.Wrap(err, "write goscript bundle report")
	}
	return nil
}

func readGoScriptBundleDirectoryOutputFiles(outDir string) ([]goScriptBundleOutputFile, error) {
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
	slices.Sort(outputPaths)
	if len(outputPaths) == 0 {
		return nil, errors.Errorf("no goscript bundle outputs under %s", outDir)
	}

	outputFiles := make([]goScriptBundleOutputFile, 0, len(outputPaths))
	for _, outputPath := range outputPaths {
		outBytes, err := os.ReadFile(outputPath)
		if err != nil {
			return nil, errors.Wrap(err, "read goscript bundle for report")
		}
		gzipBytes, err := gzipBytesLen(outBytes)
		if err != nil {
			return nil, err
		}
		outputFiles = append(outputFiles, goScriptBundleOutputFile{
			Path:      outputPath,
			Bytes:     int64(len(outBytes)),
			GzipBytes: gzipBytes,
		})
	}
	return outputFiles, nil
}

func readGoScriptBundleOutputFiles(outPath string, codeSplitting bool) ([]goScriptBundleOutputFile, error) {
	var outputPaths []string
	if codeSplitting {
		if err := filepath.WalkDir(filepath.Dir(outPath), func(path string, entry os.DirEntry, err error) error {
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
	} else {
		outputPaths = []string{outPath}
	}
	slices.Sort(outputPaths)
	if len(outputPaths) == 0 {
		return nil, errors.Errorf("no goscript bundle outputs under %s", filepath.Dir(outPath))
	}

	outputFiles := make([]goScriptBundleOutputFile, 0, len(outputPaths))
	for _, outputPath := range outputPaths {
		outBytes, err := os.ReadFile(outputPath)
		if err != nil {
			return nil, errors.Wrap(err, "read goscript bundle for report")
		}
		gzipBytes, err := gzipBytesLen(outBytes)
		if err != nil {
			return nil, err
		}
		outputFiles = append(outputFiles, goScriptBundleOutputFile{
			Path:      outputPath,
			Bytes:     int64(len(outBytes)),
			GzipBytes: gzipBytes,
		})
	}
	return outputFiles, nil
}

func marshalGoScriptBundleReport(report goScriptBundleReport) []byte {
	var arena fastjson.Arena
	root := arena.NewObject()
	root.Set("schemaVersion", arena.NewNumberInt(report.SchemaVersion))
	root.Set("outputPath", arena.NewString(report.OutputPath))
	root.Set("outputBytes", arena.NewNumberString(strconv.FormatInt(report.OutputBytes, 10)))
	root.Set("outputGzipBytes", arena.NewNumberString(strconv.FormatInt(report.OutputGzipBytes, 10)))
	root.Set("totalOutputBytes", arena.NewNumberString(strconv.FormatInt(report.TotalOutputBytes, 10)))
	root.Set("totalOutputGzipBytes", arena.NewNumberString(strconv.FormatInt(report.TotalOutputGzipBytes, 10)))
	root.Set("outputFileCount", arena.NewNumberInt(report.OutputFileCount))
	outputFiles := arena.NewArray()
	for idx, outputFile := range report.OutputFiles {
		outputFileValue := arena.NewObject()
		outputFileValue.Set("path", arena.NewString(outputFile.Path))
		outputFileValue.Set("bytes", arena.NewNumberString(strconv.FormatInt(outputFile.Bytes, 10)))
		outputFileValue.Set("gzipBytes", arena.NewNumberString(strconv.FormatInt(outputFile.GzipBytes, 10)))
		outputFiles.SetArrayItem(idx, outputFileValue)
	}
	root.Set("outputFiles", outputFiles)
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
	root.Set("inputCount", arena.NewNumberInt(report.InputCount))
	inputPaths := arena.NewArray()
	for idx, inputPath := range report.InputPaths {
		inputPaths.SetArrayItem(idx, arena.NewString(inputPath))
	}
	root.Set("inputPaths", inputPaths)
	reportBytes := root.MarshalTo(nil)
	return append(reportBytes, '\n')
}

func gzipBytesLen(contents []byte) (int64, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return 0, errors.Wrap(err, "create goscript bundle gzip reporter")
	}
	if _, err := writer.Write(contents); err != nil {
		return 0, errors.Wrap(err, "gzip goscript bundle report contents")
	}
	if err := writer.Close(); err != nil {
		return 0, errors.Wrap(err, "close goscript bundle gzip reporter")
	}
	return int64(buf.Len()), nil
}
