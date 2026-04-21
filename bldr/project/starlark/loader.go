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

	e.loadedFiles = append(e.loadedFiles, resolved)

	opts := &syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	globals, err := starlark.ExecFileOptions(opts, thread, resolved, data, thread.Local("predeclared").(starlark.StringDict))
	e.moduleCache[resolved] = &moduleEntry{globals: globals, err: err}
	return globals, err
}

// resolveModulePath resolves a module string to an absolute filesystem path.
func (e *evaluator) resolveModulePath(thread *starlark.Thread, module string) (string, error) {
	if after, ok := strings.CutPrefix(module, goVendorPrefix); ok {
		// @go/github.com/foo/bar/file.star -> vendor/github.com/foo/bar/file.star
		relPath := after
		resolved := filepath.Join(e.vendorDir, filepath.FromSlash(relPath))
		return resolved, nil
	}

	// Relative path: resolve from the directory of the calling file,
	// or from the project directory if no caller frame is available.
	baseDir := e.projectDir
	if depth := thread.CallStackDepth(); depth > 1 {
		callerFile := thread.CallFrame(1).Pos.Filename()
		if callerFile != "" {
			baseDir = filepath.Dir(callerFile)
		}
	}
	resolved := filepath.Join(baseDir, filepath.FromSlash(module))
	return filepath.Abs(resolved)
}
