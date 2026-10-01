//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/sirupsen/logrus"
)

// TestAnalyzePackagesDoesNotRequireRootVendor analyzes a module without a vendor directory and loads no types for untagged declarations.
func TestAnalyzePackagesDoesNotRequireRootVendor(t *testing.T) {
	// Write a module fixture with a replaced dependency.
	ctx := context.Background()
	workDir := t.TempDir()

	// Write the fixture source files.
	writeFile(t, workDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2

require example.com/dep v0.0.0

replace example.com/dep => ./dep
`)
	writeFile(t, workDir, "dep/go.mod", `module example.com/dep

go 1.26.2
`)
	writeFile(t, workDir, "dep/dep.go", `package dep

const Value = "dep"
`)
	writeFile(t, workDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)
	writeFile(t, workDir, "plugin/test/plugin.go", `package test

import "example.com/dep"

var Value = dep.Value
`)

	// Analyze the plugin package without type loading.
	le := logrus.NewEntry(logrus.New())
	an, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/test"},
		[]string{"build_type_dev"},
		"linux",
		"amd64",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Untagged declarations must load no types and no vendor dir must exist.
	if len(an.typedPackages) != 0 {
		t.Fatal("untagged ordinary declarations unexpectedly loaded types")
	}
	if _, err := os.Stat(filepath.Join(workDir, "vendor")); !os.IsNotExist(err) {
		t.Fatalf("expected no vendor directory, stat err = %v", err)
	}
}

// TestAnalyzePackagesHonorsBuildTags selects factory files by the target build tags.
func TestAnalyzePackagesHonorsBuildTags(t *testing.T) {
	// Write the module and dynamic package fixture files.
	ctx := context.Background()
	workDir := t.TempDir()

	// Write the fixture source files.
	writeFile(t, workDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2
`)
	writeFile(t, workDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)
	writeFile(t, workDir, "plugin/dynamic/model.go", `package dynamic

const Value = "dynamic"
`)
	writeFile(t, workDir, "plugin/dynamic/factory.go", `//go:build !tinygo

package dynamic

func NewFactory() {}
`)

	// The standard build tags must select the factory file.
	le := logrus.NewEntry(logrus.New())
	standard, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/dynamic"},
		[]string{"build_type_release", "purego"},
		"js",
		"wasm",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(standard.controllerFactories) != 1 {
		t.Fatalf("standard analysis factories: got %d, want 1", len(standard.controllerFactories))
	}

	// The tinygo tag must exclude the factory file.
	tinygo, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/dynamic"},
		[]string{"build_type_release", "purego", "tinygo"},
		"js",
		"wasm",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tinygo.controllerFactories) != 0 {
		t.Fatalf("TinyGo analysis factories: got %d, want 0", len(tinygo.controllerFactories))
	}
}

