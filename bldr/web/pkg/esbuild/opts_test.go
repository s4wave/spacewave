//go:build !js

package web_pkg_esbuild

import (
	"testing"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

func TestBuildEsbuildBuildOptsAppliesReadableJavaScriptPolicy(t *testing.T) {
	// Prepare the logger shared by both esbuild option configurations.
	le := logrus.NewEntry(logrus.New())

	// Build readable options with hashed names and without minification or maps.
	readable := BuildEsbuildBuildOpts(le, t.TempDir(), t.TempDir(), "/b/pkg/", false, false, true)

	// Assert readable output does not enable minification.
	if readable.MinifyWhitespace || readable.MinifyIdentifiers || readable.MinifySyntax {
		t.Fatalf("readable opts minified: whitespace=%v identifiers=%v syntax=%v", readable.MinifyWhitespace, readable.MinifyIdentifiers, readable.MinifySyntax)
	}

	// Assert readable output omits source maps.
	if readable.Sourcemap != esbuild_api.SourceMapNone {
		t.Fatalf("readable opts sourcemap=%v want none", readable.Sourcemap)
	}

	// Assert readable output retains tree shaking.
	if readable.TreeShaking != esbuild_api.TreeShakingTrue {
		t.Fatalf("readable opts tree shaking=%v want true", readable.TreeShaking)
	}

	// Assert readable output still uses the hashed entry pattern.
	if readable.EntryNames != "[dir]/[name]-[hash]" {
		t.Fatalf("readable opts entry names=%q want hashed pattern", readable.EntryNames)
	}

	// Assert readable output keeps code splitting enabled.
	if !readable.Splitting {
		t.Fatal("readable opts disabled splitting")
	}

	// Build minified options with linked source maps.
	minifiedWithMaps := BuildEsbuildBuildOpts(le, t.TempDir(), t.TempDir(), "/b/pkg/", true, true, true)

	// Assert minified output enables each supported minifier.
	if !minifiedWithMaps.MinifyWhitespace || !minifiedWithMaps.MinifyIdentifiers || !minifiedWithMaps.MinifySyntax {
		t.Fatalf("minified opts not fully minified: whitespace=%v identifiers=%v syntax=%v", minifiedWithMaps.MinifyWhitespace, minifiedWithMaps.MinifyIdentifiers, minifiedWithMaps.MinifySyntax)
	}

	// Assert minified output emits linked source maps.
	if minifiedWithMaps.Sourcemap != esbuild_api.SourceMapLinked {
		t.Fatalf("minified opts sourcemap=%v want linked", minifiedWithMaps.Sourcemap)
	}

	// Assert minified output retains tree shaking.
	if minifiedWithMaps.TreeShaking != esbuild_api.TreeShakingTrue {
		t.Fatalf("minified opts tree shaking=%v want true", minifiedWithMaps.TreeShaking)
	}
}
