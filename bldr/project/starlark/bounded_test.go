//go:build !js

package bldr_project_starlark

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// TestMain runs the bounded evaluation child when the test binary is started
// as one, as the spacewave command does, and the tests otherwise.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == BoundedCommand {
		if err := RunBounded(context.Background(), os.Stdin, os.Stdout); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestEvaluateFSBoundedMatchesEvaluateFS returns the config and loaded files
// that the evaluation in this process returns, including loads.
func TestEvaluateFSBoundedMatchesEvaluateFS(t *testing.T) {
	// Evaluate a project that loads a relative module and a vendored one.
	fsys := fstest.MapFS{
		"bldr.star": {Data: []byte(`
load("lib/common.star", "SHARED_PKGS")
load("@go/example.com/mod/pkgs.star", "VENDORED")
project(id="test")
manifest("core",
    builder="bldr/plugin/compiler/go",
    config={"goPkgs": SHARED_PKGS + VENDORED},
)
`)},
		"lib/common.star":                  {Data: []byte(`SHARED_PKGS = ["./shared/pkg1"]`)},
		"vendor/example.com/mod/pkgs.star": {Data: []byte(`VENDORED = ["./vendored"]`)},
	}
	want, err := EvaluateFS(t.Context(), fsys, "bldr.star")
	if err != nil {
		t.Fatal(err)
	}
	got, err := EvaluateFSBounded(t.Context(), fsys, "bldr.star", 256<<20)
	if err != nil {
		t.Fatal(err)
	}

	// Require the same config and loaded files.
	if !got.Config.EqualVT(want.Config) {
		t.Fatalf("expected config %v, got %v", want.Config, got.Config)
	}
	if !slices.Equal(got.LoadedFiles, want.LoadedFiles) {
		t.Fatalf("expected loaded files %v, got %v", want.LoadedFiles, got.LoadedFiles)
	}
}

// TestEvaluateFSBoundedReportsEvaluationError returns the error of a project
// that does not evaluate.
func TestEvaluateFSBoundedReportsEvaluationError(t *testing.T) {
	fsys := fstest.MapFS{"bldr.star": {Data: []byte("fail('broken')\n")}}
	_, err := EvaluateFSBounded(t.Context(), fsys, "bldr.star", 256<<20)
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("expected the evaluation error, got %v", err)
	}
}

// TestReadFileRefusesLargeFile stops at the size limit.
func TestReadFileRefusesLargeFile(t *testing.T) {
	fsys := fstest.MapFS{
		"fits":    {Data: make([]byte, MaxFileSize)},
		"too-big": {Data: make([]byte, MaxFileSize+1)},
	}
	if _, err := ReadFile(fsys, "fits"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(fsys, "too-big"); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}
