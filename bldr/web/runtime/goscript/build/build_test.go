//go:build !js

package web_runtime_goscript_build

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

	_, err = BuildWebGoScriptPluginScript(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		bldrDistRoot,
		readableWorkDir,
		goScriptOutputRoot,
		readableOutPath,
		"example/main",
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
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInlineAndExternalSourceMap(t, readableMapOutPath)
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

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
