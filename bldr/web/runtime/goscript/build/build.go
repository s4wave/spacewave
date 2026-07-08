//go:build !js

package web_runtime_goscript_build

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"os"
	oexec "os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_exec "github.com/s4wave/spacewave/bldr/util/exec"
	"github.com/s4wave/spacewave/bldr/util/npm"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

const (
	webRuntimeGoScriptDir = "web/runtime/goscript"

	GoScriptSharedWebPkgID       = "@s4wave/goscript-shared"
	goScriptBundleReportFilename = "plugin-goscript-bundle-report.json"
	rolldownCLIRelPath           = "node_modules/rolldown/dist/cli.mjs"

	goScriptSidecarOutputDir = "sidecars"
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

type rolldownGoScriptBundleOptions struct {
	EntrypointPath        string `json:"entrypointPath"`
	BldrDistRoot          string `json:"bldrDistRoot"`
	SourceRoot            string `json:"sourceRoot"`
	GoScriptOutputRoot    string `json:"goScriptOutputRoot"`
	OutPath               string `json:"outPath"`
	OutDir                string `json:"outDir"`
	EntryFileName         string `json:"entryFileName"`
	InputsPath            string `json:"inputsPath"`
	UndefinedImportPath   string `json:"undefinedImportPath"`
	Banner                string `json:"banner"`
	SharedWebPkgID        string `json:"sharedWebPkgID"`
	SharedImportURLPrefix string `json:"sharedImportURLPrefix"`
	Minify                bool   `json:"minify"`
	Sourcemaps            bool   `json:"sourcemaps"`
	CodeSplitting         bool   `json:"codeSplitting"`
	SharedExternalImports bool   `json:"sharedExternalImports"`
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
		"  await runGoScriptPlugin(api, () => Promise.resolve(pluginMain))\n" +
		"}\n"
	if codeSplitting {
		entrypoint = "import runGoScriptPlugin from " + strconv.Quote(runtimeImport) + "\n\n" +
			"export default async function main(api) {\n" +
			"  await runGoScriptPlugin(api, async () => (await import(" + strconv.Quote(mainImport) + ")).main)\n" +
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

	inputsPath := filepath.Join(workDir, "plugin-goscript-inputs.json")
	undefinedImportPath := filepath.Join(workDir, "plugin-goscript-undefined-import.txt")
	configPath := filepath.Join(workDir, "plugin-goscript-rolldown.config.mjs")
	banner := entrypoint_browser_bundle.DefaultBanner()["js"]
	options := rolldownGoScriptBundleOptions{
		EntrypointPath:        entrypointPath,
		BldrDistRoot:          bldrDistRoot,
		SourceRoot:            resolveGoScriptSourceRoot(bldrDistRoot),
		GoScriptOutputRoot:    goScriptOutputRoot,
		OutPath:               outPath,
		OutDir:                filepath.Dir(outPath),
		EntryFileName:         filepath.Base(outPath),
		InputsPath:            inputsPath,
		UndefinedImportPath:   undefinedImportPath,
		Banner:                banner,
		SharedWebPkgID:        sharedWebPkgID(sharedOptions.WebPkgID),
		SharedImportURLPrefix: sharedImportURLPrefix(sharedWebPkgID(sharedOptions.WebPkgID)),
		Minify:                minify,
		Sourcemaps:            sourcemaps,
		CodeSplitting:         codeSplitting,
		SharedExternalImports: sharedOptions.Enabled,
	}
	if err := os.WriteFile(configPath, renderRolldownGoScriptConfig(options), 0o644); err != nil {
		return nil, errors.Wrap(err, "write goscript rolldown config")
	}
	if err := os.Remove(undefinedImportPath); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "remove stale goscript undefined-import marker")
	}

	stateDir := filepath.Join(workDir, "..", "..", "bun")
	cmd, err := newRolldownCommand(ctx, le, stateDir, bldrDistRoot, configPath)
	if err != nil {
		return nil, err
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"NO_COLOR=1",
		"NODE_DISABLE_COLORS=1",
		"CI=1",
	)
	if err := bldr_exec.StartAndWait(ctx, le, cmd); err != nil {
		if undefinedImportErr := readUndefinedImportError(undefinedImportPath); undefinedImportErr != nil {
			return nil, undefinedImportErr
		}
		return nil, err
	}
	if sourcemaps && !codeSplitting {
		if err := inlineAndExternalSourceMap(outPath); err != nil {
			return nil, err
		}
	}
	inputPaths, err := readRolldownInputPaths(workDir, inputsPath)
	if err != nil {
		return nil, err
	}
	sidecarInputs, err := deployGoScriptSidecars(bldrDistRoot, filepath.Dir(outPath))
	if err != nil {
		return nil, err
	}
	inputPaths = append(inputPaths, sidecarInputs...)
	slices.Sort(inputPaths)
	if err := writeGoScriptBundleReport(GoScriptBundleReportPath(workDir), outPath, inputPaths, minify, sourcemaps, codeSplitting); err != nil {
		return nil, err
	}
	return inputPaths, nil
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

	inputsPath := filepath.Join(workDir, "goscript-shared-inputs.json")
	undefinedImportPath := filepath.Join(workDir, "goscript-shared-undefined-import.txt")
	configPath := filepath.Join(workDir, "goscript-shared-rolldown.config.mjs")
	options := rolldownGoScriptBundleOptions{
		BldrDistRoot:        bldrDistRoot,
		SourceRoot:          resolveGoScriptSourceRoot(bldrDistRoot),
		GoScriptOutputRoot:  goScriptOutputRoot,
		OutDir:              outWebPkgPath,
		InputsPath:          inputsPath,
		UndefinedImportPath: undefinedImportPath,
		Banner:              entrypoint_browser_bundle.DefaultBanner()["js"],
		Minify:              minify,
		Sourcemaps:          sourcemaps,
		CodeSplitting:       true,
	}
	if err := os.WriteFile(configPath, renderRolldownGoScriptSharedProviderConfig(options, entrypoints), 0o644); err != nil {
		return nil, nil, errors.Wrap(err, "write goscript shared provider rolldown config")
	}
	if err := os.Remove(undefinedImportPath); err != nil && !os.IsNotExist(err) {
		return nil, nil, errors.Wrap(err, "remove stale goscript shared provider undefined-import marker")
	}

	stateDir := filepath.Join(workDir, "..", "..", "bun")
	cmd, err := newRolldownCommand(ctx, le, stateDir, bldrDistRoot, configPath)
	if err != nil {
		return nil, nil, err
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"NO_COLOR=1",
		"NODE_DISABLE_COLORS=1",
		"CI=1",
	)
	if err := bldr_exec.StartAndWait(ctx, le, cmd); err != nil {
		if undefinedImportErr := readUndefinedImportError(undefinedImportPath); undefinedImportErr != nil {
			return nil, nil, undefinedImportErr
		}
		return nil, nil, err
	}
	inputPaths, err := readRolldownInputPaths(workDir, inputsPath)
	if err != nil {
		return nil, nil, err
	}
	if err := writeGoScriptBundleDirectoryReport(GoScriptBundleReportPath(workDir), outWebPkgPath, inputPaths, minify, sourcemaps, true); err != nil {
		return nil, nil, err
	}
	return importMap, inputPaths, nil
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

func newRolldownCommand(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	bldrDistRoot,
	configPath string,
) (*oexec.Cmd, error) {
	rolldownCLIPath, installRoot, err := ensureRolldownRuntimeDeps(ctx, le, stateDir, bldrDistRoot)
	if err != nil {
		return nil, err
	}
	if err := npm.EnsureNodeModulesLink(filepath.Dir(configPath), installRoot); err != nil {
		return nil, err
	}
	bunPath, err := npm.ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}
	return bldr_exec.NewCmd(ctx, bunPath, rolldownCLIPath, "--config", configPath), nil
}

