package bldr_platform

import "testing"

// TestParseWebPlatform tests the ParseWebPlatform function.
func TestParseWebPlatform(t *testing.T) {
	testCases := []struct {
		input       string
		expectError bool
	}{
		{"web", false},
		{"web/js", true},     // Invalid format now
		{"web/wasip2", true}, // Invalid format now
		{"web/invalid", true},
		{"web/js/extra", true},
		{"invalid", true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := ParseWebPlatform(tc.input)
			if tc.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tc.input)
				}
				return
			}
		})
	}
}

// TestWebPlatform_GetBasePlatformID tests the GetBasePlatformID method.
func TestWebPlatform_GetBasePlatformID(t *testing.T) {
	platform := &WebPlatform{}
	expectedOutput := PlatformID_WEB
	out := platform.GetBasePlatformID()

	if out != expectedOutput {
		t.Errorf("unexpected base platform id: expected %s, got %s", expectedOutput, out)
	}
}

// TestWebPlatform_GetExecutableExt tests the GetExecutableExt method.
func TestWebPlatform_GetExecutableExt(t *testing.T) {
	platform := &WebPlatform{}
	expectedOutput := ".mjs"
	out := platform.GetExecutableExt()

	if out != expectedOutput {
		t.Errorf("unexpected executable extension: expected %s, got %s", expectedOutput, out)
	}
}

// TestNewWebPlatformJs tests the NewWebPlatformJs constructor.
func TestNewWebPlatformJs(t *testing.T) {
	platform := NewWebPlatformJs()
	if platform == nil {
		t.Fatal("NewWebPlatformJs returned nil")
	}
	// Check if InputPlatformID is set correctly, though it's simple now.
	if platform.GetInputPlatformID() != PlatformID_WEB {
		t.Errorf("NewWebPlatformJs did not set InputPlatformID correctly: expected %s, got %s", PlatformID_WEB, platform.GetInputPlatformID())
	}
	// Check other methods behave as expected for the default web platform.
	if platform.GetPlatformID() != PlatformID_WEB {
		t.Errorf("NewWebPlatformJs platform ID mismatch: expected %s, got %s", PlatformID_WEB, platform.GetPlatformID())
	}
	if platform.GetBasePlatformID() != PlatformID_WEB {
		t.Errorf("NewWebPlatformJs base platform ID mismatch: expected %s, got %s", PlatformID_WEB, platform.GetBasePlatformID())
	}
	if platform.GetExecutableExt() != ".mjs" {
		t.Errorf("NewWebPlatformJs executable extension mismatch: expected %s, got %s", ".mjs", platform.GetExecutableExt())
	}
}

// TestWebPlatform_GetPlatformID tests the GetPlatformID method.
func TestWebPlatform_GetPlatformID(t *testing.T) {
	// Since WEBARCH is removed, the platform ID is always "web".
	platform := &WebPlatform{}
	expectedOutput := "web"
	out := platform.GetPlatformID()

	if out != expectedOutput {
		t.Errorf("unexpected platform id: expected %s, got %s", expectedOutput, out)
	}
}

// TestWebPlatform_GetInputPlatformID tests the GetInputPlatformID method.
func TestWebPlatform_GetInputPlatformID(t *testing.T) {
	testCases := []struct {
		name           string
		input          *WebPlatform
		expectedOutput string
	}{
		{
			name:           "With InputPlatformID",
			input:          &WebPlatform{InputPlatformID: "web"},
			expectedOutput: "web",
		},
		{
			name:           "Without InputPlatformID",
			input:          &WebPlatform{},
			expectedOutput: "web", // Falls back to GetPlatformID()
		},
		{
			name:           "Constructed with NewWebPlatformJs",
			input:          NewWebPlatformJs(),
			expectedOutput: "web",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.input.GetInputPlatformID()
			if out != tc.expectedOutput {
				t.Errorf("%s: unexpected input platform id: expected %s, got %s", tc.name, tc.expectedOutput, out)
			}
		})
	}
}