// TestAnalyzePackagesScansImportedFactoriesOnlyWhenEnabled discovers imported same-module factories only when enabled.
func TestAnalyzePackagesScansImportedFactoriesOnlyWhenEnabled(t *testing.T) {
	// Write the module and root/child package fixture files.
	ctx := context.Background()
	workDir := t.TempDir()

	// Write the fixture source files.
	writeFile(t, workDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2
`)
	writeFile(t, workDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)
	writeFile(t, workDir, "plugin/root/root.go", `package root

import _ "github.com/s4wave/spacewave/plugin/child"

func NewFactory() {}
`)
	writeFile(t, workDir, "plugin/child/child.go", `package child

func NewFactory() {}
`)

	// Analysis without imported factory discovery must find only the root factory.
	le := logrus.NewEntry(logrus.New())
	explicitOnly, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/root"},
		[]string{"build_type_release", "purego"},
		"js",
		"wasm",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicitOnly.controllerFactories) != 1 {
		t.Fatalf("explicit-only analysis factories: got %d, want 1", len(explicitOnly.controllerFactories))
	}
	if _, ok := explicitOnly.controllerFactories["child"]; ok {
		t.Fatal("explicit-only analysis unexpectedly included imported child factory")
	}

	// Analysis with imported factory discovery must include the child factory.
	withImported, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/root"},
		[]string{"build_type_release", "purego"},
		"js",
		"wasm",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(withImported.controllerFactories) != 2 {
		t.Fatalf("imported analysis factories: got %d, want 2", len(withImported.controllerFactories))
	}
	if _, ok := withImported.controllerFactories["child"]; !ok {
		t.Fatal("imported analysis did not include imported child factory")
	}
}

// TestAnalysisProgramGoCodeFilesIncludesDependencies lists same-module
// dependency files and their embedded files as program sources.
func TestAnalysisProgramGoCodeFilesIncludesDependencies(t *testing.T) {
	// Write the module and root/dep package fixture files.
	ctx := context.Background()
	workDir := t.TempDir()

	// Write the fixture source files.
	writeFile(t, workDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2
`)
	writeFile(t, workDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)
	writeFile(t, workDir, "plugin/root/root.go", `package root

import "github.com/s4wave/spacewave/lib/dep"

var Value = dep.Value
`)
	writeFile(t, workDir, "lib/dep/dep.go", `package dep

import _ "embed"

//go:embed version.txt
var Value string
`)
	writeFile(t, workDir, "lib/dep/version.txt", "1.0.0\n")

	// Analyze the root package with its same-module dependency.
	le := logrus.NewEntry(logrus.New())
	an, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/root"},
		[]string{"build_type_release", "purego"},
		"js",
		"wasm",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	// The program sources must include the root, dependency, and embedded files.
	programFiles := an.GetProgramSourceFiles()
	var programRelPaths []string
	for _, pkgFiles := range programFiles {
		for _, file := range pkgFiles {
			relPath, err := filepath.Rel(workDir, file)
			if err != nil {
				t.Fatal(err)
			}
			programRelPaths = append(programRelPaths, filepath.ToSlash(relPath))
		}
	}
	for _, want := range []string{"plugin/root/root.go", "lib/dep/dep.go", "lib/dep/version.txt"} {
		if !slices.Contains(programRelPaths, want) {
			t.Fatalf("program files missing %q: %v", want, programRelPaths)
		}
	}

	// The root package files must exclude the dependency package.
	rootFiles := an.GetGoCodeFiles()
	var rootRelPaths []string
	for _, pkgFiles := range rootFiles {
		for _, file := range pkgFiles {
			relPath, err := filepath.Rel(workDir, an.GetFileToken(file).Name())
			if err != nil {
				t.Fatal(err)
			}
			rootRelPaths = append(rootRelPaths, filepath.ToSlash(relPath))
		}
	}
	if slices.Contains(rootRelPaths, "lib/dep/dep.go") {
		t.Fatalf("root files unexpectedly included dependency: %v", rootRelPaths)
	}
}

// TestAnalysisProgramGoCodeFilesExcludesHelperModule omits files from a separate helper module.
func TestAnalysisProgramGoCodeFilesExcludesHelperModule(t *testing.T) {
	// Write plugin and helper module fixture directories.
	ctx := context.Background()
	testDir := t.TempDir()
	workDir := filepath.Join(testDir, "plugin")
	spacewaveDir := filepath.Join(testDir, "spacewave")

	// Write the plugin module fixture files.
	writeFile(t, workDir, "go.mod", `module example.com/plugin

go 1.26.2

require github.com/s4wave/spacewave v0.0.0

replace github.com/s4wave/spacewave => `+spacewaveDir+`
`)
	writeFile(t, workDir, "plugin/root/root.go", `package root

import "strings"

var Value = strings.TrimSpace(" root ")
`)

	// Write the helper module fixture files.
	writeFile(t, spacewaveDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2
`)
	writeFile(t, spacewaveDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)

	// Analyze the plugin module against the replaced helper module.
	le := logrus.NewEntry(logrus.New())
	an, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/root"},
		[]string{"build_type_release", "purego"},
		"js",
		"wasm",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Collect the program source paths relative to the plugin module.
	programFiles := an.GetProgramSourceFiles()
	var programRelPaths []string
	for _, pkgFiles := range programFiles {
		for _, file := range pkgFiles {
			relPath, err := filepath.Rel(workDir, file)
			if err != nil {
				t.Fatal(err)
			}
			programRelPaths = append(programRelPaths, filepath.ToSlash(relPath))
		}
	}

	// The helper module files must stay out of the program sources.
	if !slices.Contains(programRelPaths, "plugin/root/root.go") {
		t.Fatalf("program files missing root package: %v", programRelPaths)
	}
	helperPath := filepath.ToSlash(filepath.Join("..", "spacewave", "bldr", "web", "bundler", "output.go"))
	if slices.Contains(programRelPaths, helperPath) {
		t.Fatalf("program files include analysis helper module: %v", programRelPaths)
	}
}

// TestAnalyzePackagesReportsLoadFailureContext names the target and patterns in load failures.
func TestAnalyzePackagesReportsLoadFailureContext(t *testing.T) {
	// Write the module and root package fixture files.
	ctx := context.Background()
	workDir := t.TempDir()

	// Write the fixture source files.
	writeFile(t, workDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2
`)
	writeFile(t, workDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)
	writeFile(t, workDir, "plugin/root/root.go", `package root

import "example.invalid/missing"

var Value = missing.Value
`)

	// Load the package with a missing import and capture the failure.
	le := logrus.NewEntry(logrus.New())
	_, err := AnalyzePackages(
		ctx,
		le,
		workDir,
		[]string{"./plugin/root"},
		[]string{"build_type_dev"},
		"js",
		"wasm",
		false,
	)
	if err == nil {
		t.Fatal("expected package load failure")
	}

	// The error must name the target, patterns, tags, and environment.
	errText := err.Error()
	for _, want := range []string{
		"package load failed",
		"patterns=github.com/s4wave/spacewave/bldr/web/bundler,github.com/s4wave/spacewave/plugin/root",
		"tags=bldr_analyze,build_type_dev",
		"GOOS=js",
		"GOARCH=wasm",
		"workDir=" + workDir,
		"example.invalid/missing",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("load failure error missing %q:\n%s", want, errText)
		}
	}
	if strings.Contains(errText, "could not find "+EsbuildOutputPkgPath+"."+EsbuildOutputTypeName) {
		t.Fatalf("load failure was reported as a missing type:\n%s", errText)
	}
}

