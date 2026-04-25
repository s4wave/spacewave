package bldr_web_bundler_vite

import (
	"os"
	"path/filepath"
)

// ResolveViteEntrypointPath returns the vite bootstrap entrypoint relative to distSourcePath.
func ResolveViteEntrypointPath(distSourcePath string) string {
	return resolveExistingRelativePath(distSourcePath, []string{
		"web/bundler/vite/vite.ts",
		"bldr/web/bundler/vite/vite.ts",
	})
}

// ResolveViteBaseConfigPath returns the vite base config path relative to distSourcePath.
func ResolveViteBaseConfigPath(distSourcePath string) string {
	return resolveExistingRelativePath(distSourcePath, []string{
		"web/bundler/vite/vite-base.config.ts",
		"bldr/web/bundler/vite/vite-base.config.ts",
	})
}

func resolveExistingRelativePath(root string, candidates []string) string {
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			return filepath.ToSlash(p)
		}
	}
	return candidates[0]
}
