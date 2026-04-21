package bldr_platform

import "testing"

// TestParseJsPlatform tests the ParseJsPlatform function.
func TestParseJsPlatform(t *testing.T) {
	testCases := []struct {
		input       string
		expectError bool
	}{
		{"js", false},
		{"js/invalid", true},
		{"js/extra/params", true},
		{"invalid", true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := ParseJsPlatform(tc.input)
			if tc.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tc.input)
				}
				return
			}
		})
	}
}

// TestJsPlatform_GetBasePlatformID tests the GetBasePlatformID method.
func TestJsPlatform_GetBasePlatformID(t *testing.T) {
	platform := &JsPlatform{}
	expectedOutput := PlatformID_JS
	out := platform.GetBasePlatformID()

	if out != expectedOutput {
		t.Errorf("unexpected base platform id: expected %s, got %s", expectedOutput, out)
	}
}

// TestJsPlatform_GetExecutableExt tests the GetExecutableExt method.
func TestJsPlatform_GetExecutableExt(t *testing.T) {
	platform := &JsPlatform{}
	expectedOutput := ".mjs"
	out := platform.GetExecutableExt()

	if out != expectedOutput {
		t.Errorf("unexpected executable extension: expected %s, got %s", expectedOutput, out)
	}
}

// TestNewJsPlatform tests the NewJsPlatform constructor.
func TestNewJsPlatform(t *testing.T) {
	platform := NewJsPlatform()
	if platform == nil {
		t.Fatal("NewJsPlatform returned nil")
	}
	// Check if InputPlatformID is set correctly.
	if platform.GetInputPlatformID() != PlatformID_JS {
		t.Errorf("NewJsPlatform did not set InputPlatformID correctly: expected %s, got %s", PlatformID_JS, platform.GetInputPlatformID())
	}
	// Check other methods behave as expected for the default js platform.
	if platform.GetPlatformID() != PlatformID_JS {
		t.Errorf("NewJsPlatform platform ID mismatch: expected %s, got %s", PlatformID_JS, platform.GetPlatformID())
	}
	if platform.GetBasePlatformID() != PlatformID_JS {
		t.Errorf("NewJsPlatform base platform ID mismatch: expected %s, got %s", PlatformID_JS, platform.GetBasePlatformID())
	}
	if platform.GetExecutableExt() != ".mjs" {
		t.Errorf("NewJsPlatform executable extension mismatch: expected %s, got %s", ".mjs", platform.GetExecutableExt())
	}
}

// TestJsPlatform_GetPlatformID tests the GetPlatformID method.
func TestJsPlatform_GetPlatformID(t *testing.T) {
	platform := &JsPlatform{}
	expectedOutput := "js"
	out := platform.GetPlatformID()

	if out != expectedOutput {
		t.Errorf("unexpected platform id: expected %s, got %s", expectedOutput, out)
	}
}

// TestJsPlatform_GetInputPlatformID tests the GetInputPlatformID method.
func TestJsPlatform_GetInputPlatformID(t *testing.T) {
	testCases := []struct {
		name           string
		input          *JsPlatform
		expectedOutput string
	}{
		{
			name:           "With InputPlatformID",
			input:          &JsPlatform{InputPlatformID: "js"},
			expectedOutput: "js",
		},
		{
			name:           "Without InputPlatformID",
			input:          &JsPlatform{},
			expectedOutput: "js", // Falls back to GetPlatformID()
		},
		{
			name:           "Constructed with NewJsPlatform",
			input:          NewJsPlatform(),
			expectedOutput: "js",
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
