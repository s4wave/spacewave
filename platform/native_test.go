package bldr_platform

import (
	"runtime"
	"testing"
)

func TestParseNativePlatform(t *testing.T) {
	testCases := []struct {
		input          string
		expectedGOOS   string
		expectedGOARCH string
		expectedGOARM  *int
		expectError    bool
	}{
		{"desktop", runtime.GOOS, runtime.GOARCH, nil, false},
		{"desktop/windows", "windows", runtime.GOARCH, nil, false},
		{"desktop/linux", "linux", runtime.GOARCH, nil, false},
		{"desktop/linux/arm64", "linux", "arm64", nil, false},
		{"desktop/linux/arm", "linux", "arm", new(7), false},
		{"desktop/linux/armv6", "linux", "arm", new(6), false},
		{"desktop/linux/armv7", "linux", "arm", new(7), false},
		{"desktop/darwin", "darwin", runtime.GOARCH, nil, false},
		{"desktop/js/wasm", "js", "wasm", nil, false},
		{"desktop/wasi/wasm", "wasi", "wasm", nil, false},
		{"desktop/invalid", "", "", nil, true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			native, err := ParseNativePlatform(tc.input)
			if tc.expectError {
				if err == nil {
					t.Fatalf("%s: expected error but got none", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %s", tc.input, err.Error())
				return
			}

			if val := native.GetGOOS(); tc.expectedGOOS != val {
				t.Fatalf("%s: unexpected GOOS: expected %s got %s", tc.input, tc.expectedGOOS, val)
			}
			if val := native.GetGOARCH(); tc.expectedGOARCH != val {
				t.Fatalf("%s: unexpected GOARCH: expected %s got %s", tc.input, tc.expectedGOARCH, val)
			}
			expectedGOARM := 0
			if tc.expectedGOARM != nil {
				expectedGOARM = *tc.expectedGOARM
			}
			if val := native.GetGOARM(); expectedGOARM != val {
				t.Fatalf("%s: unexpected GOARM: expected %v got %v", tc.input, expectedGOARM, val)
			}
		})
	}
}

// TestNativePlatform_GetPlatformID tests building the native platform ID.
func TestNativePlatform_GetPlatformID(t *testing.T) {
	testCases := []struct {
		input          *NativePlatform
		expectedOutput string
	}{
		{&NativePlatform{GOOS: new("windows"), GOARCH: new("amd64")}, "desktop/windows/amd64"},
		{&NativePlatform{GOOS: new("linux"), GOARCH: new("arm"), GOARM: new(6)}, "desktop/linux/armv6"},
		{&NativePlatform{GOOS: new("linux"), GOARCH: new("arm64")}, "desktop/linux/arm64"},
		{&NativePlatform{GOOS: new("linux"), GOARCH: new("arm")}, "desktop/linux/armv7"},
		{&NativePlatform{GOOS: new("darwin"), GOARCH: new("386")}, "desktop/darwin/386"},
		{&NativePlatform{GOOS: new("js"), GOARCH: new("wasm")}, "desktop/js/wasm"},
		{&NativePlatform{GOOS: new("wasi"), GOARCH: new("wasm")}, "desktop/wasi/wasm"},
	}

	for _, tc := range testCases {
		out := tc.input.GetPlatformID()
		t.Run(tc.expectedOutput, func(t *testing.T) {
			if out != tc.expectedOutput {
				t.Fatalf("%s: unexpected platform id: expected %s", out, tc.expectedOutput)
			}
		})
	}
}
