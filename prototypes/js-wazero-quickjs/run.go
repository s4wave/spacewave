package main

import (
	"context"
	"embed"
	"log"
	"os"

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

	// Load the Wasm binary from a file.
	mainWasm, err := os.ReadFile("qjs-wasi.wasm")
	if err != nil {
		log.Panicf("failed to read wasm file: %v", err)
	}

	// Instantiate the Wasm module.
	// This will automatically run the "_start" function of the module.
	mod, err := r.InstantiateWithConfig(ctx, mainWasm, config.WithArgs("qjs-wasi.wasm", "main.js"))
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
