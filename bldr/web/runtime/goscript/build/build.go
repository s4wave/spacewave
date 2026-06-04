//go:build !js

package web_runtime_goscript_build

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"os"
	oexec "os/exec"
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

	goScriptBundleReportFilename = "plugin-goscript-bundle-report.json"
	rolldownCLIRelPath           = "node_modules/rolldown/dist/cli.mjs"
)

type rolldownGoScriptBundleOptions struct {
	EntrypointPath      string `json:"entrypointPath"`
	BldrDistRoot        string `json:"bldrDistRoot"`
	SourceRoot          string `json:"sourceRoot"`
	GoScriptOutputRoot  string `json:"goScriptOutputRoot"`
	OutPath             string `json:"outPath"`
	InputsPath          string `json:"inputsPath"`
	UndefinedImportPath string `json:"undefinedImportPath"`
	Banner              string `json:"banner"`
	Minify              bool   `json:"minify"`
	Sourcemaps          bool   `json:"sourcemaps"`
}

type goScriptBundleReport struct {
	SchemaVersion   int      `json:"schemaVersion"`
	OutputPath      string   `json:"outputPath"`
	OutputBytes     int64    `json:"outputBytes"`
	OutputGzipBytes int64    `json:"outputGzipBytes"`
	Minify          bool     `json:"minify"`
	Sourcemaps      bool     `json:"sourcemaps"`
	InputCount      int      `json:"inputCount"`
	InputPaths      []string `json:"inputPaths"`
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

	return runRolldownGoScriptBundle(ctx, le, bldrDistRoot, workDir, goScriptOutputRoot, entrypointPath, outPath, minify, sourcemaps)
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
	sourcemaps bool,
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
		EntrypointPath:      entrypointPath,
		BldrDistRoot:        bldrDistRoot,
		SourceRoot:          resolveGoScriptSourceRoot(bldrDistRoot),
		GoScriptOutputRoot:  goScriptOutputRoot,
		OutPath:             outPath,
		InputsPath:          inputsPath,
		UndefinedImportPath: undefinedImportPath,
		Banner:              banner,
		Minify:              minify,
		Sourcemaps:          sourcemaps,
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
	if sourcemaps {
		if err := inlineAndExternalSourceMap(outPath); err != nil {
			return nil, err
		}
	}
	inputPaths, err := readRolldownInputPaths(workDir, inputsPath)
	if err != nil {
		return nil, err
	}
	if err := writeGoScriptBundleReport(GoScriptBundleReportPath(workDir), outPath, inputPaths, minify, sourcemaps); err != nil {
		return nil, err
	}
	return inputPaths, nil
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
	rolldownCLIPath, err := ensureRolldownCLIPath(ctx, le, stateDir, bldrDistRoot)
	if err != nil {
		return nil, err
	}
	bunPath, err := npm.ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}
	return bldr_exec.NewCmd(ctx, bunPath, rolldownCLIPath, "--config", configPath), nil
}

func ensureRolldownCLIPath(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot string) (string, error) {
	depsRoot := filepath.Join(bldrDistRoot, "dist", "deps")
	if cliPath := installedRolldownCLIPath(depsRoot); cliPath != "" {
		return cliPath, nil
	}

	srcPackageJSON := filepath.Join(depsRoot, "package.json")
	installDir := filepath.Join(stateDir, "goscript-rolldown")
	if err := npm.EnsureBunInstall(ctx, le, stateDir, srcPackageJSON, installDir); err != nil {
		return "", errors.Wrap(err, "install bldr rolldown tool dependencies")
	}
	if cliPath := installedRolldownCLIPath(installDir); cliPath != "" {
		return cliPath, nil
	}
	return "", errors.Errorf("rolldown CLI missing after installing %s", srcPackageJSON)
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

func writeGoScriptBundleReport(reportPath, outPath string, inputPaths []string, minify, sourcemaps bool) error {
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		return errors.Wrap(err, "read goscript bundle for report")
	}
	gzipBytes, err := gzipBytesLen(outBytes)
	if err != nil {
		return err
	}
	reportBytes := marshalGoScriptBundleReport(goScriptBundleReport{
		SchemaVersion:   1,
		OutputPath:      outPath,
		OutputBytes:     int64(len(outBytes)),
		OutputGzipBytes: gzipBytes,
		Minify:          minify,
		Sourcemaps:      sourcemaps,
		InputCount:      len(inputPaths),
		InputPaths:      slices.Clone(inputPaths),
	})
	if err := os.WriteFile(reportPath, reportBytes, 0o644); err != nil {
		return errors.Wrap(err, "write goscript bundle report")
	}
	return nil
}

func marshalGoScriptBundleReport(report goScriptBundleReport) []byte {
	var arena fastjson.Arena
	root := arena.NewObject()
	root.Set("schemaVersion", arena.NewNumberInt(report.SchemaVersion))
	root.Set("outputPath", arena.NewString(report.OutputPath))
	root.Set("outputBytes", arena.NewNumberString(strconv.FormatInt(report.OutputBytes, 10)))
	root.Set("outputGzipBytes", arena.NewNumberString(strconv.FormatInt(report.OutputGzipBytes, 10)))
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
	writeConfigString(&builder, "inputsPath", options.InputsPath)
	writeConfigString(&builder, "undefinedImportPath", options.UndefinedImportPath)
	writeConfigString(&builder, "banner", options.Banner)
	writeConfigBool(&builder, "minify", options.Minify)
	writeConfigBool(&builder, "sourcemaps", options.Sourcemaps)
	builder.WriteString("}\n")
	builder.WriteString(rolldownGoScriptConfig)
	return []byte(builder.String())
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

function resolveDistSourceImport(source) {
  if (!source.endsWith(".js")) return null
  if (!bldrDistSourcePrefixes.some((prefix) => source.startsWith(prefix))) return null
  return existingSourcePath(path.join(opts.bldrDistRoot, source))
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
    if (source.startsWith("@goscript/")) {
      const resolved = resolveGoScriptImport(source)
      if (resolved) {
        trackInput(resolved)
        return resolved
      }
      return null
    }
    if (importer && !importer.startsWith("\0") && source.endsWith(".js") && (source.startsWith("./") || source.startsWith("../"))) {
      const resolved = existingTypeScriptSibling(path.join(path.dirname(importer), source))
      if (resolved) {
        trackInput(resolved)
        return resolved
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
  input: opts.entrypointPath,
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
    file: opts.outPath,
    format: "esm",
    sourcemap: opts.sourcemaps ? true : false,
    minify: opts.minify,
    codeSplitting: false,
    banner: opts.banner,
  },
}
`
