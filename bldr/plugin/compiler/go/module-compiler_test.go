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
)

func TestGenerateModuleWritesReadonlyBuildableHiddenModule(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "openmind")
	spacewaveRoot := testSpacewaveRoot(t)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spacewaveRel, err := filepath.Rel(sourceDir, spacewaveRoot)
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, sourceDir, "go.mod", `module example.com/openmind

go 1.26.3

require (
	github.com/aperturerobotics/controllerbus v0.53.5-0.20260620224135-5f6015d2a8b0
	github.com/s4wave/spacewave v0.0.0
)

replace github.com/s4wave/spacewave => `+filepath.ToSlash(spacewaveRel)+`
`)
	writeFile(t, sourceDir, "plugin/root/root.go", `package root

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)

func NewFactory(b bus.Bus) controller.Factory {
	return nil
}
`)

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

	codegenDir := filepath.Join(sourceDir, ".bldr", "build", "web", "js", "wasm", "openmind-core")
	if err := os.MkdirAll(codegenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	compiler, err := NewModuleCompiler(le, codegenDir, "OpenMind/Core")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.GenerateModule(
		ctx,
		analysis,
		bldr_plugin.NewPluginMeta("openmind", "openmind-core", "web/js/wasm", "dev"),
		nil,
		nil,
		"",
	); err != nil {
		t.Fatal(err)
	}

	goModData, err := os.ReadFile(filepath.Join(codegenDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goModText := string(goModData)
	for _, want := range []string{
		"module github.com/s4wave/spacewave/bldr/plugin/generated/openmind-core",
		"replace example.com/openmind => " + filepath.ToSlash(sourceDir),
		"replace github.com/s4wave/spacewave => " + filepath.ToSlash(spacewaveRoot),
	} {
		if !strings.Contains(goModText, want) {
			t.Fatalf("generated go.mod missing %q:\n%s", want, goModText)
		}
	}

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

func testSpacewaveRoot(t *testing.T) string {
	t.Helper()
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
