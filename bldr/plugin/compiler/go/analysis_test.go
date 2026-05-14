//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
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
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tinygo.controllerFactories) != 0 {
		t.Fatalf("TinyGo analysis factories: got %d, want 0", len(tinygo.controllerFactories))
	}
}

func TestNewBuildTagsForAnalyzeIncludesTinyGoTag(t *testing.T) {
	tags := newBuildTagsForAnalyze(bldr_manifest.BuildType_RELEASE, false, true)
	for _, want := range []string{"build_type_release", "purego", "tinygo"} {
		if !slices.Contains(tags, want) {
			t.Fatalf("TinyGo analysis tags missing %q: %v", want, tags)
		}
	}

	standardTags := newBuildTagsForAnalyze(bldr_manifest.BuildType_RELEASE, false, false)
	if slices.Contains(standardTags, "tinygo") {
		t.Fatalf("standard Go analysis tags unexpectedly include tinygo: %v", standardTags)
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
