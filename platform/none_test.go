package bldr_platform

import "testing"

func TestParseNonePlatform(t *testing.T) {
	testCases := []struct {
		input       string
		expectError bool
	}{
		{"none", false},
		{"invalid", true},
		{"none/extra", true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			none, err := ParseNonePlatform(tc.input)
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

			if id := none.GetPlatformID(); id != PlatformID_NONE {
				t.Fatalf("%s: unexpected platform id: expected %s got %s", tc.input, PlatformID_NONE, id)
			}
		})
	}
}

func TestNonePlatform_GetPlatformID(t *testing.T) {
	none := NewNonePlatform()
	if id := none.GetPlatformID(); id != PlatformID_NONE {
		t.Fatalf("unexpected platform id: expected %s got %s", PlatformID_NONE, id)
	}
}
