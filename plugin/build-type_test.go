package plugin

import "testing"

// TestToBuildType tests constructing a BuildType from strings.
func TestToBuildType(t *testing.T) {
	cases := map[string]BuildType{
		"development":    BuildType_DEV,
		"production":     BuildType_RELEASE,
		"DEV":            BuildType_DEV,
		"dEV":            BuildType_DEV,
		"   pRoDuCtioN ": BuildType_RELEASE,
		"   deV ":        BuildType_DEV,
		" fubar ":        "",
		"":               "",
	}
	for val, expected := range cases {
		bt := ToBuildType(val)
		err := bt.Validate(false)
		if expected == "" {
			if err == nil {
				t.Fatalf("case %q: expected error but got none", val)
			} else {
				t.Logf("case %q: got correct output: %s", val, err.Error())
			}
		} else {
			if err != nil {
				t.Fatalf("case %q: expected %s but got error %s", val, expected, err.Error())
			}
			if bt != expected {
				t.Fatalf("case %q: expected %s but got %s", val, expected, bt)
			}
		}
	}
}
