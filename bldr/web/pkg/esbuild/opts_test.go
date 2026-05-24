//go:build !js

package web_pkg_esbuild

import (
	"testing"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

func TestBuildEsbuildBuildOptsAppliesReadableJavaScriptPolicy(t *testing.T) {
	le := logrus.NewEntry(logrus.New())

	readable := BuildEsbuildBuildOpts(le, t.TempDir(), t.TempDir(), "/b/pkg/", false, false, true)
	if readable.MinifyWhitespace || readable.MinifyIdentifiers || readable.MinifySyntax {
		t.Fatalf("readable opts minified: whitespace=%v identifiers=%v syntax=%v", readable.MinifyWhitespace, readable.MinifyIdentifiers, readable.MinifySyntax)
	}
	if readable.Sourcemap != esbuild_api.SourceMapNone {
		t.Fatalf("readable opts sourcemap=%v want none", readable.Sourcemap)
	}
	if readable.TreeShaking != esbuild_api.TreeShakingTrue {
		t.Fatalf("readable opts tree shaking=%v want true", readable.TreeShaking)
	}
	if readable.EntryNames != "[dir]/[name]-[hash]" {
		t.Fatalf("readable opts entry names=%q want hashed pattern", readable.EntryNames)
	}
	if !readable.Splitting {
		t.Fatal("readable opts disabled splitting")
	}

	minifiedWithMaps := BuildEsbuildBuildOpts(le, t.TempDir(), t.TempDir(), "/b/pkg/", true, true, true)
	if !minifiedWithMaps.MinifyWhitespace || !minifiedWithMaps.MinifyIdentifiers || !minifiedWithMaps.MinifySyntax {
		t.Fatalf("minified opts not fully minified: whitespace=%v identifiers=%v syntax=%v", minifiedWithMaps.MinifyWhitespace, minifiedWithMaps.MinifyIdentifiers, minifiedWithMaps.MinifySyntax)
	}
	if minifiedWithMaps.Sourcemap != esbuild_api.SourceMapLinked {
		t.Fatalf("minified opts sourcemap=%v want linked", minifiedWithMaps.Sourcemap)
	}
	if minifiedWithMaps.TreeShaking != esbuild_api.TreeShakingTrue {
		t.Fatalf("minified opts tree shaking=%v want true", minifiedWithMaps.TreeShaking)
	}
}
