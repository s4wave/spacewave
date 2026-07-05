//go:build !js

package web_runtime_goscript_build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/bldr/util/npm"
	"github.com/sirupsen/logrus"
)

func TestBuildWebGoScriptPluginScriptFailsUndefinedImports(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
export default async function runGoScriptPlugin(_api, pluginMain) {
  await pluginMain()
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
import * as missing from "../missing/index.js"

export async function main() {
  return missing.Missing
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "missing", "index.ts"), `
export const Present = 1
`)

	_, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		false,
		false,
	)
	if err == nil {
		t.Fatal("expected undefined import error")
	}
	if !strings.Contains(err.Error(), "undefined GoScript import") {
		t.Fatalf("error = %q, want undefined GoScript import", err)
	}
}

func TestBuildWebGoScriptPluginScriptBuildsResolvedImports(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
export default async function runGoScriptPlugin(_api, pluginMain) {
  await pluginMain()
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
import { Present } from "../missing/index.js"

export async function main() {
  return Present
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "missing", "index.ts"), `
export const Present = 1
`)

	inputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("expected build inputs")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
}

func TestBuildWebGoScriptPluginScriptResolvesGoScriptOverrideImports(t *testing.T) {
	sourceRoot := t.TempDir()
	bldrDistRoot := filepath.Join(sourceRoot, "bldr")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(sourceRoot, "work")
	goScriptOutputRoot := filepath.Join(sourceRoot, "goscript")
	outPath := filepath.Join(sourceRoot, "out", "plugin.mjs")
	stdlibMathPath := filepath.Join(sourceRoot, "vendor", "github.com", "s4wave", "goscript", "gs", "math", "index.ts")

	writeTestFile(t, filepath.Join(sourceRoot, "go.mod"), "module github.com/s4wave/spacewave\n")
	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
export default async function runGoScriptPlugin(_api, pluginMain) {
  await pluginMain()
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
import { EncodedValue } from "@goscript/github.com/aperturerobotics/protobuf-go-lite/index.js"

export async function main() {
  return EncodedValue
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "aperturerobotics", "protobuf-go-lite", "index.ts"), `
import { MathValue } from "../../../math/index.js"

export const EncodedValue = MathValue
`)
	writeTestFile(t, stdlibMathPath, `
export const MathValue = 1
`)

	inputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	stdlibMathPath = canonicalTestPath(t, stdlibMathPath)
	if !slices.Contains(inputs, stdlibMathPath) {
		t.Fatalf("inputs missing %s: %v", stdlibMathPath, inputs)
	}
}

func TestBuildWebGoScriptPluginScriptShimsNodeEvents(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
export default async function runGoScriptPlugin(_api, pluginMain) {
  await pluginMain()
}
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
import { setMaxListeners } from "node:events"

export async function main() {
  setMaxListeners(Infinity)
}
`)

	_, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `from "node:events"`) || strings.Contains(string(out), `from 'node:events'`) {
		t.Fatalf("node:events should be shimmed out of browser bundle:\n%s", out)
	}
}

func TestBuildWebGoScriptPluginScriptResolvesBldrRuntimeAliases(t *testing.T) {
	sourceRoot := t.TempDir()
	bldrDistRoot := filepath.Join(sourceRoot, "bldr")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(sourceRoot, "work")
	goScriptOutputRoot := filepath.Join(sourceRoot, "goscript")
	outPath := filepath.Join(sourceRoot, "out", "plugin.mjs")
	sdkPath := filepath.Join(bldrDistRoot, "sdk", "plugin.ts")
	localProtoPath := filepath.Join(sourceRoot, "bldr", "plugin", "plugin.pb.ts")
	vendorProtoPath := filepath.Join(bldrDistRoot, "vendor", "github.com", "aperturerobotics", "controllerbus", "controller", "exec", "exec.pb.ts")

	writeTestFile(t, filepath.Join(sourceRoot, "go.mod"), "module github.com/s4wave/spacewave\n")
	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
import { BackendAPI } from "@aptre/bldr-sdk"

export default async function runGoScriptPlugin(_api, pluginMain) {
  void BackendAPI
  await pluginMain()
}
`)
	writeTestFile(t, sdkPath, `
import { ExecControllerRequest } from "@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js"
import { PluginStartInfo } from "@go/github.com/s4wave/spacewave/bldr/plugin/plugin.pb.js"

export const BackendAPI = {
  ExecControllerRequest,
  PluginStartInfo,
}
`)
	writeTestFile(t, localProtoPath, `
export const PluginStartInfo = 1
`)
	writeTestFile(t, vendorProtoPath, `
export const ExecControllerRequest = 2
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
export async function main() {
  return 1
}
`)

	inputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		sdkPath,
		localProtoPath,
		vendorProtoPath,
	} {
		input = canonicalTestPath(t, input)
		if !slices.Contains(inputs, input) {
			t.Fatalf("inputs missing %s: %v", input, inputs)
		}
	}
}

