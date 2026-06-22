package quickjs_test

import (
	"context"
	"embed"
	"os"
	"testing"

	quickjswasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

//go:embed *.js *.ts
var testFS embed.FS

// runQuickJSScript runs the embedded script inside the QuickJS WASM engine and
// fails the test if it throws or exits non-zero.
func runQuickJSScript(t *testing.T, scriptName string) {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	config := wazero.NewModuleConfig().
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithFS(testFS)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	args := []string{quickjswasi.QuickJSWASMFilename, "--std", scriptName}
	_, err := r.InstantiateWithConfig(ctx, quickjswasi.QuickJSWASM, config.WithArgs(args...))
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			if exitErr.ExitCode() != 0 {
				t.Fatalf("QuickJS exited with non-zero code: %d", exitErr.ExitCode())
			}
		} else {
			t.Fatalf("Failed to instantiate module: %v", err)
		}
	}
}

// TestEventTargetPolyfill tests the EventTarget polyfill implementation.
func TestEventTargetPolyfill(t *testing.T) {
	runQuickJSScript(t, "polyfill_test.js")
	t.Log("Successfully tested EventTarget polyfills")
}

// TestReadableStreamPolyfill tests the ReadableStream polyfill implementation.
func TestReadableStreamPolyfill(t *testing.T) {
	runQuickJSScript(t, "readable-stream_test.js")
	t.Log("Successfully tested ReadableStream polyfill")
}
