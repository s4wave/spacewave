package main

import (
	"context"
	"log"
	"os"

	quickjswasi "github.com/paralin/go-quickjs-wasi"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Configure the module with stdin, stdout, and stderr.
	config := wazero.NewModuleConfig().
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithEnv("EXAMPLE", "example-value")

	// Instantiate WASI, which implements system call APIs.
	// This is required for the Wasm module to print to the console.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Instantiate the Wasm module with just the wasm filename as argument.
	// This will automatically run the "_start" function of the module.
	mod, err := r.InstantiateWithConfig(ctx, quickjswasi.QuickJSWASM, config.WithArgs(quickjswasi.QuickJSWASMFilename))
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
