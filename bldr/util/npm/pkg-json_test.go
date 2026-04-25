package npm

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLoadPackageVersion tests loading a package version from package.json.
func TestLoadPackageVersion(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	ver, err := LoadPackageVersion(filepath.Clean(filepath.Join(filepath.Dir(file), "../../../package.json")), "react")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !strings.HasPrefix(ver, "^") {
		t.Fatalf("expected a version with a ^ prefix: %v", ver)
	}
	t.Logf("parsed react package version: %v", ver)
}
