package bldr

import (
	"os"
	"path/filepath"
)

// ResolveDistSourcePath resolves a dist source file from either the repo root or the bldr subdir.
func ResolveDistSourcePath(distSourcePath string, elems ...string) string {
	candidates := [][]string{
		elems,
		append([]string{"bldr"}, elems...),
	}
	for _, c := range candidates {
		p := filepath.Join(append([]string{distSourcePath}, c...)...)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(append([]string{distSourcePath}, elems...)...)
}