func ensureRolldownRuntimeDeps(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot string) (string, string, error) {
	depsRoot := filepath.Join(bldrDistRoot, "dist", "deps")
	if cliPath := installedRolldownCLIPath(depsRoot); cliPath != "" {
		return cliPath, depsRoot, nil
	}

	srcPackageJSON := filepath.Join(depsRoot, "package.json")
	installDir := filepath.Join(stateDir, "goscript-rolldown")
	if err := npm.EnsureBunInstall(ctx, le, stateDir, srcPackageJSON, installDir); err != nil {
		return "", "", errors.Wrap(err, "install bldr rolldown tool dependencies")
	}
	if cliPath := installedRolldownCLIPath(installDir); cliPath != "" {
		return cliPath, installDir, nil
	}
	return "", "", errors.Errorf("rolldown CLI missing after installing %s", srcPackageJSON)
}

func installedRolldownCLIPath(root string) string {
	if root == "" {
		return ""
	}
	cliPath := filepath.Join(root, filepath.FromSlash(rolldownCLIRelPath))
	if info, err := os.Stat(cliPath); err == nil && !info.IsDir() {
		return cliPath
	}
	return ""
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

type goScriptSidecar struct {
	Name      string
	SourceRel string
	OutputRel string
}

var goScriptSidecars = []goScriptSidecar{
	{
		Name:      "blake3",
		SourceRel: filepath.Join("rs", "blake3", "blake3.wasm"),
		OutputRel: filepath.Join(goScriptSidecarOutputDir, "blake3.wasm"),
	},
}

func deployGoScriptSidecars(bldrDistRoot, outDir string) ([]string, error) {
	inputs := make([]string, 0, len(goScriptSidecars))
	for _, sidecar := range goScriptSidecars {
		src, err := resolveGoScriptSidecarSource(bldrDistRoot, sidecar)
		if err != nil {
			return nil, err
		}
		if src == "" {
			continue
		}
		dst := filepath.Join(outDir, sidecar.OutputRel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, errors.Wrapf(err, "create goscript sidecar output dir for %s", sidecar.Name)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, errors.Wrapf(err, "read goscript sidecar %s", sidecar.Name)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, errors.Wrapf(err, "write goscript sidecar %s", sidecar.Name)
		}
		// Match the rolldown input normalization: report the symlink-resolved
		// source path so macOS /var vs /private/var aliases compare equal.
		if realPath, err := filepath.EvalSymlinks(src); err == nil {
			src = realPath
		}
		inputs = append(inputs, filepath.Clean(src))
	}
	return inputs, nil
}

func resolveGoScriptSidecarSource(bldrDistRoot string, sidecar goScriptSidecar) (string, error) {
	sourceRoot, sourceRootHasModule := resolveGoScriptSourceRootWithModule(bldrDistRoot)
	candidates := []string{
		filepath.Join(sourceRoot, sidecar.SourceRel),
		filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, sidecar.OutputRel),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		switch {
		case err == nil && !info.IsDir():
			return candidate, nil
		case err == nil:
			return "", errors.Errorf("goscript sidecar %s path is a directory: %s", sidecar.Name, candidate)
		case os.IsNotExist(err):
			continue
		default:
			return "", errors.Wrapf(err, "stat goscript sidecar %s", sidecar.Name)
		}
	}
	if !sourceRootHasModule {
		return "", nil
	}
	return "", errors.Errorf("goscript sidecar %s missing; rebuild %s", sidecar.Name, sidecar.SourceRel)
}

