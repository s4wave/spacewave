package bldr_platform

import "testing"

func TestParseWebPlatform(t *testing.T) {
	testCases := []struct {
		input        string
		expectedArch string
		expectError  bool
	}{
		{"web", "js", false},
		{"web/js", "js", false},
		{"web/wasip2", "wasip2", false},
		{"web/invalid", "", true},
		{"web/js/extra", "", true},
		{"invalid", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			web, err := ParseWebPlatform(tc.input)
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

			if val := web.GetWEBARCH(); tc.expectedArch != val {
				t.Fatalf("%s: unexpected WEBARCH: expected %s got %s", tc.input, tc.expectedArch, val)
			}
		})
	}
}

func TestWebPlatform_GetPlatformID(t *testing.T) {
	testCases := []struct {
		input          *WebPlatform
		expectedOutput string
	}{
		{&WebPlatform{WEBARCH: newStr("js")}, "web"},
		{&WebPlatform{WEBARCH: newStr("wasip2")}, "web/wasip2"},
		{&WebPlatform{}, "web"},
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
