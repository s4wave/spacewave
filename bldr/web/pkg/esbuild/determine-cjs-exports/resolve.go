package determine_cjs_exports

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
)

var resolveExtensions = []string{".cjs", ".js", ".json", ".node", ".es"}

type moduleResolver struct {
	manifests []string
}

// ResolveModule resolves a module import path to an absolute file path.
func ResolveModule(baseDir, importPath string) (string, error) {
	return new(moduleResolver).resolveModule(baseDir, importPath)
}

// ResolveModuleWithNodePaths resolves a module, trying extra node paths if needed.
func ResolveModuleWithNodePaths(baseDir, importPath string, nodePaths []string) (string, error) {
	resolved, _, err := ResolveModuleWithProvenance(baseDir, importPath, nodePaths)
	return resolved, err
}

// ResolveModuleWithProvenance resolves a module and reports package manifests consulted during resolution.
func ResolveModuleWithProvenance(baseDir, importPath string, nodePaths []string) (string, []string, error) {
	r := new(moduleResolver)
	resolved, err := r.resolveModule(baseDir, importPath)
	primaryErr := err
	if err != nil && !isRelativeOrAbsoluteImport(importPath) {
		for _, nodePath := range nodePaths {
			resolved, err = r.resolvePackageDir(filepath.Join(nodePath, importPath))
			if err == nil {
				break
			}
		}
		if err != nil {
			err = primaryErr
		}
	}
	for i, manifest := range r.manifests {
		if canonical, canonicalErr := filepath.EvalSymlinks(manifest); canonicalErr == nil {
			r.manifests[i] = canonical
		}
	}
	slices.Sort(r.manifests)
	return resolved, slices.Compact(r.manifests), err
}

func (r *moduleResolver) resolveModule(baseDir, importPath string) (string, error) {
	if filepath.IsAbs(importPath) {
		if hasExtension(importPath) {
			return importPath, nil
		}
		return r.resolveFile(importPath)
	}
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return r.resolveFile(filepath.Join(baseDir, importPath))
	}
	return r.resolveBarePath(baseDir, importPath)
}

func (r *moduleResolver) resolveFile(candidate string) (string, error) {
	if isFile(candidate) {
		return candidate, nil
	}
	for _, ext := range resolveExtensions {
		path := candidate + ext
		if isFile(path) {
			return path, nil
		}
	}
	return r.resolveIndex(candidate)
}

func (r *moduleResolver) resolveIndex(dir string) (string, error) {
	for _, ext := range resolveExtensions {
		path := filepath.Join(dir, "index"+ext)
		if isFile(path) {
			return path, nil
		}
	}
	return "", &ModuleNotFoundError{Path: dir}
}

func (r *moduleResolver) resolveBarePath(baseDir, importPath string) (string, error) {
	for dir := baseDir; ; dir = filepath.Dir(dir) {
		nodeModules := filepath.Join(dir, "node_modules")
		if isDir(nodeModules) {
			resolved, err := r.resolvePackageDir(filepath.Join(nodeModules, importPath))
			if err == nil {
				return resolved, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", &ModuleNotFoundError{Path: importPath}
}

func (r *moduleResolver) resolvePackageDir(packageDir string) (string, error) {
	if isFile(packageDir) {
		return packageDir, nil
	}
	for _, ext := range resolveExtensions {
		path := packageDir + ext
		if isFile(path) {
			return path, nil
		}
	}

	packageJSON := filepath.Join(packageDir, "package.json")
	if isFile(packageJSON) {
		r.manifests = append(r.manifests, packageJSON)
		main, err := readPackageMain(packageJSON)
		if err == nil && main != "" {
			if resolved, resolveErr := r.resolveFile(filepath.Join(packageDir, main)); resolveErr == nil {
				return resolved, nil
			}
		}
	}
	return r.resolveIndex(packageDir)
}

func isRelativeOrAbsoluteImport(importPath string) bool {
	return filepath.IsAbs(importPath) || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../")
}

func readPackageMain(path string) (string, error) {
	// #nosec G703 -- path is resolved from the package graph under analysis, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return "", err
	}
	return string(value.GetStringBytes("main")), nil
}

func hasExtension(path string) bool {
	return slices.Contains(resolveExtensions, filepath.Ext(path))
}

func isFile(path string) bool {
	// #nosec G703 -- path is resolved from the package graph under analysis, not user input.
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ModuleNotFoundError reports the unresolved import path.
type ModuleNotFoundError struct {
	Path string
}

// Error returns the unresolved-module message.
func (e *ModuleNotFoundError) Error() string {
	return "cannot resolve module: " + e.Path
}
