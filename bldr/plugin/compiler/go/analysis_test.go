//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
