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
	// Prepare the Rolldown fixture and plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Write a plugin that references a missing GoScript export.
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

	// Build the plugin with the undefined import.
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

	// Require the build error to identify the undefined GoScript import.
	if err == nil {
		t.Fatal("expected undefined import error")
	}
	if !strings.Contains(err.Error(), "undefined GoScript import") {
		t.Fatalf("error = %q, want undefined GoScript import", err)
	}
}

func TestBuildWebGoScriptPluginScriptBuildsResolvedImports(t *testing.T) {
	// Prepare the Rolldown fixture and plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Write a plugin and its resolved GoScript dependency.
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

	// Build the plugin with the resolved import.
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

	// Require recorded inputs and the plugin output file.
	if len(inputs) == 0 {
		t.Fatal("expected build inputs")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
}

func TestBuildWebGoScriptPluginScriptExternalizesSharedGoScriptImports(t *testing.T) {
	// Prepare the Rolldown fixture and plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Locate the main, shared dependency, and application modules.
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "s4wave", "spacewave", "main", "plugin.gs.ts")
	sharedPath := filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "aperturerobotics", "protobuf-go-lite", "index.ts")
	appPath := filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "s4wave", "spacewave", "local", "index.ts")

	// Write a plugin that combines shared and application values.
	writeTestFile(t, filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"), `
export default async function runGoScriptPlugin(_api, pluginMain) {
  globalThis.__pluginResult = await pluginMain()
}
`)
	writeTestFile(t, mainPath, `
import { SharedValue } from "@goscript/github.com/aperturerobotics/protobuf-go-lite/index.js"
import { AppValue } from "@goscript/github.com/s4wave/spacewave/local/index.js"

export async function main() {
  return SharedValue + ":" + AppValue
}
`)
	writeTestFile(t, sharedPath, `
export const SharedValue = "shared-source-payload"
`)
	writeTestFile(t, appPath, `
export const AppValue = "local-app-payload"
`)

	// Build the plugin with shared dependency externalization.
	inputs, err := BuildWebGoScriptPluginScriptWithOptions(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outPath,
		"github.com/s4wave/spacewave/main",
		GoScriptMinifyNone,
		false,
		false,
		GoScriptSharedBundleOptions{
			WebPkgID: GoScriptSharedWebPkgID,
			Enabled:  true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require local inputs and exclude the shared source from the bundle report.
	assertInputsContainPaths(t, inputs,
		filepath.Join(workDir, "plugin-goscript-entrypoint.ts"),
		filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts"),
		mainPath,
		appPath,
	)
	sharedPath = canonicalTestPath(t, sharedPath)
	if slices.Contains(inputs, sharedPath) {
		t.Fatalf("inputs included externalized shared source %s: %v", sharedPath, inputs)
	}
	assertBundleReport(t, GoScriptBundleReportPath(workDir), outPath, false, false, false, inputs)

	// Read the plugin bundle for shared import assertions.
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	// Require the provider URL and exclude the bare shared specifier.
	outString := string(out)
	sharedURL := "/b/pkg/@s4wave/goscript-shared/github.com/aperturerobotics/protobuf-go-lite/index.mjs"
	if !strings.Contains(outString, sharedURL) {
		t.Fatalf("bundle missing absolute shared provider URL %q:\n%s", sharedURL, outString)
	}
	bareSharedSpecifier := "@goscript/github.com/aperturerobotics/protobuf-go-lite/index.js"
	if strings.Contains(outString, bareSharedSpecifier) {
		t.Fatalf("bundle kept bare shared import specifier %q:\n%s", bareSharedSpecifier, outString)
	}

	// Require local application bytes and exclude shared dependency bytes.
	if strings.Contains(outString, "shared-source-payload") {
		t.Fatalf("bundle included externalized shared source payload:\n%s", outString)
	}
	if !strings.Contains(outString, "local-app-payload") {
		t.Fatalf("bundle did not keep github.com/s4wave app module local:\n%s", outString)
	}
}

func TestBuildWebGoScriptSharedProviderScriptPublishesSharedModules(t *testing.T) {
	// Prepare the Rolldown fixture and provider build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outWebPkgPath := filepath.Join(root, "out", "webpkg")

	// Locate the shared modules and application module.
	protobufPath := filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "aperturerobotics", "protobuf-go-lite", "index.ts")
	clockPath := filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "example", "clock", "index.ts")
	appPath := filepath.Join(goScriptOutputRoot, "@goscript", "github.com", "s4wave", "spacewave", "local", "index.ts")

	// Write shared dependency modules and an application-only module.
	writeTestFile(t, protobufPath, `
export const EncodedValue = "encoded-for-consumer-a"
export const DecodedValue = "decoded-for-consumer-a"
`)
	writeTestFile(t, clockPath, `
export const ClockValue = "clock-for-consumer-b"
`)
	writeTestFile(t, appPath, `
export const LocalOnlyValue = "must-not-be-published"
`)

	// Build the shared GoScript provider.
	importMap, inputs, err := BuildWebGoScriptSharedProviderScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		outWebPkgPath,
		GoScriptSharedWebPkgID,
		GoScriptMinifyNone,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require shared provider inputs and exclude application sources.
	assertInputsContainPaths(t, inputs, protobufPath, clockPath)
	appPath = canonicalTestPath(t, appPath)
	if slices.Contains(inputs, appPath) {
		t.Fatalf("provider inputs included github.com/s4wave app source %s: %v", appPath, inputs)
	}

	// Require provider URLs for the shared modules only.
	wantImportMap := GoScriptSharedImportMap{
		"@goscript/github.com/aperturerobotics/protobuf-go-lite/index.js": "/b/pkg/@s4wave/goscript-shared/github.com/aperturerobotics/protobuf-go-lite/index.mjs",
		"@goscript/github.com/example/clock/index.js":                     "/b/pkg/@s4wave/goscript-shared/github.com/example/clock/index.mjs",
	}
	for source, wantURL := range wantImportMap {
		if gotURL := importMap[source]; gotURL != wantURL {
			t.Fatalf("importMap[%q] = %q, want %q (full map %v)", source, gotURL, wantURL, importMap)
		}
	}
	if _, ok := importMap["@goscript/github.com/s4wave/spacewave/local/index.js"]; ok {
		t.Fatalf("provider import map included github.com/s4wave app module: %v", importMap)
	}

	// Require the provider outputs and their directory report.
	protobufOut := filepath.Join(outWebPkgPath, "github.com", "aperturerobotics", "protobuf-go-lite", "index.mjs")
	clockOut := filepath.Join(outWebPkgPath, "github.com", "example", "clock", "index.mjs")
	if _, err := os.Stat(protobufOut); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(clockOut); err != nil {
		t.Fatal(err)
	}
	assertDirectoryBundleReport(t, GoScriptBundleReportPath(workDir), outWebPkgPath, false, false, true, inputs, protobufOut, clockOut)

	// Execute the provider modules and verify their exported values.
	runBunModuleScript(t, filepath.Join(workDir, "..", "..", "bun"), `
import { pathToFileURL } from "node:url"

const protobuf = await import(pathToFileURL(process.argv[2]).href)
const clock = await import(pathToFileURL(process.argv[3]).href)
if (protobuf.EncodedValue !== "encoded-for-consumer-a") {
  throw new Error("EncodedValue = " + JSON.stringify(protobuf.EncodedValue))
}
if (protobuf.DecodedValue !== "decoded-for-consumer-a") {
  throw new Error("DecodedValue = " + JSON.stringify(protobuf.DecodedValue))
}
if (clock.ClockValue !== "clock-for-consumer-b") {
  throw new Error("ClockValue = " + JSON.stringify(clock.ClockValue))
}
`, protobufOut, clockOut)
}

func TestBuildWebGoScriptPluginScriptResolvesGoScriptOverrideImports(t *testing.T) {
	// Prepare the Rolldown fixture and standard library override path.
	sourceRoot := t.TempDir()
	bldrDistRoot := filepath.Join(sourceRoot, "bldr")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(sourceRoot, "work")
	goScriptOutputRoot := filepath.Join(sourceRoot, "goscript")
	outPath := filepath.Join(sourceRoot, "out", "plugin.mjs")
	stdlibMathPath := filepath.Join(sourceRoot, "vendor", "github.com", "s4wave", "goscript", "gs", "math", "index.ts")

	// Write a plugin that imports the GoScript math override through a dependency.
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

	// Build the plugin with the standard library override.
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

	// Require the overridden math module among the bundle inputs.
	stdlibMathPath = canonicalTestPath(t, stdlibMathPath)
	if !slices.Contains(inputs, stdlibMathPath) {
		t.Fatalf("inputs missing %s: %v", stdlibMathPath, inputs)
	}
}

func TestBuildWebGoScriptPluginScriptShimsNodeEvents(t *testing.T) {
	// Prepare the Rolldown fixture and plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Write a plugin that calls the Node events API.
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

	// Build the plugin with the browser events shim.
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

	// Require the browser output to exclude Node events imports.
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `from "node:events"`) || strings.Contains(string(out), `from 'node:events'`) {
		t.Fatalf("node:events should be shimmed out of browser bundle:\n%s", out)
	}
}

func TestBuildWebGoScriptPluginScriptResolvesBldrRuntimeAliases(t *testing.T) {
	// Prepare the Rolldown fixture and plugin build paths.
	sourceRoot := t.TempDir()
	bldrDistRoot := filepath.Join(sourceRoot, "bldr")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(sourceRoot, "work")
	goScriptOutputRoot := filepath.Join(sourceRoot, "goscript")
	outPath := filepath.Join(sourceRoot, "out", "plugin.mjs")

	// Locate SDK and generated protobuf fixtures for runtime aliases.
	sdkPath := filepath.Join(bldrDistRoot, "sdk", "plugin.ts")
	localProtoPath := filepath.Join(sourceRoot, "bldr", "plugin", "plugin.pb.ts")
	vendorProtoPath := filepath.Join(bldrDistRoot, "vendor", "github.com", "aperturerobotics", "controllerbus", "controller", "exec", "exec.pb.ts")

	// Write a plugin runtime whose SDK imports local and vendored protobuf modules.
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

	// Build the plugin with Bldr runtime aliases.
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

	// Require every aliased SDK and protobuf module among the inputs.
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
	// Prepare the extracted Bldr fixture and plugin build paths.
	sourceRoot := t.TempDir()
	bldrDistRoot := filepath.Join(sourceRoot, ".bldr", "src")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(sourceRoot, "work")
	goScriptOutputRoot := filepath.Join(sourceRoot, "goscript")
	outPath := filepath.Join(sourceRoot, "out", "plugin.mjs")

	// Locate SDK, Bldr protobuf, and application protobuf modules.
	sdkPath := filepath.Join(bldrDistRoot, "sdk", "plugin.ts")
	spacewaveProtoPath := filepath.Join(bldrDistRoot, "vendor", "github.com", "s4wave", "spacewave", "bldr", "plugin", "plugin.pb.ts")
	vendorProtoPath := filepath.Join(bldrDistRoot, "vendor", "github.com", "aperturerobotics", "controllerbus", "controller", "exec", "exec.pb.ts")
	appProtoPath := filepath.Join(sourceRoot, "vendor", "github.com", "example", "geometry", "types.pb.ts")

	// Write an external application with an extracted Bldr runtime and aliased imports.
	writeTestFile(t, filepath.Join(sourceRoot, "go.mod"), "module github.com/example/app\n")
	writeTestFile(t, filepath.Join(bldrDistRoot, "go.mod"), "module github.com/s4wave/spacewave/bldr-dist\n")
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
	writeTestFile(t, appProtoPath, `
export const Vector = 3
`)
	writeTestFile(t, filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts"), `
import { Vector } from "@go/github.com/example/geometry/types.pb.js"

export async function main() {
  return Vector
}
`)

	// Build the external application plugin with Bldr runtime aliases.
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

	// Require both Bldr and application alias targets among the inputs.
	for _, input := range []string{
		sdkPath,
		spacewaveProtoPath,
		vendorProtoPath,
		appProtoPath,
	} {
		input = canonicalTestPath(t, input)
		if !slices.Contains(inputs, input) {
			t.Fatalf("inputs missing %s: %v", input, inputs)
		}
	}
}

func TestRunRolldownGoScriptBundleSplitsAndLoadsDynamicGoScriptChunk(t *testing.T) {
	// Prepare the Rolldown fixture and split plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Locate the plugin runtime, entrypoint, main module, and lazy module.
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	// Write a plugin entrypoint whose main module dynamically imports a dependency.
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

	// Build the plugin with dynamic chunk splitting.
	inputs, err := runRolldownGoScriptBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		entrypointPath,
		outPath,
		GoScriptMinifyNone,
		false,
		true,
		GoScriptSharedBundleOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require the plugin entry output and enumerate its dynamic chunks.
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

	// Require a dynamic chunk and the lazy module among the build inputs.
	if len(chunkFiles) == 0 {
		t.Fatalf("expected at least one dynamic chunk under %s", chunksDir)
	}
	lazyPath = canonicalTestPath(t, lazyPath)
	if !slices.Contains(inputs, lazyPath) {
		t.Fatalf("inputs missing lazy GoScript module %s: %v", lazyPath, inputs)
	}

	// Execute the plugin entrypoint and verify the dynamic chunk value.
	runBundledEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from dynamic goscript chunk")
}

func TestRunRolldownGoScriptBundleSplitsWithSourceMapsAndLoadsDynamicGoScriptChunk(t *testing.T) {
	// Prepare the Rolldown fixture and mapped split plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Locate the plugin runtime, entrypoint, main module, and lazy module.
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	entrypointPath := filepath.Join(workDir, "plugin-goscript-entrypoint.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	// Write a plugin entrypoint whose main module dynamically imports a dependency.
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

	// Build the split plugin with sourcemaps.
	inputs, err := runRolldownGoScriptBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		workDir,
		goScriptOutputRoot,
		entrypointPath,
		outPath,
		GoScriptMinifyNone,
		true,
		true,
		GoScriptSharedBundleOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require the entry output and its source in the external sourcemap.
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
	entryMapSources := assertExternalSourceMapForOutput(t, outPath)
	if !sourceMapSourcesContainSuffix(entryMapSources, "plugin-goscript-entrypoint.ts") {
		t.Fatalf("entry sourcemap sources = %v, want plugin-goscript-entrypoint.ts", entryMapSources)
	}

	// Enumerate dynamic chunks and find the lazy module in their sourcemaps.
	chunksDir := filepath.Join(filepath.Dir(outPath), "chunks")
	chunkEntries, err := os.ReadDir(chunksDir)
	if err != nil {
		t.Fatal(err)
	}
	var chunkFiles []string
	var lazyChunkFile string
	var lazyChunkMapSources []string

	// Inspect each emitted chunk sourcemap for the lazy module.
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

	// Require a lazy chunk and the lazy module among the build inputs.
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

	// Execute the mapped plugin entrypoint and verify the dynamic chunk value.
	runBundledEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from dynamic goscript sourcemap chunk")
}

func TestBuildWebGoScriptPluginScriptCodeSplittingUsesLazyMainLoader(t *testing.T) {
	// Prepare the Rolldown fixture and split plugin build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "plugin.mjs")

	// Locate the plugin runtime, main module, and lazy module.
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	// Write a runtime that checks lazy main loading and a plugin that imports a lazy value.
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

	// Build the plugin with split chunks and sourcemaps.
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

	// Require the generated plugin entrypoint to forward environment and startup completion.
	entrypointData, err := os.ReadFile(filepath.Join(workDir, "plugin-goscript-entrypoint.ts"))
	if err != nil {
		t.Fatal(err)
	}
	entrypointSource := string(entrypointData)
	if !strings.Contains(entrypointSource, "export default function main(api, _abortSignal, env)") ||
		!strings.Contains(entrypointSource, "return runGoScriptPlugin") ||
		!strings.Contains(entrypointSource, ".main, env)") {
		t.Fatalf("entrypoint does not forward the env and return the GoScript startup lifecycle: %s", entrypointSource)
	}

	// Require the main and lazy inputs and the mapped bundle report.
	assertInputsContainPaths(t, inputs, mainPath, lazyPath)
	assertBundleReport(t, GoScriptBundleReportPath(workDir), outPath, false, true, true, inputs)
	entryMapSources := assertExternalSourceMapForOutput(t, outPath)
	if !sourceMapSourcesContainSuffix(entryMapSources, "plugin-goscript-entrypoint.ts") {
		t.Fatalf("entry sourcemap sources = %v, want plugin-goscript-entrypoint.ts", entryMapSources)
	}

	// Require the lazy payload and its source in a split chunk.
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

	// Execute the plugin wrapper and verify lazy main loading.
	runBundledPluginEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from public plugin split chunk")
}

func TestBuildWebGoScriptRuntimeScriptCodeSplittingDefersMainChunkUntilMessage(t *testing.T) {
	// Prepare the Rolldown fixture and split browser runtime build paths.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	workDir := filepath.Join(root, "work")
	goScriptOutputRoot := filepath.Join(root, "goscript")
	outPath := filepath.Join(root, "out", "runtime.mjs")

	// Locate the browser runtime, main module, and lazy module.
	runtimePath := filepath.Join(bldrDistRoot, "web", "entrypoint", "browser", "runtime-goscript.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "runtime", "main.gs.ts")
	lazyPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "lazy", "index.ts")

	// Write a browser runtime that loads the main module only after a message.
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

	// Build the browser runtime with split chunks.
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

	// Require the main and lazy inputs and a split lazy payload.
	assertInputsContainPaths(t, inputs, mainPath, lazyPath)
	assertSplitOutputLoadsPayloadFromChunk(t, outPath, "loaded from public runtime split chunk")

	// Execute the browser entrypoint and verify message-triggered main loading.
	runBundledRuntimeEntryModule(t, filepath.Join(workDir, "..", "..", "bun"), outPath, "loaded from public runtime split chunk")
}

func TestBuildWebGoScriptPluginScriptAppliesRolldownPolicies(t *testing.T) {
	// Prepare the Rolldown fixture and GoScript output root.
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
	writeRolldownToolFixture(t, bldrDistRoot)
	goScriptOutputRoot := filepath.Join(root, "goscript")

	// Locate work directories and outputs for the minification and sourcemap policies.
	minWorkDir := filepath.Join(root, "work-min")
	readableWorkDir := filepath.Join(root, "work-readable")
	readableMapWorkDir := filepath.Join(root, "work-readable-map")
	minOutPath := filepath.Join(root, "out", "plugin.min.mjs")
	readableOutPath := filepath.Join(root, "out", "plugin.readable.mjs")
	readableMapOutPath := filepath.Join(root, "out", "plugin.readable-map.mjs")

	// Locate the plugin runtime, main module, and dependency fixtures.
	runtimePath := filepath.Join(bldrDistRoot, webRuntimeGoScriptDir, "plugin-goscript.ts")
	mainPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "main", "plugin.gs.ts")
	valuesPath := filepath.Join(goScriptOutputRoot, "@goscript", "example", "values", "index.ts")

	// Write a plugin with unused exports and local names for minification checks.
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

	// Build the plugin with full minification and sourcemaps.
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

	// Require the generated entrypoint and source modules among the inputs.
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

	// Require browser ESM output and both sourcemap forms.
	assertInlineAndExternalSourceMap(t, minOutPath)
	minOut, err := os.ReadFile(minOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(minOut), "export") {
		t.Fatalf("minified output should remain browser ESM:\n%s", minOut)
	}

	// Read and validate the private GoScript bundle report.
	assertBundleReport(t, GoScriptBundleReportPath(minWorkDir), minOutPath, true, true, false, inputs)
	reportBytes, err := os.ReadFile(GoScriptBundleReportPath(minWorkDir))
	if err != nil {
		t.Fatal(err)
	}
	var reportParser fastjson.Parser
	report, err := reportParser.ParseBytes(reportBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Report bundle measurements and require the report to stay out of distribution output.
	t.Logf(
		"GoScript Rolldown seed: raw=%d files=%d inputs=%d",
		report.GetInt64("totalOutputBytes"),
		report.GetInt("outputFileCount"),
		report.GetInt("inputCount"),
	)
	if _, err := os.Stat(minOutPath + ".goscript-bundle-report.json"); !os.IsNotExist(err) {
		t.Fatalf("dist report path exists or stat failed: %v", err)
	}

	// Build the plugin with readable output and no sourcemaps.
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

	// Require minified code to be smaller than readable code and verify the readable report.
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
	minCode, _, _ := strings.Cut(string(minOut), "sourceMappingURL=")
	if len(minCode) >= len(readableOut) {
		t.Fatalf("minified code size = %d, readable code size = %d", len(minCode), len(readableOut))
	}
	assertBundleReport(t, GoScriptBundleReportPath(readableWorkDir), readableOutPath, false, false, false, readableInputs)

	// Build readable output with both sourcemap forms.
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

	// Build the plugin with name mangling and no compression.
	mangleWorkDir := filepath.Join(root, "work-mangle")
	mangleOutPath := filepath.Join(root, "out", "plugin.mangle.mjs")
	_, err = BuildWebGoScriptPluginScriptWithOptions(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		mangleWorkDir,
		goScriptOutputRoot,
		mangleOutPath,
		"example/main",
		GoScriptMinifyMangle,
		false,
		false,
		GoScriptSharedBundleOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require mangled output to remove the verbose local names.
	mangleOut, err := os.ReadFile(mangleOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mangleOut), "verboseLocalName") {
		t.Fatalf("mangle output should mangle local names:\n%s", mangleOut)
	}

	// Compress folds the constant sum; the mangle level leaves it in place.
	if !strings.Contains(minCode, "return 7") || !strings.Contains(string(mangleOut), "1+1+2+3") {
		t.Fatalf("mangle output should skip compress:\nmangle: %s\nfull: %s", mangleOut, minCode)
	}
}

func assertBundleReport(t *testing.T, reportPath, outPath string, minify, sourcemaps, codeSplitting bool, inputs []string) {
	// Read and parse the GoScript bundle report.
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

	// Compare the report identity and entry size with the output file.
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

	// Require the selected bundle policies and input count in the report.
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

	// Require the report input paths to match the build inputs.
	inputValues := report.GetArray("inputPaths")
	gotInputs := make([]string, 0, len(inputValues))
	for _, inputValue := range inputValues {
		gotInputs = append(gotInputs, string(inputValue.GetStringBytes()))
	}
	if !slices.Equal(gotInputs, inputs) {
		t.Fatalf("inputPaths = %v, want %v", gotInputs, inputs)
	}
}

func assertDirectoryBundleReport(t *testing.T, reportPath, outPath string, minify, sourcemaps, codeSplitting bool, inputs []string, outputPaths ...string) {
	// Read and parse the shared provider directory report.
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

	// Require the directory identity, selected policies, and file counts.
	if got := report.GetInt("schemaVersion"); got != 1 {
		t.Fatalf("schemaVersion = %d, want 1", got)
	}
	if got := string(report.GetStringBytes("outputPath")); got != outPath {
		t.Fatalf("outputPath = %q, want %q", got, outPath)
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
	if got := report.GetInt("outputFileCount"); got != len(outputPaths) {
		t.Fatalf("outputFileCount = %d, want %d", got, len(outputPaths))
	}

	// Require every provider output path in the directory report.
	outputValues := report.GetArray("outputFiles")
	gotOutputs := make([]string, 0, len(outputValues))
	for _, outputValue := range outputValues {
		gotOutputs = append(gotOutputs, string(outputValue.GetStringBytes("path")))
	}
	for _, outputPath := range outputPaths {
		if !slices.Contains(gotOutputs, outputPath) {
			t.Fatalf("outputFiles missing %s: %v", outputPath, gotOutputs)
		}
	}
}

func assertInlineAndExternalSourceMap(t *testing.T, outPath string) {
	// Require the external map file and read the mapped bundle output.
	t.Helper()
	if _, err := os.Stat(outPath + ".map"); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	// Require an inline map reference without an external reference in the bundle.
	outString := string(out)
	if !strings.Contains(outString, "sourceMappingURL=data:application/json;base64,") {
		t.Fatalf("output missing inline sourcemap reference:\n%s", out)
	}
	if strings.Contains(outString, "sourceMappingURL="+filepath.Base(outPath)+".map") {
		t.Fatalf("output should use inline sourcemap reference, got external reference:\n%s", out)
	}
}

func assertExternalSourceMapForOutput(t *testing.T, outPath string) []string {
	// Read the mapped bundle output and require nonempty code.
	t.Helper()
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	outString := strings.TrimRight(string(out), "\r\n")
	if outString == "" {
		t.Fatalf("output %s is empty", outPath)
	}

	// Require a trailing external sourcemap URL for the bundle.
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

	// Read and parse the external sourcemap.
	mapBytes, err := os.ReadFile(filepath.Join(filepath.Dir(outPath), sourceMappingURL))
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	sourceMap, err := parser.ParseBytes(mapBytes)
	if err != nil {
		t.Fatalf("parse sourcemap for %s: %v", outPath, err)
	}

	// Require sourcemap version 3 and nonempty mappings.
	if got := sourceMap.GetInt("version"); got != 3 {
		t.Fatalf("sourcemap version for %s = %d, want 3", outPath, got)
	}
	mappings := string(sourceMap.GetStringBytes("mappings"))
	if mappings == "" {
		t.Fatalf("sourcemap for %s has empty mappings", outPath)
	}

	// Collect the source paths from the external sourcemap.
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
	// Read the chunk directory beside the split entry output.
	t.Helper()
	chunksDir := filepath.Join(filepath.Dir(outPath), "chunks")
	chunkEntries, err := os.ReadDir(chunksDir)
	if err != nil {
		t.Fatal(err)
	}

	// Collect emitted JavaScript chunk files and require at least one.
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
	// Require the split entry to exclude the lazy payload.
	t.Helper()
	entryOut, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(entryOut), payload) {
		t.Fatalf("split entry %s contains lazy payload %q instead of leaving it in a chunk", outPath, payload)
	}

	// Search the emitted chunks for the lazy payload.
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

	// Fail when no emitted chunk contains the lazy payload.
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
	// Write the Bun module runner fixture.
	t.Helper()
	runnerPath := filepath.Join(t.TempDir(), "run-module.mjs")
	writeTestFile(t, runnerPath, script)

	// Resolve Bun for the module runner.
	bunPath, err := npm.ResolveBunPath(context.Background(), logrus.NewEntry(logrus.New()), stateDir)
	if err != nil {
		t.Fatal(err)
	}

	// Execute the module runner and require successful completion.
	cmd := exec.CommandContext(context.Background(), bunPath, append([]string{runnerPath}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("module execution failed: %v\n%s", err, output)
	}
}

func writeRolldownToolFixture(t *testing.T, bldrDistRoot string) {
	// Link the installed Rolldown package into the build fixture.
	t.Helper()
	packageRoot, runnerPath := findTestRolldownPaths(t)
	targetPackageRoot := filepath.Join(bldrDistRoot, "dist", "deps", "node_modules", "rolldown")
	if err := os.MkdirAll(filepath.Dir(targetPackageRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(packageRoot, targetPackageRoot); err != nil {
		t.Fatal(err)
	}

	// Read the installed Rolldown manifest and require a package version.
	installedPackage, err := os.ReadFile(filepath.Join(packageRoot, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	manifest, err := parser.ParseBytes(installedPackage)
	if err != nil {
		t.Fatal(err)
	}
	installedVersion := string(manifest.GetStringBytes("version"))
	if installedVersion == "" {
		t.Fatal("Rolldown test package has no version")
	}

	// Write the matching Rolldown dependency declaration into the fixture.
	writeTestFile(
		t,
		filepath.Join(bldrDistRoot, "dist", "deps", "package.json"),
		`{"dependencies":{"rolldown":`+strconv.Quote(installedVersion)+`}}`,
	)

	// Copy the direct Rolldown runner into the build fixture.
	runnerBytes, err := os.ReadFile(runnerPath)
	if err != nil {
		t.Fatal(err)
	}
	targetRunnerPath := filepath.Join(bldrDistRoot, "web", "bundler", "rolldown", "run-build.mjs")
	writeTestFile(t, targetRunnerPath, string(runnerBytes))
}

func findTestRolldownPaths(t *testing.T) (string, string) {
	// Start the Rolldown fixture search from the test working directory.
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var packageRoot, runnerPath string
	for {
		// Locate the installed Rolldown package under the current ancestor.
		if packageRoot == "" {
			for _, candidate := range []string{
				filepath.Join(dir, "dist", "deps", "node_modules", "rolldown"),
				filepath.Join(dir, "node_modules", "rolldown"),
			} {
				if info, err := os.Stat(filepath.Join(candidate, "dist", "index.mjs")); err == nil && !info.IsDir() {
					packageRoot = candidate
					break
				}
			}
		}

		// Locate the direct Rolldown runner under the current ancestor.
		if runnerPath == "" {
			candidate := filepath.Join(dir, "web", "bundler", "rolldown", "run-build.mjs")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				runnerPath = candidate
			}
		}

		// Return complete fixture paths or continue at the parent directory.
		if packageRoot != "" && runnerPath != "" {
			return packageRoot, runnerPath
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Rolldown package or direct runner test fixture not found")
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
	// Write a Bun runner that checks the bundled entrypoint result.
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

	// Resolve Bun for the bundled entrypoint runner.
	bunPath, err := npm.ResolveBunPath(context.Background(), logrus.NewEntry(logrus.New()), stateDir)
	if err != nil {
		t.Fatal(err)
	}

	// Execute the bundled entrypoint and require the expected result.
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