// TestNewBuildTagsForAnalyzeIncludesTinyGoTag adds the tinygo tag for TinyGo targets.
func TestNewBuildTagsForAnalyzeIncludesTinyGoTag(t *testing.T) {
	// The TinyGo tags must carry the tinygo and compatibility tags.
	tags := newBuildTagsForAnalyze(nil, bldr_manifest.BuildType_RELEASE, gocompiler.GoCompilerTinyGo)
	for _, want := range []string{
		"build_type_release",
		"purego",
		"tinygo",
		gocompiler.BldrTinyGoJSImportBuildTag,
		gocompiler.SQLLiteBuildTag,
	} {
		if !slices.Contains(tags, want) {
			t.Fatalf("TinyGo analysis tags missing %q: %v", want, tags)
		}
	}

	// Native Go keeps its assembly, including hardware hashing.
	standardTags := newBuildTagsForAnalyze(nil, bldr_manifest.BuildType_RELEASE, gocompiler.GoCompilerGo)

	// The native tags must exclude purego, tinygo, and sql_lite.
	if slices.Contains(standardTags, gocompiler.PureGoBuildTag) {
		t.Fatalf("native Go analysis tags unexpectedly include purego: %v", standardTags)
	}
	if slices.Contains(standardTags, "tinygo") {
		t.Fatalf("standard Go analysis tags unexpectedly include tinygo: %v", standardTags)
	}
	if slices.Contains(standardTags, gocompiler.SQLLiteBuildTag) {
		t.Fatalf("standard Go analysis tags unexpectedly include sql_lite: %v", standardTags)
	}
}

// TestNewBuildTagsForAnalyzeIncludesGoScriptTag adds the goscript tag for GoScript targets.
func TestNewBuildTagsForAnalyzeIncludesGoScriptTag(t *testing.T) {
	// The GoScript tags must carry the goscript and sql_lite tags.
	tags := newBuildTagsForAnalyze(nil, bldr_manifest.BuildType_RELEASE, gocompiler.GoCompilerGoScript)
	for _, want := range []string{
		"build_type_release",
		"purego",
		gocompiler.GoScriptBuildTag,
		gocompiler.SQLLiteBuildTag,
	} {
		if !slices.Contains(tags, want) {
			t.Fatalf("GoScript analysis tags missing %q: %v", want, tags)
		}
	}
}

// writeFile creates a fixture source file and its parent directories.
func writeFile(t *testing.T, root, relPath, contents string) {
	t.Helper()

	// Create the parent directories and write the file.
	absPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
