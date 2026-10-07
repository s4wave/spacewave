//go:build !js

package bldr_project_starlark

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
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

	// fsys is the project file system every load reads from.
	fsys fs.FS
	// moduleCache caches loaded modules by resolved path.
	moduleCache map[string]*moduleEntry
}

// moduleEntry caches the result of loading a module.
type moduleEntry struct {
	globals starlark.StringDict
	err     error
}

// Evaluate evaluates a .star file and returns the resulting ProjectConfig.
// The path is the filesystem path to the .star file. Loads resolve within the
// directory containing it, and LoadedFiles holds absolute paths.
func Evaluate(path string) (*Result, error) {
	// Confine every read to the directory containing the root file.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, "resolve starlark file path")
	}
	root, err := os.OpenRoot(filepath.Dir(absPath))
	if err != nil {
		return nil, errors.Wrap(err, "open project directory")
	}
	defer root.Close()

	// Evaluate the project and map its loaded files back to the host.
	result, err := EvaluateFS(context.Background(), root.FS(), filepath.Base(absPath))
	if err != nil {
		return nil, err
	}
	for i, name := range result.LoadedFiles {
		result.LoadedFiles[i] = filepath.Join(root.Name(), filepath.FromSlash(name))
	}
	return result, nil
}

// EvaluateFS evaluates the .star file name within fsys.
//
// Relative loads resolve within fsys and @go/ loads within its vendor
// directory, so evaluation reads only fsys and has no other effect.
// LoadedFiles holds slash paths relative to the root of fsys. Cancelling ctx
// stops the evaluation.
func EvaluateFS(ctx context.Context, fsys fs.FS, name string) (*Result, error) {
	// Read the root Starlark project source.
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, errors.Wrap(err, "read starlark file")
	}

	// Create the project evaluator and its module cache.
	eval := &evaluator{
		config:      &bldr_project.ProjectConfig{},
		loadedFiles: []string{name},
		fsys:        fsys,
		moduleCache: make(map[string]*moduleEntry),
	}

	// Expose project registration and configuration constructors to Starlark.
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

	// Create the Starlark thread with the evaluator module loader.
	thread := &starlark.Thread{
		Name: "bldr",
		Load: eval.load,
	}
	stop := context.AfterFunc(ctx, func() {
		thread.Cancel(context.Cause(ctx).Error())
	})
	defer stop()

	// Enable the supported Starlark syntax for the project source.
	opts := &syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	// Store predeclared as thread-local so load() can pass them to sub-modules.
	thread.SetLocal("predeclared", predeclared)

	// Evaluate the project source and retain its configuration changes.
	_, err = starlark.ExecFileOptions(opts, thread, name, data, predeclared)
	if err != nil {
		return nil, errors.Wrap(err, "evaluate starlark file")
	}

	return &Result{
		Config:      eval.config,
		LoadedFiles: eval.loadedFiles,
	}, nil
}
