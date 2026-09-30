//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

func TestGenerateModuleWritesReadonlyBuildableHiddenModule(t *testing.T) {
	// Create the fixture module directory inside a temp dir.
	ctx := context.Background()
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "openmind")
	spacewaveRoot := testSpacewaveRoot(t)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write the plugin factory and module fixture files.
	writeFile(t, sourceDir, "plugin/root/root.go", `package root

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)

func NewFactory(b bus.Bus) controller.Factory {
	return nil
}
`)
	writeModuleCompilerFixtureGoMod(t, sourceDir, spacewaveRoot)

	// Analyze the fixture module's plugin package.
	le := logrus.NewEntry(logrus.New())
	analysis, err := AnalyzePackages(
		ctx,
		le,
		sourceDir,
		[]string{"./plugin/root"},
		[]string{"build_type_dev", "purego"},
		"js",
		"wasm",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Generate the module wrapper into the codegen directory.
	codegenDir := filepath.Join(sourceDir, ".bldr", "build", "web", "js", "wasm", "openmind-core")
	if err := os.MkdirAll(codegenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	compiler, err := NewModuleCompiler(le, codegenDir, "OpenMind/Core")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.GenerateModule(
		analysis,
		bldr_plugin.NewPluginMeta("openmind", "openmind-core", "web/js/wasm", "dev"),
		nil,
		nil,
		"",
	); err != nil {
		t.Fatal(err)
	}

	// The generated go.mod must carry the module and replace directives.
	goModData, err := os.ReadFile(filepath.Join(codegenDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goModText := string(goModData)
	for _, want := range []string{
		"module github.com/s4wave/spacewave/bldr/plugin/generated/openmind-core",
		"replace example.com/openmind => " + localReplacePath(codegenDir, sourceDir),
		"replace github.com/s4wave/spacewave => " + localReplacePath(codegenDir, spacewaveRoot),
	} {
		if !strings.Contains(goModText, want) {
			t.Fatalf("generated go.mod missing %q:\n%s", want, goModText)
		}
	}

	// The generated module must build with readonly modules.
	cmd := exec.CommandContext(ctx, "go", "list", "-mod=readonly", ".")
	cmd.Dir = codegenDir
	cmd.Env = append(os.Environ(),
		"GO111MODULE=on",
		"GOPROXY=direct",
		"GOWORK=off",
		"GOOS=js",
		"GOARCH=wasm",
		"CGO_ENABLED=0",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go list -mod=readonly failed: %v\n%s", err, out)
	}
}

func writeModuleCompilerFixtureGoMod(t *testing.T, sourceDir, spacewaveRoot string) {
	// Mark the helper and write the fixture go.mod.
	t.Helper()

	// Read the repo go.mod and extract its required versions.
	rootGoModPath := filepath.Join(spacewaveRoot, "go.mod")
	rootGoModData, err := os.ReadFile(rootGoModPath)
	if err != nil {
		t.Fatalf("read repo go.mod: %v", err)
	}
	rootGoMod, err := modfile.Parse(rootGoModPath, rootGoModData, nil)
	if err != nil {
		t.Fatalf("parse repo go.mod: %v", err)
	}
	if rootGoMod.Go == nil {
		t.Fatal("repo go.mod missing go directive")
	}
	controllerbusVersion := rootRequireVersion(t, rootGoMod, "github.com/aperturerobotics/controllerbus")

	// Build the fixture go.mod from the repo directives.
	fixtureGoMod := new(modfile.File)
	if err := fixtureGoMod.AddModuleStmt("example.com/openmind"); err != nil {
		t.Fatalf("add fixture module directive: %v", err)
	}
	if err := fixtureGoMod.AddGoStmt(rootGoMod.Go.Version); err != nil {
		t.Fatalf("add fixture go directive: %v", err)
	}
	fixtureGoMod.AddNewRequire("github.com/aperturerobotics/controllerbus", controllerbusVersion, false)
	fixtureGoMod.AddNewRequire("github.com/s4wave/spacewave", "v0.0.0", false)
	if err := fixtureGoMod.AddReplace("github.com/s4wave/spacewave", "", filepath.ToSlash(spacewaveRoot), ""); err != nil {
		t.Fatalf("add spacewave fixture replace: %v", err)
	}
	fixtureGoModData, err := fixtureGoMod.Format()
	if err != nil {
		t.Fatalf("format fixture go.mod: %v", err)
	}

	// Write the fixture go.mod and a temporary tools package.
	writeFile(t, sourceDir, "go.mod", string(fixtureGoModData))
	writeFile(t, sourceDir, "tools/tools.go", `//go:build tools

package tools

import _ "github.com/s4wave/spacewave/bldr/web/bundler"
`)

	// Tidy the fixture module and drop the tools package.
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = sourceDir
	cmd.Env = append(os.Environ(),
		"GO111MODULE=on",
		"GOPROXY=direct",
		"GOWORK=off",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy fixture module: %v\n%s", err, out)
	}
	if err := os.RemoveAll(filepath.Join(sourceDir, "tools")); err != nil {
		t.Fatalf("remove fixture tidy tools package: %v", err)
	}
}

func rootRequireVersion(t *testing.T, modFile *modfile.File, modulePath string) string {
	t.Helper()

	// Return the required version for the module path.
	for _, req := range modFile.Require {
		if req.Mod.Path == modulePath {
			return req.Mod.Version
		}
	}
	t.Fatalf("repo go.mod missing required fixture module %s", modulePath)
	return ""
}

func testSpacewaveRoot(t *testing.T) string {
	// Mark the helper and resolve the repository root.
	t.Helper()

	// Resolve the repository root from this file's path.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "../../../.."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
