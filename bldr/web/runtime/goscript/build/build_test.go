//go:build !js

package web_runtime_goscript_build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestBuildWebGoScriptPluginScriptFailsUndefinedImports(t *testing.T) {
	root := t.TempDir()
	bldrDistRoot := filepath.Join(root, "dist")
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

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
