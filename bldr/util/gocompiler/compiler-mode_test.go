package gocompiler

import (
	"errors"
	"testing"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

func TestResolveGoPluginCompilerMode(t *testing.T) {
	testCases := []struct {
		name                 string
		platformID           string
		compilerMode         GoPluginCompilerMode
		defaultTinygoEnabled bool
		expected             GoPluginCompilerMode
		expectError          bool
	}{
		{
			name:         "explicit tinygo browser wasm",
			platformID:   "web/js/wasm",
			compilerMode: GoPluginCompilerModeTinyGo,
			expected:     GoPluginCompilerModeTinyGo,
		},
		{
			name:         "explicit standard go overrides default tinygo",
			platformID:   "web/js/wasm",
			compilerMode: GoPluginCompilerModeGo,
			expected:     GoPluginCompilerModeGo,
		},
		{
			name:         "explicit tinygo mode",
			platformID:   "web/js/wasm",
			compilerMode: GoPluginCompilerModeTinyGo,
			expected:     GoPluginCompilerModeTinyGo,
		},
		{
			name:         "explicit goscript mode",
			platformID:   "web/js/wasm",
			compilerMode: GoPluginCompilerModeGoScript,
			expected:     GoPluginCompilerModeGoScript,
		},
		{
			name:         "explicit tinygo unsupported js",
			platformID:   "js",
			compilerMode: GoPluginCompilerModeTinyGo,
			expectError:  true,
		},
		{
			name:       "default standard go",
			platformID: "web/js/wasm",
			expected:   GoPluginCompilerModeGo,
		},
		{
			name:                 "default browser release tinygo",
			platformID:           "web/js/wasm",
			defaultTinygoEnabled: true,
			expected:             GoPluginCompilerModeTinyGo,
		},
		{
			name:                 "explicit standard go overrides default",
			platformID:           "web/js/wasm",
			compilerMode:         GoPluginCompilerModeGo,
			defaultTinygoEnabled: true,
			expected:             GoPluginCompilerModeGo,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plat, err := bldr_platform.ParsePlatform(tc.platformID)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
			}

			actual, err := ResolveGoPluginCompilerMode(
				plat,
				tc.compilerMode,
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
