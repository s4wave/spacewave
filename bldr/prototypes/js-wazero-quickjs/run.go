package main

import (
	"context"
	"embed"
	"log"
	"os"

	quickjs_wasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// Embed the script to expose to quickjs within wasi.
//
//go:embed main.js
var ScriptFS embed.FS

func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Combine the above into our baseline config, overriding defaults.
	// By default, I/O streams are discarded and there's no file system.
	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithFS(ScriptFS)

	// Instantiate WASI, which implements system call APIs.
	// This is required for the Wasm module to print to the console.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Instantiate the Wasm module.
	// This will automatically run the "_start" function of the module.
	mod, err := r.InstantiateWithConfig(ctx, quickjs_wasi.QuickJSWASM, config.WithArgs(quickjs_wasi.QuickJSWASMFilename, "main.js"))
	if err != nil {
		// Note: Most compilers do not exit the module after running "_start",
		// unless there was an error. This allows you to call exported functions.
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			log.Panicf("exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			log.Panicln(err)
		}
	}
	_ = mod
}
