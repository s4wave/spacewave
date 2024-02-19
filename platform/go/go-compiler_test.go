package bldr_platform_go

import (
	"testing"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"golang.org/x/exp/slices"
)

func TestGoCompilerEnvVars(t *testing.T) {
	testCases := []struct {
		platformID   string
		expectedVars []string
	}{
		{"native/windows/amd64", []string{"GOOS=windows", "GOARCH=amd64"}},
		{"native/windows/armv6", []string{"GOOS=windows", "GOARCH=arm", "GOARM=6"}},
		{"native/linux/armv5", []string{"GOOS=linux", "GOARCH=arm", "GOARM=5"}},
		{"native/darwin/arm64", []string{"GOOS=darwin", "GOARCH=arm64"}},
		{"web", []string{"GOOS=wasip1", "GOARCH=wasm"}},
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
