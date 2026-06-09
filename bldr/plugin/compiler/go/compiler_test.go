//go:build !js

package bldr_plugin_compiler_go

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

func TestAddTinyGoStartupCacheInputsIncludesProfileIdentity(t *testing.T) {
	values := map[string]string{
		gocompiler.TinyGoProfileEnv:       gocompiler.TinyGoProfileFast,
		gocompiler.TinyGoOptEnv:           "0",
		gocompiler.TinyGoPanicStrategyEnv: "print",
		gocompiler.TinyGoGCEnv:            "leaking",
		gocompiler.TinyGoSchedulerEnv:     "none",
		gocompiler.TinyGoStackSizeEnv:     "512KB",
		gocompiler.TinyGoLLVMFeaturesEnv:  "+atomics,+bulk-memory",
		gocompiler.TinyGoInterpTimeoutEnv: "10m",
	}
	for _, key := range gocompiler.TinyGoStartupCacheEnvKeys() {
		t.Setenv(key, values[key])
	}

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	addTinyGoStartupCacheInputs(inputManifest)

	got := make(map[string]string, len(inputManifest.GetStartupInputs()))
	for _, input := range inputManifest.GetStartupInputs() {
		if input.GetKind() != bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR {
			t.Fatalf("startup input kind = %v, want env var", input.GetKind())
		}
		got[input.GetKey()] = input.GetStringValue()
	}
	for _, key := range gocompiler.TinyGoStartupCacheEnvKeys() {
		if got[key] != values[key] {
			t.Fatalf("startup input %s = %q, want %q", key, got[key], values[key])
		}
	}
}

func TestAddGoScriptStartupCacheInputsIgnoresCommandIdentity(t *testing.T) {
	t.Setenv("BLDR_GOSCRIPT", "/opt/bin/goscript-dev")

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	addGoScriptStartupCacheInputs(inputManifest)

	if got := inputManifest.GetStartupInputs(); len(got) != 0 {
		t.Fatalf("GoScript library compilation should not record command env startup inputs: %v", got)
	}
}

func TestAddGoCompilerStartupCacheInputsIncludesModeIdentity(t *testing.T) {
	t.Setenv(gocompiler.GoCompilerEnv, string(gocompiler.GoCompilerGoScript))

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	addGoCompilerStartupCacheInputs(inputManifest)

	got := make(map[string]string, len(inputManifest.GetStartupInputs()))
	for _, input := range inputManifest.GetStartupInputs() {
		if input.GetKind() != bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR {
			t.Fatalf("startup input kind = %v, want env var", input.GetKind())
		}
		got[input.GetKey()] = input.GetStringValue()
	}
	if got[gocompiler.GoCompilerEnv] != string(gocompiler.GoCompilerGoScript) {
		t.Fatalf("startup input %s = %q, want %q", gocompiler.GoCompilerEnv, got[gocompiler.GoCompilerEnv], gocompiler.GoCompilerGoScript)
	}
}

func TestAddGoWasmOptimizeStartupCacheInputsIncludesOptimizerIdentity(t *testing.T) {
	t.Setenv(gocompiler.GoWasmOptimizeEnv, "false")

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	addGoWasmOptimizeStartupCacheInputs(inputManifest)

	got := make(map[string]string, len(inputManifest.GetStartupInputs()))
	for _, input := range inputManifest.GetStartupInputs() {
		if input.GetKind() != bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR {
			t.Fatalf("startup input kind = %v, want env var", input.GetKind())
		}
		got[input.GetKey()] = input.GetStringValue()
	}
	if got[gocompiler.GoWasmOptimizeEnv] != "false" {
		t.Fatalf("startup input %s = %q, want false", gocompiler.GoWasmOptimizeEnv, got[gocompiler.GoWasmOptimizeEnv])
	}
}