func resolveGoScriptSourceRootWithModule(bldrDistRoot string) (string, bool) {
	dir := bldrDistRoot
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return bldrDistRoot, false
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

func inlineAndExternalSourceMap(outPath string) error {
	mapBytes, err := os.ReadFile(outPath + ".map")
	if err != nil {
		return errors.Wrap(err, "read goscript rolldown sourcemap")
	}
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		return errors.Wrap(err, "read goscript rolldown output")
	}
	lines := strings.Split(strings.TrimRight(string(outBytes), "\r\n"), "\n")
	for len(lines) != 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "//# sourceMappingURL=") {
		lines = lines[:len(lines)-1]
	}
	inlineURL := "//# sourceMappingURL=data:application/json;base64," + base64.StdEncoding.EncodeToString(mapBytes)
	lines = append(lines, inlineURL)
	return os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func readUndefinedImportError(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	message := strings.TrimSpace(string(contents))
	if message == "" {
		return nil
	}
	return errors.New(message)
}

func readRolldownInputPaths(absWorkingDir, inputsPath string) ([]string, error) {
	inputBytes, err := os.ReadFile(inputsPath)
	if os.IsNotExist(err) {
		return nil, errors.New("goscript rolldown inputs were not written")
	}
	if err != nil {
		return nil, errors.Wrap(err, "read goscript rolldown inputs")
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(inputBytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse goscript rolldown inputs")
	}
	inputs := value.GetArray()
	if inputs == nil {
		return nil, errors.New("goscript rolldown inputs are not an array")
	}

	seen := make(map[string]struct{}, len(inputs))
	inputPaths := make([]string, 0, len(inputs))
	for _, input := range inputs {
		inputPathBytes := input.GetStringBytes()
		if len(inputPathBytes) == 0 {
			continue
		}
		inputPath := string(inputPathBytes)
		if strings.HasPrefix(inputPath, "<") {
			continue
		}
		if !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(absWorkingDir, inputPath)
		}
		inputPath = filepath.Clean(inputPath)
		if _, err := os.Stat(inputPath); err != nil {
			continue
		}
		if realPath, err := filepath.EvalSymlinks(inputPath); err == nil {
			inputPath = filepath.Clean(realPath)
		}
		if _, ok := seen[inputPath]; ok {
			continue
		}
		seen[inputPath] = struct{}{}
		inputPaths = append(inputPaths, inputPath)
	}
	slices.Sort(inputPaths)
	return inputPaths, nil
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

func renderRolldownGoScriptConfig(options rolldownGoScriptBundleOptions) []byte {
	var builder strings.Builder
	builder.WriteString("const opts = {\n")
	writeConfigString(&builder, "entrypointPath", options.EntrypointPath)
	writeConfigString(&builder, "bldrDistRoot", options.BldrDistRoot)
	writeConfigString(&builder, "sourceRoot", options.SourceRoot)
	writeConfigString(&builder, "goScriptOutputRoot", options.GoScriptOutputRoot)
	writeConfigString(&builder, "outPath", options.OutPath)
	writeConfigString(&builder, "outDir", options.OutDir)
	writeConfigString(&builder, "entryFileName", options.EntryFileName)
	writeConfigString(&builder, "inputsPath", options.InputsPath)
	writeConfigString(&builder, "undefinedImportPath", options.UndefinedImportPath)
	writeConfigString(&builder, "banner", options.Banner)
	writeConfigString(&builder, "sharedWebPkgID", options.SharedWebPkgID)
	writeConfigString(&builder, "sharedImportURLPrefix", options.SharedImportURLPrefix)
	writeConfigBool(&builder, "minify", options.Minify)
	writeConfigBool(&builder, "sourcemaps", options.Sourcemaps)
	writeConfigBool(&builder, "codeSplitting", options.CodeSplitting)
	writeConfigBool(&builder, "sharedExternalImports", options.SharedExternalImports)
	builder.WriteString("}\n")
	config := strings.Replace(rolldownGoScriptConfig, rolldownGoScriptOutputConfigPlaceholder, renderRolldownGoScriptOutputConfig(options.CodeSplitting), 1)
	builder.WriteString(config)
	return []byte(builder.String())
}

func renderRolldownGoScriptSharedProviderConfig(options rolldownGoScriptBundleOptions, entrypoints map[string]string) []byte {
	var builder strings.Builder
	builder.WriteString("const opts = {\n")
	writeConfigString(&builder, "bldrDistRoot", options.BldrDistRoot)
	writeConfigString(&builder, "sourceRoot", options.SourceRoot)
	writeConfigString(&builder, "goScriptOutputRoot", options.GoScriptOutputRoot)
	writeConfigString(&builder, "outDir", options.OutDir)
	writeConfigString(&builder, "inputsPath", options.InputsPath)
	writeConfigString(&builder, "undefinedImportPath", options.UndefinedImportPath)
	writeConfigString(&builder, "banner", options.Banner)
	writeConfigBool(&builder, "minify", options.Minify)
	writeConfigBool(&builder, "sourcemaps", options.Sourcemaps)
	builder.WriteString("  entrypoints: {\n")
	keys := make([]string, 0, len(entrypoints))
	for key := range entrypoints {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		builder.WriteString("    ")
		builder.WriteString(strconv.Quote(key))
		builder.WriteString(": ")
		builder.WriteString(strconv.Quote(entrypoints[key]))
		builder.WriteString(",\n")
	}
	builder.WriteString("  },\n")
	builder.WriteString("}\n")
	config := strings.Replace(rolldownGoScriptConfig, rolldownGoScriptOutputConfigPlaceholder, renderRolldownGoScriptSharedProviderOutputConfig(), 1)
	builder.WriteString(config)
	return []byte(builder.String())
}

func renderRolldownGoScriptSharedProviderOutputConfig() string {
	return `    dir: opts.outDir,
    entryFileNames: "[name].mjs",
    chunkFileNames: "chunks/[name]-[hash].mjs",
    codeSplitting: true,
`
}

func renderRolldownGoScriptOutputConfig(codeSplitting bool) string {
	if !codeSplitting {
		return `    file: opts.outPath,
    codeSplitting: false,
`
	}
	return `    dir: opts.outDir,
    entryFileNames: opts.entryFileName,
    chunkFileNames: "chunks/[name]-[hash].mjs",
    codeSplitting: {
      groups: [
        {
          name: "shared",
          test: (id) => !isEntrypointModule(id) && !id.startsWith("\0") && !isGoScriptModule(id, "github.com/s4wave/"),
          priority: 1,
        },
        {
          name: "app",
          test: (id) => isGoScriptModule(id, "github.com/s4wave/"),
        },
      ],
    },
`
}

func writeConfigString(builder *strings.Builder, name, value string) {
	builder.WriteString("  ")
	builder.WriteString(name)
	builder.WriteString(": ")
	builder.WriteString(strconv.Quote(value))
	builder.WriteString(",\n")
}

func writeConfigBool(builder *strings.Builder, name string, value bool) {
	builder.WriteString("  ")
	builder.WriteString(name)
	builder.WriteString(": ")
	builder.WriteString(strconv.FormatBool(value))
	builder.WriteString(",\n")
}

const rolldownGoScriptOutputConfigPlaceholder = "__BLDR_GOSCRIPT_OUTPUT_CONFIG__"

const rolldownGoScriptConfig = `import fs from "node:fs"
import path from "node:path"

const nodeEventsModule = "\0goscript-node-events"
const localModulePrefix = "github.com/s4wave/spacewave/"
const bldrDistSourcePrefixes = ["devtool/", "manifest/", "plugin/", "resource/", "sdk/", "web/"]
const inputs = new Set()
const localModule = readLocalModule(opts.sourceRoot)

function existingFile(filePath) {
  try {
    return fs.statSync(filePath).isFile()
  } catch {
    return false
  }
}

function trackInput(filePath) {
  if (!filePath || !path.isAbsolute(filePath) || filePath.startsWith("\0")) return
  if (!existingFile(filePath)) return
  inputs.add(path.normalize(filePath))
}

function existingTypeScriptSibling(filePath) {
  if (filePath.endsWith(".js")) {
    const tsPath = filePath.slice(0, -".js".length) + ".ts"
    if (existingFile(tsPath)) return tsPath
  }
  if (existingFile(filePath)) return filePath
  return null
}

function realFilePath(filePath) {
  try {
    return fs.realpathSync(filePath)
  } catch {
    return path.normalize(filePath)
  }
}

function isEntrypointModule(id) {
  return realFilePath(id) === realFilePath(opts.entrypointPath)
}

function existingSourcePath(filePath) {
  if (existingFile(filePath)) return filePath
  if (filePath.endsWith(".js")) {
    const tsPath = filePath.slice(0, -".js".length) + ".ts"
    if (existingFile(tsPath)) return tsPath
    const tsxPath = filePath.slice(0, -".js".length) + ".tsx"
    if (existingFile(tsxPath)) return tsxPath
  }
  return null
}

function resolveGoScriptImport(source) {
  const rel = source.slice("@goscript/".length)
  return existingTypeScriptSibling(path.join(opts.goScriptOutputRoot, "@goscript", rel))
}

function goScriptSharedURLFromRel(rel) {
  return opts.sharedImportURLPrefix + rel.slice(0, -".js".length) + ".mjs"
}

function sharedGoScriptRel(rel) {
  return rel && !rel.startsWith("github.com/s4wave/")
}

function resolveSharedGoScriptImport(source, importer) {
  if (!opts.sharedExternalImports) return null
  if (source.startsWith("@goscript/")) {
    const rel = source.slice("@goscript/".length)
    if (!rel.endsWith(".js") || !sharedGoScriptRel(rel)) return null
    return goScriptSharedURLFromRel(rel)
  }
  if (!importer || importer.startsWith("\0")) return null
  if (!source.endsWith(".js")) return null
  if (!source.startsWith("./") && !source.startsWith("../")) return null
  const outputRoot = path.join(opts.goScriptOutputRoot, "@goscript")
  const targetPath = path.normalize(path.join(path.dirname(importer), source))
  const rel = path.relative(outputRoot, targetPath).replaceAll("\\", "/")
  if (rel === "" || rel.startsWith("..") || path.isAbsolute(rel)) return null
  if (!sharedGoScriptRel(rel)) return null
  if (!existingTypeScriptSibling(targetPath)) return null
  return goScriptSharedURLFromRel(rel)
}

function resolveGoScriptOverrideSourceImport(source, importer) {
  if (!importer || importer.startsWith("\0")) return null
  if (!source.endsWith(".js")) return null
  if (!source.startsWith("./") && !source.startsWith("../")) return null
  const outputRoot = path.join(opts.goScriptOutputRoot, "@goscript")
  const targetPath = path.normalize(path.join(path.dirname(importer), source))
  const rel = path.relative(outputRoot, targetPath)
  if (rel === "" || rel.startsWith("..") || path.isAbsolute(rel)) return null
  return existingSourcePath(path.join(opts.sourceRoot, "vendor", "github.com", "s4wave", "goscript", "gs", rel))
}

function readLocalModule(projectRoot) {
  try {
    const contents = fs.readFileSync(path.join(projectRoot, "go.mod"), "utf8")
    const match = contents.match(/^\s*module\s+(\S+)/m)
    return match ? match[1] : ""
  } catch {
    return ""
  }
}

function resolveBldrSourcePath(sourceRel) {
  const monorepoPath = existingSourcePath(path.join(opts.bldrDistRoot, "bldr", sourceRel))
  if (monorepoPath) return monorepoPath
  return existingSourcePath(path.join(opts.bldrDistRoot, sourceRel))
}

function resolveBldrAlias(source) {
  if (source === "@aptre/bldr-sdk") return resolveBldrSourcePath("sdk/plugin.ts")
  const bldrSDKPrefix = "@aptre/bldr-sdk/"
  if (source.startsWith(bldrSDKPrefix)) return resolveBldrSourcePath(path.join("sdk", source.slice(bldrSDKPrefix.length)))
  if (source === "@aptre/bldr") return resolveBldrSourcePath("web/bldr/index.js")
  if (source === "@aptre/bldr-react") return resolveBldrSourcePath("web/bldr-react/index.js")
  return null
}

function resolveGoImport(source) {
  if (!source.startsWith("@go/") || !source.endsWith(".js")) return null
  const importPath = source.slice("@go/".length)
  if (localModule && importPath.startsWith(localModule + "/")) {
    return existingSourcePath(path.join(opts.sourceRoot, importPath.slice(localModule.length + 1)))
  }
  if (!localModule && importPath.startsWith(localModulePrefix)) {
    return existingSourcePath(path.join(opts.sourceRoot, importPath.slice(localModulePrefix.length)))
  }
  return existingSourcePath(path.join(opts.bldrDistRoot, "vendor", importPath))
}

function barePackageParts(source) {
  if (!source || source.startsWith(".") || source.startsWith("/") || source.startsWith("\0")) return null
  const parts = source.split("/")
  if (source.startsWith("@")) {
    if (parts.length < 2) return null
    return {
      packageParts: parts.slice(0, 2),
      subpathParts: parts.slice(2),
    }
  }
  return {
    packageParts: parts.slice(0, 1),
    subpathParts: parts.slice(1),
  }
}

function resolveNodeModuleImport(source) {
  const parts = barePackageParts(source)
  if (!parts) return null
  const packageRoot = path.join(process.cwd(), "node_modules", ...parts.packageParts)
  if (parts.subpathParts.length !== 0) {
    return existingSourcePath(path.join(packageRoot, ...parts.subpathParts))
  }
  try {
    const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, "package.json"), "utf8"))
    const entry = pkg.module || pkg.browser || pkg.main || "index.js"
    if (typeof entry === "string") {
      return existingSourcePath(path.join(packageRoot, entry))
    }
  } catch {
  }
  return existingSourcePath(path.join(packageRoot, "index.js"))
}

function resolveDistSourceImport(source) {
  if (!source.endsWith(".js")) return null
  if (!bldrDistSourcePrefixes.some((prefix) => source.startsWith(prefix))) return null
  return existingSourcePath(path.join(opts.bldrDistRoot, source))
}

function isGoScriptModule(id, importPrefix) {
  const normalized = id.replaceAll("\\", "/")
  const marker = "@goscript/" + importPrefix
  return normalized.startsWith(marker) || normalized.includes("/" + marker)
}

function isUndefinedImport(log) {
  const message = log?.message || ""
  const code = log?.code || ""
  return code === "IMPORT_IS_UNDEFINED" ||
    code === "import-is-undefined" ||
    code === "ImportIsUndefined" ||
    message.includes("will always be undefined because there is no matching export")
}

const plugin = {
  name: "goscript-import-resolver",
  buildStart() {
    trackInput(opts.entrypointPath)
  },
  resolveId(source, importer) {
    if (source === "node:events") return nodeEventsModule
    const bldrAlias = resolveBldrAlias(source)
    if (bldrAlias) {
      trackInput(bldrAlias)
      return bldrAlias
    }
    const goImport = resolveGoImport(source)
    if (goImport) {
      trackInput(goImport)
      return goImport
    }
    const distSourceImport = resolveDistSourceImport(source)
    if (distSourceImport) {
      trackInput(distSourceImport)
      return distSourceImport
    }
    const nodeModuleImport = resolveNodeModuleImport(source)
    if (nodeModuleImport) {
      trackInput(nodeModuleImport)
      return nodeModuleImport
    }
    if (source.startsWith("@goscript/")) {
      const sharedImport = resolveSharedGoScriptImport(source, importer)
      if (sharedImport) return { id: sharedImport, external: true }
      const resolved = resolveGoScriptImport(source)
      if (resolved) {
        trackInput(resolved)
        return resolved
      }
      return null
    }
    if (importer && !importer.startsWith("\0") && source.endsWith(".js") && (source.startsWith("./") || source.startsWith("../"))) {
      const sharedImport = resolveSharedGoScriptImport(source, importer)
      if (sharedImport) return { id: sharedImport, external: true }
      const resolved = existingTypeScriptSibling(path.join(path.dirname(importer), source))
      if (resolved) {
        trackInput(resolved)
        return resolved
      }
      const overrideSource = resolveGoScriptOverrideSourceImport(source, importer)
      if (overrideSource) {
        trackInput(overrideSource)
        return overrideSource
      }
    }
    return null
  },
  load(id) {
    if (id === nodeEventsModule) return "export function setMaxListeners() {}\n"
    trackInput(id)
    return null
  },
  writeBundle() {
    fs.writeFileSync(opts.inputsPath, JSON.stringify(Array.from(inputs).sort(), null, 2) + "\n")
  },
}

export default {
  input: opts.entrypointPath || opts.entrypoints,
  platform: "browser",
  treeshake: true,
  logLevel: "warn",
  checks: {
    importIsUndefined: true,
  },
  transform: {
    define: {
      BLDR_IS_BROWSER: "true",
      BLDR_IS_PLUGIN: "true",
    },
    target: "es2024",
  },
  plugins: [plugin],
  onLog(level, log, defaultHandler) {
    if (level === "warn" && isUndefinedImport(log)) {
      const file = log?.loc?.file || log?.id || ""
      const suffix = file ? " in " + file : ""
      const message = "undefined GoScript import" + suffix + ": " + (log?.message || log?.code || "missing export")
      fs.writeFileSync(opts.undefinedImportPath, message + "\n")
      const err = new Error(message)
      err.stack = err.message
      throw err
    }
    defaultHandler(level, log)
  },
  output: {
__BLDR_GOSCRIPT_OUTPUT_CONFIG__
    format: "esm",
    sourcemap: opts.sourcemaps ? true : false,
    minify: opts.minify,
    banner: opts.banner,
  },
}
`
