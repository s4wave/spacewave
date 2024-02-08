package npm

import (
	"strings"
	"testing"
)

// TestLoadPackageVersion tests loading a package version from package.json.
func TestLoadPackageVersion(t *testing.T) {
	ver, err := LoadPackageVersion("../../package.json", "react")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !strings.HasPrefix(ver, "^") {
		t.Fatalf("expected a version with a ^ prefix: %v", ver)
	}
	t.Logf("parsed react package version: %v", ver)
}
