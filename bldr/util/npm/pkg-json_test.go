package npm

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLoadPackageVersion tests loading a package version from package.json.
func TestLoadPackageVersion(t *testing.T) {
	// Locate the package manifest relative to the test source.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}

	// Load the React dependency version from package.json.
	ver, err := LoadPackageVersion(filepath.Clean(filepath.Join(filepath.Dir(file), "../../../package.json")), "react")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the React dependency retains its caret version constraint.
	if !strings.HasPrefix(ver, "^") {
		t.Fatalf("expected a version with a ^ prefix: %v", ver)
	}
	t.Logf("parsed react package version: %v", ver)
}
