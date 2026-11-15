package npm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/testbed/reporoot"
)

// TestLoadPackageVersion tests loading a package version from package.json.
func TestLoadPackageVersion(t *testing.T) {
	// Load the React dependency version from the repository's package.json.
	ver, err := LoadPackageVersion(filepath.Join(reporoot.Must(t), "package.json"), "react")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the React dependency retains its caret version constraint.
	if !strings.HasPrefix(ver, "^") {
		t.Fatalf("expected a version with a ^ prefix: %v", ver)
	}
	t.Logf("parsed react package version: %v", ver)
}
