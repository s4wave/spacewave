package quickjs_test

import (
	"context"
	"embed"
	"os"
	"testing"

	quickjswasi "github.com/paralin/go-quickjs-wasi"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

//go:embed *.js *.ts
var testFS embed.FS

// TestEventTargetPolyfill tests the EventTarget polyfill implementation.
func TestEventTargetPolyfill(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	config := wazero.NewModuleConfig().
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithFS(testFS)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	args := []string{quickjswasi.QuickJSWASMFilename, "--std", "polyfill_test.js"}
	mod, err := r.InstantiateWithConfig(ctx, quickjswasi.QuickJSWASM, config.WithArgs(args...))
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			if exitErr.ExitCode() != 0 {
				t.Fatalf("QuickJS exited with non-zero code: %d", exitErr.ExitCode())
			}
		} else {
			t.Fatalf("Failed to instantiate module: %v", err)
		}
	}
	_ = mod

	t.Log("Successfully tested EventTarget polyfills")
}
