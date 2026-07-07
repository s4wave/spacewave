package gocompiler

import (
	"errors"
	"testing"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

func TestResolveGoCompiler(t *testing.T) {
	testCases := []struct {
		name                 string
		platformID           string
		goCompiler           GoCompiler
		defaultTinygoEnabled bool
		expected             GoCompiler
		expectError          bool
	}{
		{
			name:       "explicit standard go overrides browser default",
			platformID: "web/js/wasm",
			goCompiler: GoCompilerGo,
			expected:   GoCompilerGo,
		},
		{
			name:       "explicit tinygo overrides browser default",
			platformID: "web/js/wasm",
			goCompiler: GoCompilerTinyGo,
			expected:   GoCompilerTinyGo,
		},
		{
			name:       "explicit goscript mode",
			platformID: "web/js/wasm",
			goCompiler: GoCompilerGoScript,
			expected:   GoCompilerGoScript,
		},
		{
			name:        "explicit tinygo unsupported js",
			platformID:  "js",
			goCompiler:  GoCompilerTinyGo,
			expectError: true,
		},
		{
			name:       "default browser wasm uses goscript",
			platformID: "web/js/wasm",
			expected:   GoCompilerGoScript,
		},
		{
			name:       "default browser js uses goscript",
			platformID: "js",
			expected:   GoCompilerGoScript,
		},
		{
			name:       "default desktop uses standard go",
			platformID: "desktop/windows/armv6",
			expected:   GoCompilerGo,
		},
		{
			name:                 "default browser ignores tinygo release default",
			platformID:           "web/js/wasm",
			defaultTinygoEnabled: true,
			expected:             GoCompilerGoScript,
		},
		{
			name:                 "explicit standard go overrides default",
			platformID:           "web/js/wasm",
			goCompiler:           GoCompilerGo,
			defaultTinygoEnabled: true,
			expected:             GoCompilerGo,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(GoCompilerEnv, "")
			plat, err := bldr_platform.ParsePlatform(tc.platformID)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
			}

			actual, err := ResolveGoCompiler(
				plat,
				tc.goCompiler,
				tc.defaultTinygoEnabled,
			)
			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: expected error but got none", tc.platformID)
				}
				if !errors.Is(err, bldr_platform_go.ErrTinyGoUnsupported) {
					t.Fatalf("%s: expected ErrTinyGoUnsupported, got %s", tc.platformID, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
			}
			if actual != tc.expected {
				t.Fatalf("%s: compiler mode = %s, want %s", tc.platformID, actual, tc.expected)
			}
		})
	}
}

func TestResolveGoCompilerUsesEnvForDefault(t *testing.T) {
	t.Setenv(GoCompilerEnv, string(GoCompilerGoScript))
	plat, err := bldr_platform.ParsePlatform("web/js/wasm")
	if err != nil {
		t.Fatal(err)
	}

	actual, err := ResolveGoCompiler(plat, GoCompilerDefault, false)
	if err != nil {
		t.Fatal(err)
	}
	if actual != GoCompilerGoScript {
		t.Fatalf("compiler mode = %s, want %s", actual, GoCompilerGoScript)
	}
}

func TestResolveGoCompilerExplicitOverridesEnv(t *testing.T) {
	t.Setenv(GoCompilerEnv, string(GoCompilerGoScript))
	plat, err := bldr_platform.ParsePlatform("web/js/wasm")
	if err != nil {
		t.Fatal(err)
	}

	actual, err := ResolveGoCompiler(plat, GoCompilerGo, false)
	if err != nil {
		t.Fatal(err)
	}
	if actual != GoCompilerGo {
		t.Fatalf("compiler mode = %s, want %s", actual, GoCompilerGo)
	}
}
