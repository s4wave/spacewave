package plugin_compiler

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"time"

	debounce_fswatcher "github.com/aperturerobotics/controllerbus/util/debounce-fswatcher"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// Watcher watches a set of packages and re-generates an output plugin codegen
// and binary when the code files change.
type Watcher struct {
	le                *logrus.Entry
	packageLookupPath string
	packagePaths      []string
}

// NewWatcher constructs a new watcher.
//
// Recognizes and replaces {buildHash} in the output filename.
// The output path should be output-plugin-dir/output-plugin-{buildHash}.cbus.so
func NewWatcher(le *logrus.Entry, packageLookupPath string, packagePaths []string) *Watcher {
	return &Watcher{
		le:                le,
		packagePaths:      packagePaths,
		packageLookupPath: packageLookupPath,
	}
}

// WatchCompilePlugin watches and compiles package.
// Detects if the output with the same {buildHash} already exists.
// Replaces {buildHash} in output filename and in plugin binary version.
func (w *Watcher) WatchCompilePlugin(
	ctx context.Context,
	pluginCodegenPath string,
	pluginOutputPath string,
	pluginBinaryID string,
	compiledCb func(packages []string, outpPath string) error,
) error {
	le := w.le

	le.
		WithField("codegen-path", pluginCodegenPath).
		Info("hot: starting to build/watch plugin")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// passOutputPath may or may not contain {buildHash}
	compilePluginOnce := func(
		ctx context.Context,
		an *Analysis,
		passOutputPath string,
	) error {
		moduleCompiler, err := NewModuleCompiler(
			ctx,
			w.le,
			pluginCodegenPath,
			pluginBinaryID,
		)
		if err != nil {
			return err
		}
		if err := moduleCompiler.GenerateModule(an); err != nil {
			return err
		}
		return moduleCompiler.CompilePlugin(passOutputPath)
	}

	compilePlugin := func() (*Analysis, error) {
		rctx := ctx
		ctx, compileCtxCancel := context.WithCancel(rctx)
		defer compileCtxCancel()

		le.
			WithField("plugin-output-filename", path.Base(pluginOutputPath)).
			Debugf("analyzing packages: %v", w.packagePaths)
		an, err := AnalyzePackages(ctx, w.le, w.packageLookupPath, w.packagePaths)
		if err != nil {
			return nil, err
		}

		// pass 1: codegen + build with static build prefix.
		passBinDir := filepath.Join(pluginCodegenPath, "bin")
		if err := os.MkdirAll(passBinDir, 0755); err != nil {
			return nil, err
		}

		targetOutputPath := filepath.Join(passBinDir, "entrypoint")
		if err := compilePluginOnce(ctx, an, targetOutputPath); err != nil {
			return nil, err
		}

		if err == nil && compiledCb != nil {
			err = compiledCb(w.packagePaths, targetOutputPath)
		}

		return an, err
	}

	watchedFiles := make(map[string]struct{})
	for {
		an, err := compilePlugin()
		if err != nil {
			return err
		}

		// build file watchlist
		codefileMap := an.GetProgramCodeFiles(w.packagePaths, "")
		nextWatchedFiles := make(map[string]struct{})
		for _, filePaths := range codefileMap {
			for _, filePath := range filePaths {
				nextWatchedFiles[filePath] = struct{}{}
			}
		}
		for filePath := range watchedFiles {
			if _, ok := nextWatchedFiles[filePath]; ok {
				delete(nextWatchedFiles, filePath)
				continue
			}
			le.Debugf("removing watcher for file: %s", filePath)
			if err := watcher.Remove(filePath); err != nil {
				return err
			}
		}
		for filePath := range nextWatchedFiles {
			le.Debugf("adding watcher for file: %s", filePath)
			watchedFiles[filePath] = struct{}{}
			if err := watcher.Add(filePath); err != nil {
				return err
			}
		}

		le.Debugf(
			"hot: watching %d packages with %d files",
			len(w.packagePaths),
			len(watchedFiles),
		)

		// wait for a file change
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			ctx,
			watcher,
			time.Second,
		)
		if err != nil {
			return err
		}
		le.Infof("re-analyzing packages after %d filesystem events", len(happened))
	}
}