func TestAddCompilerStartupCacheInputsIncludesOptimizerIdentityForExplicitGo(t *testing.T) {
	t.Setenv(gocompiler.GoCompilerEnv, string(gocompiler.GoCompilerTinyGo))
	t.Setenv(gocompiler.GoWasmOptimizeEnv, "false")

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	addCompilerStartupCacheInputs(
		inputManifest,
		GoCompiler_GO_COMPILER_GO,
		gocompiler.GoCompilerGo,
	)

	got := make(map[string]string, len(inputManifest.GetStartupInputs()))
	for _, input := range inputManifest.GetStartupInputs() {
		got[input.GetKey()] = input.GetStringValue()
	}
	if got[gocompiler.GoWasmOptimizeEnv] != "false" {
		t.Fatalf("startup input %s = %q, want false", gocompiler.GoWasmOptimizeEnv, got[gocompiler.GoWasmOptimizeEnv])
	}
	if _, ok := got[gocompiler.GoCompilerEnv]; ok {
		t.Fatalf("explicit Go compiler mode should not depend on default-mode env: %v", got)
	}
}

func TestNewGoScriptBuildFlagsIncludesGoScriptTag(t *testing.T) {
	flags := newGoScriptBuildFlags(bldr_manifest.BuildType_DEV, false)
	if len(flags) != 1 || !strings.HasPrefix(flags[0], "-tags=") {
		t.Fatalf("GoScript build flags = %v, want single -tags flag", flags)
	}
	tags := strings.Split(strings.TrimPrefix(flags[0], "-tags="), ",")
	for _, want := range []string{"purego", gocompiler.GoScriptBuildTag} {
		if !slices.Contains(tags, want) {
			t.Fatalf("GoScript build tags missing %q: %v", want, tags)
		}
	}
}

func TestAppendInputManifestFilesKeepsGoInputsAppRootRelative(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "app")
	moduleCachePath := filepath.Join(t.TempDir(), "pkg", "mod", "github.com", "s4wave", "spacewave@v0.0.0")
	paths := []string{
		filepath.Join(sourcePath, "core", "doc.go"),
		filepath.Join(moduleCachePath, "bldr", "platform", "native.go"),
		filepath.Join(sourcePath, "go.mod"),
		filepath.Join(sourcePath, "go.sum"),
	}

	inputManifest := &bldr_manifest_builder.InputManifest{}
	if err := appendInputManifestFiles(inputManifest, sourcePath, InputFileKind_InputFileKind_GO, paths); err != nil {
		t.Fatal(err)
	}

	var gotPaths []string
	for _, file := range inputManifest.GetFiles() {
		gotPaths = append(gotPaths, filepath.ToSlash(file.GetPath()))
		meta := &InputFileMeta{}
		if err := meta.UnmarshalVT(file.GetMetadata()); err != nil {
			t.Fatal(err)
		}
		if meta.GetKind() != InputFileKind_InputFileKind_GO {
			t.Fatalf("input kind = %s, want GO", meta.GetKind())
		}
	}

	for _, want := range []string{"core/doc.go", "go.mod", "go.sum"} {
		if !slices.Contains(gotPaths, want) {
			t.Fatalf("input manifest paths missing %q: %v", want, gotPaths)
		}
	}
	for _, relPath := range gotPaths {
		if strings.HasPrefix(relPath, "..") {
			t.Fatalf("input manifest paths included external module-cache file: %v", gotPaths)
		}
	}
}

func TestAppendInputManifestFilesRejectsExternalAssetInputs(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "app")
	externalAssetPath := filepath.Join(t.TempDir(), "assets", "icon.png")
	inputManifest := &bldr_manifest_builder.InputManifest{}

	err := appendInputManifestFiles(
		inputManifest,
		sourcePath,
		InputFileKind_InputFileKind_ASSET,
		[]string{externalAssetPath},
	)
	if err == nil {
		t.Fatal("expected external asset input to fail")
	}
	if !strings.Contains(err.Error(), "path cannot be above the base dir") {
		t.Fatalf("external asset error = %v, want base-dir failure", err)
	}
	if len(inputManifest.GetFiles()) != 0 {
		t.Fatalf("external asset appended files: %v", inputManifest.GetFiles())
	}
}