func TestBuildWebGoScriptPluginScriptResolvesExternalBldrRuntimeAliases(t *testing.T) {
	sourceRoot := t.TempDir()
	bldrDistRoot := filepath.Join(sourceRoot, ".bldr", "src")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(sourceRoot, "work")
	goScriptOutputRoot := filepath.Join(sourceRoot, "goscript")
	outPath := filepath.Join(sourceRoot, "out", "plugin.mjs")
	sdkPath := filepath.Join(bldrDistRoot, "sdk", "plugin.ts")
	spacewaveProtoPath := filepath.Join(bldrDistRoot, "vendor", "github.com", "s4wave", "spacewave", "bldr", "plugin", "plugin.pb.ts")
	vendorProtoPath := filepath.Join(bldrDistRoot, "vendor", "github.com", "aperturerobotics", "controllerbus", "controller", "exec", "exec.pb.ts")

	writeTestFile(t, filepath.Join(sourceRoot, "go.mod"), "module github.com/example/app\n")
	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
import { BackendAPI } from "@aptre/bldr-sdk"

export default async function runGoScriptPlugin(_api, pluginMain) {
  void BackendAPI
  await pluginMain()
}
`)
	writeTestFile(t, sdkPath, `
import { ExecControllerRequest } from "@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js"
import { PluginStartInfo } from "@go/github.com/s4wave/spacewave/bldr/plugin/plugin.pb.js"

export const BackendAPI = {
  ExecControllerRequest,
  PluginStartInfo,
}
`)
	writeTestFile(t, spacewaveProtoPath, `
export const PluginStartInfo = 1
`)
	writeTestFile(t, vendorProtoPath, `
export const ExecControllerRequest = 2
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
export async function main() {
  return 1
}
`)

	inputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		sdkPath,
		spacewaveProtoPath,
		vendorProtoPath,
	} {
		input = canonicalTestPath(t, input)
		if !slices.Contains(inputs, input) {
			t.Fatalf("inputs missing %s: %v", input, inputs)
		}
	}
}

func TestRunRolldownGoScriptBundleSplitsAndLoadsDynamicGoScriptChunk(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	writeTestFile(t, runtimePath, `
export default async function runGoScriptPlugin(_api, pluginMain) {
  return await pluginMain()
}
`)
	runtimeImport, err := relativeImportPath(workDir, runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, entrypointPath, `import runGoScriptPlugin from `+strconv.Quote(runtimeImport)+`
import { main as pluginMain } from "@goscript/example/main/plugin.gs.js"

export default async function main(api) {
  return await runGoScriptPlugin(api, pluginMain)
}
`)
	writeTestFile(t, mainPath, `
export async function main() {
  const lazy = await import("../lazy/index.js")
  return lazy.LazyValue
}
`)
	writeTestFile(t, lazyPath, `
export const LazyValue = "loaded from dynamic goscript chunk"
`)

	inputs, err := runRolldownGoScriptBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		entrypointPath,
		outPath,
		false,
		false,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
	chunksDir := filepath.Join(filepath.Dir(outPath), "chunks")
	chunkEntries, err := os.ReadDir(chunksDir)
	if err != nil {
		t.Fatal(err)
	}
	var chunkFiles []string
	for _, chunkEntry := range chunkEntries {
		if chunkEntry.Type().IsRegular() && strings.HasSuffix(chunkEntry.Name(), ".mjs") {
			chunkFiles = append(chunkFiles, filepath.Join(chunksDir, chunkEntry.Name()))
		}
	}
	if len(chunkFiles) == 0 {
		t.Fatalf("expected at least one dynamic chunk under %s", chunksDir)
	}
	lazyPath = canonicalTestPath(t, lazyPath)
	if !slices.Contains(inputs, lazyPath) {
		t.Fatalf("inputs missing lazy GoScript module %s: %v", lazyPath, inputs)
	}

	runBundledEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from dynamic goscript chunk")
}

func TestRunRolldownGoScriptBundleSplitsWithSourceMapsAndLoadsDynamicGoScriptChunk(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	writeTestFile(t, runtimePath, `
export default async function runGoScriptPlugin(_api, pluginMain) {
  return await pluginMain()
}
`)
	runtimeImport, err := relativeImportPath(workDir, runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, entrypointPath, `import runGoScriptPlugin from `+strconv.Quote(runtimeImport)+`
import { main as pluginMain } from "@goscript/example/main/plugin.gs.js"

export default async function main(api) {
  return await runGoScriptPlugin(api, pluginMain)
}
`)
	writeTestFile(t, mainPath, `
export async function main() {
  const lazy = await import("../lazy/index.js")
  return lazy.LazyValue
}
`)
	writeTestFile(t, lazyPath, `
export const LazyValue = "loaded from dynamic goscript sourcemap chunk"
`)

	inputs, err := runRolldownGoScriptBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		entrypointPath,
		outPath,
		false,
		true,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
	entryMapSources := assertExternalSourceMapForOutput(t, outPath)
	if !sourceMapSourcesContainSuffix(entryMapSources, "plugin-goscript-entrypoint.ts") {
		t.Fatalf("entry sourcemap sources = %v, want plugin-goscript-entrypoint.ts", entryMapSources)
	}

	chunksDir := filepath.Join(filepath.Dir(outPath), "chunks")
	chunkEntries, err := os.ReadDir(chunksDir)
	if err != nil {
		t.Fatal(err)
	}
	var chunkFiles []string
	var lazyChunkFile string
	var lazyChunkMapSources []string
	for _, chunkEntry := range chunkEntries {
		if !chunkEntry.Type().IsRegular() || !strings.HasSuffix(chunkEntry.Name(), ".mjs") {
			continue
		}
		chunkFile := filepath.Join(chunksDir, chunkEntry.Name())
		chunkFiles = append(chunkFiles, chunkFile)
		chunkMapSources := assertExternalSourceMapForOutput(t, chunkFile)
		lazyChunkMapSources = chunkMapSources
		if sourceMapSourcesContainSuffix(chunkMapSources, "lazy/index.ts") {
			lazyChunkFile = chunkFile
		}
	}
	if len(chunkFiles) == 0 {
		t.Fatalf("expected at least one dynamic chunk under %s", chunksDir)
	}
	if lazyChunkFile == "" {
		t.Fatalf("chunk sourcemap sources did not include lazy GoScript module; chunks=%v lastSources=%v", chunkFiles, lazyChunkMapSources)
	}
	lazyPath = canonicalTestPath(t, lazyPath)
	if !slices.Contains(inputs, lazyPath) {
		t.Fatalf("inputs missing lazy GoScript module %s: %v", lazyPath, inputs)
	}

	runBundledEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from dynamic goscript sourcemap chunk")
}

func TestBuildWebGoScriptPluginScriptCodeSplittingUsesLazyMainLoader(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	writeTestFile(t, runtimePath, `
export default async function runGoScriptPlugin(api, loadPluginMain) {
  if (typeof loadPluginMain !== "function") {
    throw new Error("plugin main loader was not a function")
  }
  if (globalThis.__pluginMainModuleEvaluated) {
    throw new Error("plugin main module loaded before wrapper invoked the lazy loader")
  }
  globalThis.__pluginLoaderWasFunction = true
  const pluginMain = await loadPluginMain()
  globalThis.__pluginMainLoadedAfterLoader = globalThis.__pluginMainModuleEvaluated === true
  await pluginMain(api)
}
`)
	writeTestFile(t, mainPath, `
globalThis.__pluginMainModuleEvaluated = true

export async function main(api) {
  const lazy = await import("../lazy/index.js")
  globalThis.__pluginMainResult = api.prefix + ":" + lazy.LazyValue
}
`)
	writeTestFile(t, lazyPath, `
export const LazyValue = "loaded from public plugin split chunk"
`)

	inputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/main",
		false,
		true,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInputsContainPaths(t, inputs, mainPath, lazyPath)
	assertBundleReport(t, GoScriptBundleReportPath(workDir), outPath, false, true, true, inputs)
	entryMapSources := assertExternalSourceMapForOutput(t, outPath)
	if !sourceMapSourcesContainSuffix(entryMapSources, "plugin-goscript-entrypoint.ts") {
		t.Fatalf("entry sourcemap sources = %v, want plugin-goscript-entrypoint.ts", entryMapSources)
	}
	chunkFiles := assertSplitOutputLoadsPayloadFromChunk(t, outPath, "loaded from public plugin split chunk")
	var lazyChunkMapSources []string
	for _, chunkFile := range chunkFiles {
		chunkMapSources := assertExternalSourceMapForOutput(t, chunkFile)
		if sourceMapSourcesContainSuffix(chunkMapSources, "lazy/index.ts") {
			lazyChunkMapSources = chunkMapSources
		}
	}
	if len(lazyChunkMapSources) == 0 {
		t.Fatalf("no chunk sourcemap referenced lazy GoScript module; chunks=%v", chunkFiles)
	}

	runBundledPluginEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from public plugin split chunk")
}

func TestBuildWebGoScriptRuntimeScriptCodeSplittingDefersMainChunkUntilMessage(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "runtime.mjs")
	runtimePath := filepath.Join(bldrDistRoot, "web", "entrypoint", "browser", "runtime-goscript.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "runtime", "main.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	writeTestFile(t, runtimePath, `
export default function runGoScriptRuntime(loadDistMain) {
  if (typeof loadDistMain !== "function") {
    throw new Error("runtime main loader was not a function")
  }
  if (globalThis.__runtimeMainModuleEvaluated) {
    throw new Error("runtime main module loaded before listener setup")
  }
  globalThis.__runtimeLoaderInstalled = true
  self.onmessage = async (event) => {
    if (globalThis.__runtimeMainModuleEvaluated) {
      throw new Error("runtime main module loaded before message-triggered lazy import")
    }
    const distMain = await loadDistMain()
    await distMain(event.data)
  }
}
`)
	writeTestFile(t, mainPath, `
globalThis.__runtimeMainModuleEvaluated = true

export async function main(init) {
  const lazy = await import("../lazy/index.js")
  globalThis.__runtimeMainResult = init.prefix + ":" + lazy.LazyValue
}
`)
	writeTestFile(t, lazyPath, `
export const LazyValue = "loaded from public runtime split chunk"
`)

	inputs, err := BuildWebGoScriptRuntimeScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"example/runtime",
		false,
		false,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInputsContainPaths(t, inputs, mainPath, lazyPath)
	assertSplitOutputLoadsPayloadFromChunk(t, outPath, "loaded from public runtime split chunk")

	runBundledRuntimeEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from public runtime split chunk")
}

func TestBuildWebGoScriptPluginScriptAppliesRolldownPolicies(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	goScriptOutputRoot := filepath.Join(root, "goscript")
	minWorkDir := filepath.Join(root, "work-min")
	readableWorkDir := filepath.Join(root, "work-readable")
	readableMapWorkDir := filepath.Join(root, "work-readable-map")
	minOutPath := filepath.Join(root, "out", "plugin.min.mjs")
	readableOutPath := filepath.Join(root, "out", "plugin.readable.mjs")
	readableMapOutPath := filepath.Join(root, "out", "plugin.readable-map.mjs")
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	valuesPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "values", "index.ts")

	writeTestFile(t, runtimePath, `
export default async function runGoScriptPlugin(_api, pluginMain) {
  await pluginMain()
}
`)
	writeTestFile(t, mainPath, `
import { Present } from "../values/index.js"

export async function main() {
  const verboseLocalNameOne = Present + 1
  const verboseLocalNameTwo = verboseLocalNameOne + 2
  const verboseLocalNameThree = verboseLocalNameTwo + 3
  return verboseLocalNameThree
}
`)
	writeTestFile(t, valuesPath, `
export const Present = 1
export const Unused = 2
`)

	inputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		minWorkDir,
		goScriptOutputRoot,
		minOutPath,
		"example/main",
		true,
		true,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		filepath.Join(minWorkDir, "plugin-goscript-entrypoint.ts"),
		runtimePath,
		mainPath,
		valuesPath,
	} {
		input = canonicalTestPath(t, input)
		if !slices.Contains(inputs, input) {
			t.Fatalf("inputs missing %s: %v", input, inputs)
		}
	}
	assertInlineAndExternalSourceMap(t, minOutPath)
	minOut, err := os.ReadFile(minOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(minOut), "export") {
		t.Fatalf("minified output should remain browser ESM:\n%s", minOut)
	}
	assertBundleReport(t, GoScriptBundleReportPath(minWorkDir), minOutPath, true, true, false, inputs)
	if _, err := os.Stat(minOutPath + ".goscript-bundle-report.json"); !os.IsNotExist(err) {
		t.Fatalf("dist report path exists or stat failed: %v", err)
	}

	readableInputs, err := BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		readableWorkDir,
		goScriptOutputRoot,
		readableOutPath,
		"example/main",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	minInfo, err := os.Stat(minOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if minInfo.Size() == 0 {
		t.Fatal("expected minified output")
	}
	readableOut, err := os.ReadFile(readableOutPath)
	if err != nil {
		t.Fatal(err)
	}
	minCode := strings.Split(string(minOut), "sourceMappingURL=")[0]
	if len(minCode) >= len(readableOut) {
		t.Fatalf("minified code size = %d, readable code size = %d", len(minCode), len(readableOut))
	}
	assertBundleReport(t, GoScriptBundleReportPath(readableWorkDir), readableOutPath, false, false, false, readableInputs)

	_, err = BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		readableMapWorkDir,
		goScriptOutputRoot,
		readableMapOutPath,
		"example/main",
		false,
		true,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInlineAndExternalSourceMap(t, readableMapOutPath)
}

func assertBundleReport(t *testing.T, reportPath, outPath string, minify, sourcemaps, codeSplitting bool, inputs []string) {
	t.Helper()
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	report, err := parser.ParseBytes(reportBytes)
	if err != nil {
		t.Fatal(err)
	}
	outInfo, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.GetInt("schemaVersion"); got != 1 {
		t.Fatalf("schemaVersion = %d, want 1", got)
	}
	if got := string(report.GetStringBytes("outputPath")); got != outPath {
		t.Fatalf("outputPath = %q, want %q", got, outPath)
	}
	if got := report.GetInt64("outputBytes"); got != outInfo.Size() {
		t.Fatalf("outputBytes = %d, want %d", got, outInfo.Size())
	}
	if got := report.GetInt64("outputGzipBytes"); got <= 0 {
		t.Fatalf("outputGzipBytes = %d, want positive", got)
	}
	if got := report.GetBool("minify"); got != minify {
		t.Fatalf("minify = %v, want %v", got, minify)
	}
	if got := report.GetBool("sourcemaps"); got != sourcemaps {
		t.Fatalf("sourcemaps = %v, want %v", got, sourcemaps)
	}
	if got := report.GetBool("codeSplitting"); got != codeSplitting {
		t.Fatalf("codeSplitting = %v, want %v", got, codeSplitting)
	}
	if got := report.GetInt("inputCount"); got != len(inputs) {
		t.Fatalf("inputCount = %d, want %d", got, len(inputs))
	}
	inputValues := report.GetArray("inputPaths")
	gotInputs := make([]string, 0, len(inputValues))
	for _, inputValue := range inputValues {
		gotInputs = append(gotInputs, string(inputValue.GetStringBytes()))
	}
	if !slices.Equal(gotInputs, inputs) {
		t.Fatalf("inputPaths = %v, want %v", gotInputs, inputs)
	}
}

func assertInlineAndExternalSourceMap(t *testing.T, outPath string) {
	t.Helper()
	if _, err := os.Stat(outPath + ".map"); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	outString := string(out)
	if !strings.Contains(outString, "sourceMappingURL=data:application/json;base64,") {
		t.Fatalf("output missing inline sourcemap reference:\n%s", out)
	}
	if strings.Contains(outString, "sourceMappingURL="+filepath.Base(outPath)+".map") {
		t.Fatalf("output should use inline sourcemap reference, got external reference:\n%s", out)
	}
}

func assertExternalSourceMapForOutput(t *testing.T, outPath string) []string {
	t.Helper()
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	outString := strings.TrimRight(string(out), "\r\n")
	if outString == "" {
		t.Fatalf("output %s is empty", outPath)
	}
	lines := strings.Split(outString, "\n")
	const sourceMappingURLPrefix = "//# sourceMappingURL="
	sourceMappingURLLine := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(sourceMappingURLLine, sourceMappingURLPrefix) {
		t.Fatalf("output %s missing trailing sourcemap reference:\n%s", outPath, out)
	}
	sourceMappingURL := strings.TrimPrefix(sourceMappingURLLine, sourceMappingURLPrefix)
	if strings.HasPrefix(sourceMappingURL, "data:") {
		t.Fatalf("split output %s should keep an external sourcemap reference, got inline reference", outPath)
	}
	wantSourceMappingURL := filepath.Base(outPath) + ".map"
	if sourceMappingURL != wantSourceMappingURL {
		t.Fatalf("sourceMappingURL for %s = %q, want %q", outPath, sourceMappingURL, wantSourceMappingURL)
	}
	mapBytes, err := os.ReadFile(filepath.Join(filepath.Dir(outPath), sourceMappingURL))
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	sourceMap, err := parser.ParseBytes(mapBytes)
	if err != nil {
		t.Fatalf("parse sourcemap for %s: %v", outPath, err)
	}
	if got := sourceMap.GetInt("version"); got != 3 {
		t.Fatalf("sourcemap version for %s = %d, want 3", outPath, got)
	}
	mappings := string(sourceMap.GetStringBytes("mappings"))
	if mappings == "" {
		t.Fatalf("sourcemap for %s has empty mappings", outPath)
	}
	sourceValues := sourceMap.GetArray("sources")
	if len(sourceValues) == 0 {
		t.Fatalf("sourcemap for %s has no sources", outPath)
	}
	sources := make([]string, 0, len(sourceValues))
	for _, sourceValue := range sourceValues {
		sources = append(sources, string(sourceValue.GetStringBytes()))
	}
	return sources
}

func sourceMapSourcesContainSuffix(sources []string, suffix string) bool {
	suffix = filepath.ToSlash(suffix)
	for _, source := range sources {
		if strings.HasSuffix(filepath.ToSlash(source), suffix) {
			return true
		}
	}
	return false
}

func assertInputsContainPaths(t *testing.T, inputs []string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		path = canonicalTestPath(t, path)
		if !slices.Contains(inputs, path) {
			t.Fatalf("inputs missing %s: %v", path, inputs)
		}
	}
}

func collectSplitChunkFiles(t *testing.T, outPath string) []string {
	t.Helper()
	chunksDir := filepath.Join(filepath.Dir(outPath), "chunks")
	chunkEntries, err := os.ReadDir(chunksDir)
	if err != nil {
		t.Fatal(err)
	}
	var chunkFiles []string
	for _, chunkEntry := range chunkEntries {
		if chunkEntry.Type().IsRegular() && strings.HasSuffix(chunkEntry.Name(), ".mjs") {
			chunkFiles = append(chunkFiles, filepath.Join(chunksDir, chunkEntry.Name()))
		}
	}
	if len(chunkFiles) == 0 {
		t.Fatalf("expected at least one dynamic chunk under %s", chunksDir)
	}
	return chunkFiles
}

func assertSplitOutputLoadsPayloadFromChunk(t *testing.T, outPath, payload string) []string {
	t.Helper()
	entryOut, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(entryOut), payload) {
		t.Fatalf("split entry %s contains lazy payload %q instead of leaving it in a chunk", outPath, payload)
	}
	chunkFiles := collectSplitChunkFiles(t, outPath)
	for _, chunkFile := range chunkFiles {
		chunkOut, err := os.ReadFile(chunkFile)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(chunkOut), payload) {
			return chunkFiles
		}
	}
	t.Fatalf("no split chunk contained lazy payload %q; chunks=%v", payload, chunkFiles)
	return nil
}

func runBundledPluginEntryModule(t *testing.T, stateDir, entryPath, want string) {
	t.Helper()
	runBunModuleScript(t, stateDir, `
import { pathToFileURL } from "node:url"

globalThis.__pluginMainModuleEvaluated = false
const mod = await import(pathToFileURL(process.argv[2]).href)
if (globalThis.__pluginMainModuleEvaluated) {
  throw new Error("plugin main chunk loaded during entry import")
}
await mod.default({ prefix: "plugin-api" })
if (globalThis.__pluginLoaderWasFunction !== true) {
  throw new Error("plugin wrapper did not receive a loader function")
}
if (globalThis.__pluginMainLoadedAfterLoader !== true) {
  throw new Error("plugin main was not loaded through the lazy loader")
}
const got = globalThis.__pluginMainResult
const want = "plugin-api:" + process.argv[3]
if (got !== want) {
  throw new Error("plugin main result " + JSON.stringify(got) + ", want " + JSON.stringify(want))
}
process.exit(0)
`, entryPath, want)
}

func runBundledRuntimeEntryModule(t *testing.T, stateDir, entryPath, want string) {
	t.Helper()
	runBunModuleScript(t, stateDir, `
import { pathToFileURL } from "node:url"

globalThis.self = globalThis
globalThis.__runtimeMainModuleEvaluated = false
await import(pathToFileURL(process.argv[2]).href)
if (globalThis.__runtimeLoaderInstalled !== true) {
  throw new Error("runtime wrapper did not install the lazy loader")
}
if (typeof globalThis.self.onmessage !== "function") {
  throw new Error("runtime wrapper did not install a message listener")
}
if (globalThis.__runtimeMainModuleEvaluated) {
  throw new Error("runtime main chunk loaded before listener setup")
}
await globalThis.self.onmessage({ data: { prefix: "runtime-api" } })
const got = globalThis.__runtimeMainResult
const want = "runtime-api:" + process.argv[3]
if (got !== want) {
  throw new Error("runtime main result " + JSON.stringify(got) + ", want " + JSON.stringify(want))
}
process.exit(0)
`, entryPath, want)
}

func runBunModuleScript(t *testing.T, stateDir, script string, args ...string) {
	t.Helper()
	runnerPath := filepath.Join(t.TempDir(), "run-module.mjs")
	writeTestFile(t, runnerPath, script)
	bunPath, err := npm.ResolveBunPath(context.Background(), logrus.NewEntry(logrus.New()), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), bunPath, append([]string{runnerPath}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("module execution failed: %v\n%s", err, output)
	}
}

func writeRolldownToolFixture(t *testing.T, bldrDistRoot string) {
	t.Helper()
	cliPath := findTestRolldownCLIPath(t)
	targetPath := filepath.Join(bldrDistRoot, "dist", "deps", filepath.FromSlash(rolldownCLIRelPath))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cliPath, targetPath); err != nil {
		t.Fatal(err)
	}
}

func findTestRolldownCLIPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		cliPath := filepath.Join(dir, filepath.FromSlash(rolldownCLIRelPath))
		if info, err := os.Stat(cliPath); err == nil && !info.IsDir() {
			return cliPath
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("rolldown test CLI not found")
		}
		dir = parent
	}
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(realPath)
}

func runBundledEntryModule(t *testing.T, stateDir, entryPath, want string) {
	t.Helper()
	runnerPath := filepath.Join(t.TempDir(), "run-entry.mjs")
	writeTestFile(t, runnerPath, `
import { pathToFileURL } from "node:url"

const mod = await import(pathToFileURL(process.argv[2]).href)
const got = await mod.default({})
if (got !== process.argv[3]) {
  throw new Error("entry returned " + JSON.stringify(got) + ", want " + JSON.stringify(process.argv[3]))
}
`)
	bunPath, err := npm.ResolveBunPath(context.Background(), logrus.NewEntry(logrus.New()), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), bunPath, runnerPath, entryPath, want)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("entry module execution failed: %v\n%s", err, output)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
