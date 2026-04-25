package projectroot

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

var rootMarkers = [...]string{
	"bldr.star",
	"bldr.yaml",
}

// FindFromCwd walks up from the current working directory to find the project root.
func FindFromCwd(maxWalkDepth int) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return FindFromDir(dir, maxWalkDepth)
}

// FindFromDir walks up from dir to find the nearest project root marker.
func FindFromDir(dir string, maxWalkDepth int) (string, error) {
	startDir := dir
	for range maxWalkDepth {
		if hasRootMarker(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.Errorf(
		"project root not found (searched %d levels up from %s for %s)",
		maxWalkDepth,
		startDir,
		strings.Join(rootMarkers[:], ", "),
	)
}

func hasRootMarker(dir string) bool {
	for _, marker := range rootMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}
