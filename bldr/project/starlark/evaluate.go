//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"

	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Result contains the result of evaluating a .star file.
type Result struct {
	// Config is the project config produced by the evaluation.
	Config *bldr_project.ProjectConfig
	// LoadedFiles is the list of all files loaded during evaluation.
	// Includes the root .star file and any files loaded via load().
	LoadedFiles []string
}

// evaluator holds mutable state during .star file evaluation.
type evaluator struct {
	config      *bldr_project.ProjectConfig
	loadedFiles []string

	// projectDir is the directory containing the root .star file.
	projectDir string
	// vendorDir is the vendor/ directory for @go/ imports.
	vendorDir string
	// moduleCache caches loaded modules by resolved path.
	moduleCache map[string]*moduleEntry
}

// moduleEntry caches the result of loading a module.
type moduleEntry struct {
	globals starlark.StringDict
	err     error
}

// Evaluate evaluates a .star file and returns the resulting ProjectConfig.
// The path is the filesystem path to the .star file.
func Evaluate(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "read starlark file")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, "resolve starlark file path")
	}
	projectDir := filepath.Dir(absPath)

	eval := &evaluator{
		config:      &bldr_project.ProjectConfig{},
		loadedFiles: []string{absPath},
		projectDir:  projectDir,
		vendorDir:   filepath.Join(projectDir, "vendor"),
		moduleCache: make(map[string]*moduleEntry),
	}

	predeclared := starlark.StringDict{
		// Registration built-ins (mutate config)
		"project":  starlark.NewBuiltin("project", eval.projectBuiltin),
		"manifest": starlark.NewBuiltin("manifest", eval.manifestBuiltin),
		"build":    starlark.NewBuiltin("build", eval.buildBuiltin),
		"remote":   starlark.NewBuiltin("remote", eval.remoteBuiltin),
		"publish":  starlark.NewBuiltin("publish", eval.publishBuiltin),

		// Convenience constructors (return dicts)
		"config_entry": starlark.NewBuiltin("config_entry", configEntryBuiltin),
		"start_config": starlark.NewBuiltin("start_config", startConfigBuiltin),
		"web_pkg":      starlark.NewBuiltin("web_pkg", webPkgBuiltin),
		"js_module":    starlark.NewBuiltin("js_module", jsModuleBuiltin),

		// Typed per-builder constructors (return dicts with field validation)
		"go_plugin_config":           starlark.NewBuiltin("go_plugin_config", goPluginConfigBuiltin),
		"js_plugin_config":           starlark.NewBuiltin("js_plugin_config", jsPluginConfigBuiltin),
		"cli_compiler_config":        starlark.NewBuiltin("cli_compiler_config", cliCompilerConfigBuiltin),
		"dist_compiler_config":       starlark.NewBuiltin("dist_compiler_config", distCompilerConfigBuiltin),
		"web_plugin_compiler_config": starlark.NewBuiltin("web_plugin_compiler_config", webPluginCompilerConfigBuiltin),
	}

	thread := &starlark.Thread{
		Name: "bldr",
		Load: eval.load,
	}

	opts := &syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	// Store predeclared as thread-local so load() can pass them to sub-modules.
	thread.SetLocal("predeclared", predeclared)

	_, err = starlark.ExecFileOptions(opts, thread, absPath, data, predeclared)
	if err != nil {
		return nil, errors.Wrap(err, "evaluate starlark file")
	}

	return &Result{
		Config:      eval.config,
		LoadedFiles: eval.loadedFiles,
	}, nil
}
