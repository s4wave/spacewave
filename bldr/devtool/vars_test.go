//go:build !js

package devtool

import (
	"maps"
	"testing"
)

// TestParseVars splits at the first equals sign, keeps an empty value, and
// takes the last value of a repeated name.
func TestParseVars(t *testing.T) {
	got, err := parseVars([]string{"deployment=staging", "empty=", "url=a=b", "deployment=production"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"deployment": "production", "empty": "", "url": "a=b"}
	if !maps.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// TestParseVarsRejectsMalformed reports the flag value that has no name or no
// equals sign.
func TestParseVarsRejectsMalformed(t *testing.T) {
	for _, spec := range []string{"deployment", "=staging", ""} {
		if _, err := parseVars([]string{spec}); err == nil {
			t.Fatalf("expected an error for %q", spec)
		}
	}
}
