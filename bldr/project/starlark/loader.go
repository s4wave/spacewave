//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// goVendorPrefix is the prefix for vendored Go module imports.
const goVendorPrefix = "@go/"

// load implements the starlark-go load() callback.
// Resolves relative paths from the project directory.
// Resolves @go/ paths from the vendor/ directory.
func (e *evaluator) load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	// Resolve the Starlark module path before checking its cached result.
	resolved, err := e.resolveModulePath(thread, module)
	if err != nil {
		return nil, err
	}

	// Check cache.
	if entry, ok := e.moduleCache[resolved]; ok {
		return entry.globals, entry.err
	}

	// Read and execute the module.
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, errors.Wrapf(err, "load %q", module)
	}

	// Retain the loaded module path for evaluation readback.
	e.loadedFiles = append(e.loadedFiles, resolved)

	// Enable the supported Starlark syntax for loaded modules.
	opts := &syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	// Evaluate the loaded module and cache its globals and error.
	globals, err := starlark.ExecFileOptions(opts, thread, resolved, data, thread.Local("predeclared").(starlark.StringDict))
	e.moduleCache[resolved] = &moduleEntry{globals: globals, err: err}
	return globals, err
}

// resolveModulePath resolves a module string to an absolute filesystem path.
func (e *evaluator) resolveModulePath(thread *starlark.Thread, module string) (string, error) {
	// Resolve Go vendor imports beneath the configured vendor directory.
	if after, ok := strings.CutPrefix(module, goVendorPrefix); ok {
		// @go/github.com/foo/bar/file.star -> vendor/github.com/foo/bar/file.star
		return resolveModuleUnder(e.vendorDir, after, module)
	}

	// Relative path: resolve from the directory of the calling file,
	// or from the project directory if no caller frame is available.
	if filepath.IsAbs(module) {
		return "", errors.Errorf("load %q: absolute paths are not allowed", module)
	}
	baseDir := e.projectDir
	if depth := thread.CallStackDepth(); depth > 1 {
		callerFile := thread.CallFrame(1).Pos.Filename()
		if callerFile != "" {
			baseDir = filepath.Dir(callerFile)
		}
	}
	resolved, err := filepath.Abs(filepath.Join(baseDir, filepath.FromSlash(module)))
	if err != nil {
		return "", errors.Wrapf(err, "resolve load %q", module)
	}
	if !isPathWithin(e.projectDir, resolved) {
		return "", errors.Errorf("load %q: path escapes project root", module)
	}
	if ok, err := isExistingPathWithin(e.projectDir, resolved); err != nil {
		return "", errors.Wrapf(err, "resolve load %q", module)
	} else if !ok {
		return "", errors.Errorf("load %q: path escapes project root", module)
	}
	return resolved, nil
}

// resolveModuleUnder resolves a module path relative to a root directory,
// erroring when the path escapes the root.
func resolveModuleUnder(root, module, display string) (string, error) {
	// Resolve a relative module path and require it to remain beneath the vendor root.
	if module == "" {
		return "", errors.Errorf("load %q: empty module path", display)
	}
	if filepath.IsAbs(module) {
		return "", errors.Errorf("load %q: absolute paths are not allowed", display)
	}
	resolved, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(module)))
	if err != nil {
		return "", errors.Wrapf(err, "resolve load %q", display)
	}
	if !isPathWithin(root, resolved) {
		return "", errors.Errorf("load %q: path escapes vendor root", display)
	}
	if ok, err := isExistingPathWithin(root, resolved); err != nil {
		return "", errors.Wrapf(err, "resolve load %q", display)
	} else if !ok {
		return "", errors.Errorf("load %q: path escapes vendor root", display)
	}
	return resolved, nil
}

// isPathWithin reports whether path is inside root by lexical comparison.
// See isExistingPathWithin for the symlink-resolved check.
func isPathWithin(root, path string) bool {
	// Compare the module path against the absolute root directory.
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// isExistingPathWithin reports whether path is inside root after resolving
// symlinks on both paths.
func isExistingPathWithin(root, path string) (bool, error) {
	// Resolve root and module symlinks before checking containment.
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	pathReal, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	return isPathWithin(rootReal, pathReal), nil
}
