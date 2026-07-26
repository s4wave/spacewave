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

func TestAnalyzePackagesDoesNotRequireRootVendor(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

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
	if an.webBundlerOutputType == nil {
		t.Fatal("expected esbuild output type to be loaded")
	}
	if _, err := os.Stat(filepath.Join(workDir, "vendor")); !os.IsNotExist(err) {
		t.Fatalf("expected no vendor directory, stat err = %v", err)
	}
}

func TestAnalyzePackagesHonorsBuildTags(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

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

func TestAnalyzePackagesScansImportedFactoriesOnlyWhenEnabled(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

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

func TestAnalysisProgramGoCodeFilesIncludesDependencies(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

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

const Value = "dep"
`)

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

	programFiles := an.GetProgramGoCodeFiles()
	var programRelPaths []string
	for _, pkgFiles := range programFiles {
		for _, file := range pkgFiles {
			relPath, err := filepath.Rel(workDir, an.GetFileToken(file).Name())
			if err != nil {
				t.Fatal(err)
			}
			programRelPaths = append(programRelPaths, filepath.ToSlash(relPath))
		}
	}
	for _, want := range []string{"plugin/root/root.go", "lib/dep/dep.go"} {
		if !slices.Contains(programRelPaths, want) {
			t.Fatalf("program files missing %q: %v", want, programRelPaths)
		}
	}

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

func TestAnalysisProgramGoCodeFilesExcludesHelperModule(t *testing.T) {
	ctx := context.Background()
	testDir := t.TempDir()
	workDir := filepath.Join(testDir, "plugin")
	spacewaveDir := filepath.Join(testDir, "spacewave")

	writeFile(t, workDir, "go.mod", `module example.com/plugin

go 1.26.2

require github.com/s4wave/spacewave v0.0.0

replace github.com/s4wave/spacewave => `+spacewaveDir+`
`)
	writeFile(t, workDir, "plugin/root/root.go", `package root

const Value = "root"
`)

	writeFile(t, spacewaveDir, "go.mod", `module github.com/s4wave/spacewave

go 1.26.2
`)
	writeFile(t, spacewaveDir, "bldr/web/bundler/output.go", `package bundler

type WebBundlerOutput struct{}
`)

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

	programFiles := an.GetProgramGoCodeFiles()
	var programRelPaths []string
	for _, pkgFiles := range programFiles {
		for _, file := range pkgFiles {
			relPath, err := filepath.Rel(workDir, an.GetFileToken(file).Name())
			if err != nil {
				t.Fatal(err)
			}
			programRelPaths = append(programRelPaths, filepath.ToSlash(relPath))
		}
	}

	if !slices.Contains(programRelPaths, "plugin/root/root.go") {
		t.Fatalf("program files missing root package: %v", programRelPaths)
	}
	helperPath := filepath.ToSlash(filepath.Join("..", "spacewave", "bldr", "web", "bundler", "output.go"))
	if slices.Contains(programRelPaths, helperPath) {
		t.Fatalf("program files include analysis helper module: %v", programRelPaths)
	}
}

func TestAnalyzePackagesReportsLoadFailureContext(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

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

func TestNewBuildTagsForAnalyzeIncludesTinyGoTag(t *testing.T) {
	tags := newBuildTagsForAnalyze(bldr_manifest.BuildType_RELEASE, false, gocompiler.GoCompilerTinyGo)
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

	standardTags := newBuildTagsForAnalyze(bldr_manifest.BuildType_RELEASE, false, gocompiler.GoCompilerGo)
	if slices.Contains(standardTags, "tinygo") {
		t.Fatalf("standard Go analysis tags unexpectedly include tinygo: %v", standardTags)
	}
	if slices.Contains(standardTags, gocompiler.SQLLiteBuildTag) {
		t.Fatalf("standard Go analysis tags unexpectedly include sql_lite: %v", standardTags)
	}
}

func TestNewBuildTagsForAnalyzeIncludesGoScriptTag(t *testing.T) {
	tags := newBuildTagsForAnalyze(bldr_manifest.BuildType_RELEASE, false, gocompiler.GoCompilerGoScript)
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

func writeFile(t *testing.T, root, relPath, contents string) {
	t.Helper()
	absPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
