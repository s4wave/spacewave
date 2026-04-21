package determine_cjs_exports

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
)

// resolveExtensions is the ordered list of extensions to try when resolving.
var resolveExtensions = []string{".cjs", ".js", ".json", ".node", ".es"}

// ResolveModule resolves a module import path to an absolute file path.
// baseDir is the directory to resolve from (for relative imports).
// importPath is the import specifier (e.g., "./lib", "react", "/abs/path.js").
func ResolveModule(baseDir, importPath string) (string, error) {
	// Absolute path with supported extension: return directly.
	if strings.HasPrefix(importPath, "/") {
		if hasExtension(importPath) {
			return importPath, nil
		}
		return resolveFile(importPath)
	}

	// Relative path: resolve relative to baseDir.
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		abs := filepath.Join(baseDir, importPath)
		return resolveFile(abs)
	}

	// Bare specifier: walk up looking for node_modules.
	return resolveBarePath(baseDir, importPath)
}

// ResolveModuleWithNodePaths resolves a module, trying extra node paths if needed.
func ResolveModuleWithNodePaths(baseDir, importPath string, nodePaths []string) (string, error) {
	resolved, err := ResolveModule(baseDir, importPath)
	if err == nil {
		return resolved, nil
	}

	// Try each extra node path as a base directory for bare specifiers.
	if !strings.HasPrefix(importPath, "./") && !strings.HasPrefix(importPath, "../") && !strings.HasPrefix(importPath, "/") {
		for _, np := range nodePaths {
			// nodePaths entries are node_modules directories themselves.
			pkgDir := filepath.Join(np, importPath)
			resolved, tryErr := resolvePackageDir(pkgDir)
			if tryErr == nil {
				return resolved, nil
			}
		}
	}

	return "", err
}

// resolveFile tries to resolve a path as a file or directory.
func resolveFile(abs string) (string, error) {
	// Try exact path.
	if isFile(abs) {
		return abs, nil
	}

	// Try with each extension.
	for _, ext := range resolveExtensions {
		p := abs + ext
		if isFile(p) {
			return p, nil
		}
	}

	// Try as directory with index file.
	return resolveIndex(abs)
}

// resolveIndex tries to resolve a directory by looking for index files.
func resolveIndex(dir string) (string, error) {
	for _, ext := range resolveExtensions {
		p := filepath.Join(dir, "index"+ext)
		if isFile(p) {
			return p, nil
		}
	}
	return "", &ModuleNotFoundError{Path: dir}
}

// resolveBarePath resolves a bare module specifier by walking up node_modules.
func resolveBarePath(baseDir, importPath string) (string, error) {
	dir := baseDir
	for {
		nmDir := filepath.Join(dir, "node_modules")
		if isDir(nmDir) {
			pkgDir := filepath.Join(nmDir, importPath)
			resolved, err := resolvePackageDir(pkgDir)
			if err == nil {
				return resolved, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", &ModuleNotFoundError{Path: importPath}
}

// resolvePackageDir resolves a package directory using package.json main field.
func resolvePackageDir(pkgDir string) (string, error) {
	// If pkgDir is actually a file (or resolvable as one), use it directly.
	if isFile(pkgDir) {
		return pkgDir, nil
	}
	for _, ext := range resolveExtensions {
		p := pkgDir + ext
		if isFile(p) {
			return p, nil
		}
	}

	// Read package.json for main field.
	pkgJSON := filepath.Join(pkgDir, "package.json")
	if isFile(pkgJSON) {
		main, err := readPackageMain(pkgJSON)
		if err == nil && main != "" {
			resolved, err := resolveFile(filepath.Join(pkgDir, main))
			if err == nil {
				return resolved, nil
			}
		}
	}

	// Fall back to index files.
	return resolveIndex(pkgDir)
}

// readPackageMain reads the "main" field from a package.json file.
func readPackageMain(path string) (string, error) {
	// #nosec G703 -- path is resolved from the package graph under analysis, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return "", err
	}
	return string(v.GetStringBytes("main")), nil
}

// hasExtension checks if the path has one of the supported extensions.
func hasExtension(p string) bool {
	ext := filepath.Ext(p)
	return slices.Contains(resolveExtensions, ext)
}

// isFile checks if the path is a regular file.
func isFile(p string) bool {
	// #nosec G703 -- p is resolved from the package graph under analysis, not user input.
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// isDir checks if the path is a directory.
func isDir(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ModuleNotFoundError is returned when a module cannot be resolved.
type ModuleNotFoundError struct {
	Path string
}

// Error returns the error message.
func (e *ModuleNotFoundError) Error() string {
	return "cannot resolve module: " + e.Path
}
