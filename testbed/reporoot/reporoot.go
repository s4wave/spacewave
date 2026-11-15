// Package reporoot finds the Spacewave module root from the working directory.
//
// go test runs each test binary in its package directory, so a test or the
// harness it calls finds the checkout from there. A source path from
// runtime.Caller does not serve: under -trimpath it is a module path, not a
// file path.
package reporoot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
)

// Find returns the nearest ancestor of the working directory, itself
// included, that holds go.mod.
func Find() (string, error) {
	// Start at the working directory, the package directory under go test.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Stop at the first ancestor that holds go.mod.
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above the working directory")
		}
		dir = parent
	}
}

// Must returns Find's directory and fails tb when there is none.
func Must(tb testing.TB) string {
	tb.Helper()
	dir, err := Find()
	if err != nil {
		tb.Fatal(err)
	}
	return dir
}
