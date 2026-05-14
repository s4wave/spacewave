package gocompiler

import (
	"errors"
	"testing"

	"github.com/aperturerobotics/util/enabled"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

func TestResolveTinyGoEnabled(t *testing.T) {
	testCases := []struct {
		name           string
		platformID     string
		opt            enabled.Enabled
		defaultEnabled bool
		expected       bool
		expectError    bool
	}{
		{
			name:       "explicit browser go plugin",
			platformID: "web/js/wasm",
			opt:        enabled.Enabled_ENABLE,
			expected:   true,
		},
		{
			name:        "explicit js unsupported",
			platformID:  "js",
			opt:         enabled.Enabled_ENABLE,
			expectError: true,
		},
		{
			name:        "explicit native unsupported",
			platformID:  "desktop/linux/amd64",
			opt:         enabled.Enabled_ENABLE,
			expectError: true,
		},
		{
			name:       "default preserves standard go",
			platformID: "web/js/wasm",
			opt:        enabled.Enabled_DEFAULT,
			expected:   false,
		},
		{
			name:           "default can opt into supported browser",
			platformID:     "web/js/wasm",
			opt:            enabled.Enabled_DEFAULT,
			defaultEnabled: true,
			expected:       true,
		},
		{
			name:           "disable overrides default",
			platformID:     "web/js/wasm",
			opt:            enabled.Enabled_DISABLE,
			defaultEnabled: true,
			expected:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plat, err := bldr_platform.ParsePlatform(tc.platformID)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
			}

			actual, err := ResolveTinyGoEnabled(plat, tc.opt, tc.defaultEnabled)
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
				t.Fatalf("%s: expected %v got %v", tc.platformID, tc.expected, actual)
			}
		})
	}
}
