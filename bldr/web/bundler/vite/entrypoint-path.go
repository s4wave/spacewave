package bldr_web_bundler_vite

import (
	"os"
	"path/filepath"
)

// ResolveViteEntrypointPath returns the vite bootstrap entrypoint relative to distSourcePath.
func ResolveViteEntrypointPath(distSourcePath string) string {
	candidates := []string{
		"web/bundler/vite/vite.ts",
		"bldr/web/bundler/vite/vite.ts",
	}
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(distSourcePath, p)); err == nil {
			return filepath.ToSlash(p)
		}
	}
	return candidates[0]
}
