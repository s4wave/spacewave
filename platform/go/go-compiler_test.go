package bldr_platform_go

import (
	"slices"
	"testing"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
)

func TestTinyGoTarget(t *testing.T) {
	testCases := []struct {
		platformID     string
		expectedTarget string
		expectError    bool
	}{
		{"native/wasi/wasm", "wasm-unknown", false},
		{"native/linux/amd64", "", true},
		{"js", "", true},
		{"native/js/wasm", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.platformID, func(t *testing.T) {
			plat, err := bldr_platform.ParsePlatform(tc.platformID)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
				return
			}

			target, err := PlatformToTinyGoTarget(plat)
			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: expected error but got none", tc.platformID)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
				return
			}
			if target != tc.expectedTarget {
				t.Fatalf("%s: expected %s got %s", tc.platformID, tc.expectedTarget, target)
			}
		})
	}
}

func TestGoCompilerEnvVars(t *testing.T) {
	testCases := []struct {
		platformID   string
		expectedVars []string
	}{
		{"native/windows/amd64", []string{"GOOS=windows", "GOARCH=amd64"}},
		{"native/windows/armv6", []string{"GOOS=windows", "GOARCH=arm", "GOARM=6"}},
		{"native/linux/armv5", []string{"GOOS=linux", "GOARCH=arm", "GOARM=5"}},
		{"native/darwin/arm64", []string{"GOOS=darwin", "GOARCH=arm64"}},
		{"native/js/wasm", []string{"GOOS=js", "GOARCH=wasm"}},
		{"native/wasi/wasm", []string{"GOOS=wasi", "GOARCH=wasm"}},
		{"js", []string{"GOOS=js", "GOARCH=wasm"}},
	}

	for _, tc := range testCases {
		t.Run(tc.platformID, func(t *testing.T) {
			plat, err := bldr_platform.ParsePlatform(tc.platformID)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
				return
			}

			genv, err := PlatformToGoEnv(plat)
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.platformID, err.Error())
				return
			}
			if !slices.Equal(genv, tc.expectedVars) {
				t.Fatalf("%s: expected %v got %v", tc.platformID, tc.expectedVars, genv)
			}
		})
	}
}
