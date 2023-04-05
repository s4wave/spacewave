package platform

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
		{"native", runtime.GOOS, runtime.GOARCH, nil, false},
		{"native/windows", "windows", runtime.GOARCH, nil, false},
		{"native/linux", "linux", runtime.GOARCH, nil, false},
		{"native/linux/arm64", "linux", "arm64", nil, false},
		{"native/linux/arm", "linux", "arm", newInt(7), false},
		{"native/linux/armv6", "linux", "arm", newInt(6), false},
		{"native/linux/armv7", "linux", "arm", newInt(7), false},
		{"native/darwin", "darwin", runtime.GOARCH, nil, false},
		{"native/invalid", "", "", nil, true},
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

func newInt(val int) *int {
	return &val
}

func newStr(str string) *string {
	return &str
}

// TestNativePlatform_GetPlatformID tests building the native platform ID.
func TestNativePlatform_GetPlatformID(t *testing.T) {
	testCases := []struct {
		input          *NativePlatform
		expectedOutput string
	}{
		{&NativePlatform{GOOS: newStr("windows"), GOARCH: newStr("amd64")}, "native/windows/amd64"},
		{&NativePlatform{GOOS: newStr("linux"), GOARCH: newStr("arm"), GOARM: newInt(6)}, "native/linux/armv6"},
		{&NativePlatform{GOOS: newStr("linux"), GOARCH: newStr("arm64")}, "native/linux/arm64"},
		{&NativePlatform{GOOS: newStr("linux"), GOARCH: newStr("arm")}, "native/linux/armv7"},
		{&NativePlatform{GOOS: newStr("darwin"), GOARCH: newStr("386")}, "native/darwin/386"},
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
