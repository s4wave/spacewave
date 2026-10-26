//go:build !goscript

package plugin_host_wazero_quickjs

import (
	"context"
	"io/fs"
	"path"
	"strings"
	"time"

	quickjs "github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs"
	bldr_util_qjsbytecode "github.com/s4wave/spacewave/bldr/util/qjsbytecode"
	"github.com/sirupsen/logrus"
)

// bytecodeCache stores the bytecode of plugin modules compiled by the QuickJS
// reactor, so a plugin start links its modules without parsing their source.
// Each plugin owns a directory of the cache.
type bytecodeCache struct {
	// cache is the module bytecode cache.
	cache *bldr_util_qjsbytecode.Cache
}

// newBytecodeCache returns a plugin bytecode cache rooted at dir.
func newBytecodeCache(dir string) *bytecodeCache {
	return &bytecodeCache{cache: bldr_util_qjsbytecode.NewCache(dir)}
}

// loadModules registers each ES module in the plugin dist with the QuickJS
// instance, from cached bytecode or by compiling and caching it. It then
// removes the plugin's cached modules that the dist no longer contains.
//
// A module that does not compile is left for the QuickJS module loader to
// report if the plugin imports it.
func (c *bytecodeCache) loadModules(ctx context.Context, le *logrus.Entry, qjs *quickjs.QuickJS, pluginID string, dist fs.FS) error {
	// Register every module in the dist.
	start := time.Now()
	used := make(map[string]struct{})
	var hits, misses int
	err := fs.WalkDir(dist, ".", func(distPath string, d fs.DirEntry, err error) error {
		// Read the module source.
		if err != nil || d.IsDir() || !isModulePath(distPath) {
			return err
		}
		src, err := fs.ReadFile(dist, distPath)
		if err != nil {
			return err
		}

		// Link the cached bytecode or compile and cache it.
		name := path.Join(DistFsMount, distPath)
		file, hit, err := c.cache.LoadModule(ctx, le, qjs, pluginID, name, src)
		used[file] = struct{}{}
		if hit {
			hits++
		} else {
			misses++
		}
		if err != nil {
			le.WithError(err).WithField("module", name).Warn("cannot compile quickjs module bytecode")
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove cached modules this dist does not use.
	if err := c.cache.Prune(pluginID, used); err != nil {
		return err
	}

	// Report the cache use and the time to register the modules.
	le.WithFields(logrus.Fields{
		"hits":     hits,
		"misses":   misses,
		"duration": time.Since(start).String(),
	}).Debug("loaded quickjs module bytecode")
	return nil
}

// deletePlugin removes the cached modules of a plugin.
func (c *bytecodeCache) deletePlugin(pluginID string) error {
	return c.cache.DeleteOwner(pluginID)
}

// isModulePath reports whether a dist path names an ES module.
func isModulePath(p string) bool {
	return strings.HasSuffix(p, ".mjs") || strings.HasSuffix(p, ".js")
}
