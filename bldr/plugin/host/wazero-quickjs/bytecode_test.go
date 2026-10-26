//go:build !goscript

package plugin_host_wazero_quickjs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	quickjs "github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs"
	bldr_util_qjsbytecode "github.com/s4wave/spacewave/bldr/util/qjsbytecode"
	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"
)

// TestBytecodeCache imports a dist module in two QuickJS instances: the first
// compiles and caches it, and the second links it from the cache. Neither
// mounts the dist, so an import resolves only to a registered module.
func TestBytecodeCache(t *testing.T) {
	// Build the cache and a dist with a module that imports a chunk.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	cacheDir := t.TempDir()
	cache := newBytecodeCache(cacheDir)
	dist := fstest.MapFS{
		"main.mjs":       {Data: []byte("import { answer } from './chunks/lib.mjs';\nprint(answer);\n")},
		"chunks/lib.mjs": {Data: []byte("export const answer = 6 * 7;\n")},
		"dev-info.bin":   {},
	}

	// run loads the dist modules into a fresh instance and imports main.mjs.
	run := func() string {
		// Start an instance that prints to stdout.
		var stdout bytes.Buffer
		r := wazero.NewRuntime(ctx)
		defer r.Close(ctx)
		config := wazero.NewModuleConfig().WithStdout(&stdout).WithStderr(&stdout)
		qjs, err := quickjs.NewQuickJS(ctx, r, config)
		if err != nil {
			t.Fatal(err)
		}
		defer qjs.Close(ctx)
		if err := qjs.Init(ctx, []string{"qjs", "--std"}); err != nil {
			t.Fatal(err)
		}

		// Register the dist modules and import the entry.
		if err := cache.loadModules(ctx, le, qjs, "test-plugin", dist); err != nil {
			t.Fatal(err)
		}
		if err := qjs.Eval(ctx, "import '/dist/main.mjs';", true); err != nil {
			t.Fatalf("%v\n%s", err, stdout.String())
		}
		if err := qjs.RunLoop(ctx); err != nil {
			t.Fatal(err)
		}
		return stdout.String()
	}

	// Compile on the first run, then link the cached bytecode.
	for i := range 2 {
		if got := run(); got != "42\n" {
			t.Fatalf("run %d printed %q", i, got)
		}
	}

	// Each module has one cache entry, and a stale entry is removed.
	pluginDir := filepath.Join(cacheDir, "test-plugin")
	stale := filepath.Join(pluginDir, "stale"+bldr_util_qjsbytecode.Ext)
	if err := os.WriteFile(stale, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	run()
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("cache has %d entries, want 2", len(entries))
	}
}
