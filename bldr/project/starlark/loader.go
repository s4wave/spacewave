//go:build !js

package bldr_project_starlark

import (
	"io/fs"
	"path"
	"strings"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// goVendorPrefix is the prefix for vendored Go module imports.
const goVendorPrefix = "@go/"

// goVendorDir is the project directory holding vendored Go modules.
const goVendorDir = "vendor"

// load implements the starlark-go load() callback.
// Resolves relative paths from the calling file's directory.
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
	data, err := fs.ReadFile(e.fsys, resolved)
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

// resolveModulePath resolves a module string to a slash path in the project
// file system.
func (e *evaluator) resolveModulePath(thread *starlark.Thread, module string) (string, error) {
	// Resolve Go vendor imports beneath the vendor directory.
	if after, ok := strings.CutPrefix(module, goVendorPrefix); ok {
		// @go/github.com/foo/bar/file.star -> vendor/github.com/foo/bar/file.star
		resolved, ok := joinWithin(goVendorDir, goVendorDir, after)
		if !ok {
			return "", errors.Errorf("load %q: path escapes vendor root", module)
		}
		return resolved, nil
	}

	// Resolve other modules from the directory of the calling file.
	baseDir := "."
	if thread.CallStackDepth() > 1 {
		baseDir = path.Dir(thread.CallFrame(1).Pos.Filename())
	}
	resolved, ok := joinWithin(".", baseDir, module)
	if !ok {
		return "", errors.Errorf("load %q: path escapes project root", module)
	}
	return resolved, nil
}

// joinWithin joins the relative module path to dir and reports whether the
// result stays within root. All paths are slash paths relative to the
// project file system.
func joinWithin(root, dir, module string) (string, bool) {
	// Reject empty and absolute module paths before joining.
	if module == "" || path.IsAbs(module) {
		return "", false
	}

	// Require the cleaned result to name a file beneath root.
	resolved := path.Join(dir, module)
	if !fs.ValidPath(resolved) || resolved == "." {
		return "", false
	}
	return resolved, root == "." || strings.HasPrefix(resolved, root+"/")
}
