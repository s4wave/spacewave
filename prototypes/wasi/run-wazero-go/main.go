package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	wazero_sys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// FSConfigWithSysFSMount extends FSConfig to expose the existing WithSysFSMount function.
// https://github.com/tetratelabs/wazero/issues/2076
type FSConfigWithSysFSMount interface {
	WithSysFSMount(fs wazero_sys.FS, guestPath string) wazero.FSConfig
}

// demoFS is an embedded filesystem
//
//go:embed main.go
var demoFS embed.FS

func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	fsConf := wazero.NewFSConfig()
	fsConf = fsConf.WithReadOnlyDirMount(".", "/")
	fsConf = fsConf.WithDirMount("/tmp", "/tmp")
	fsConf = fsConf.WithFSMount(demoFS, "/example")

	// NOTE: We can pass a wazero_sys.FS to enable a custom read/write fs.
	var writableFS wazero_sys.FS
	_ = writableFS
	// type assertion
	// fsConf.(FSConfigWithSysFSMount).WithSysFSMount(writableFS, "/")
	_ = fsConf.(FSConfigWithSysFSMount)

	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithFS(demoFS).
		WithFSConfig(fsConf)

	closeWasi, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	check(err)
	defer func() {
		_ = closeWasi.Close(ctx)
	}()

	demoWasm, err := os.ReadFile("../demo.wasm")
	check(err)

	// args are the command line os.Args
	args := config.WithArgs("demo.wasm")

	// This is a convenience utility that chains CompileModule with
	// InstantiateModule. To instantiate the same source multiple times, use
	// CompileModule as InstantiateModule avoids redundant decoding and/or
	// compilation.
	//
	// Note: Most compilers do not exit the module after running "_start",
	// unless there was an error. This allows you to call exported functions.
	if _, err = r.InstantiateWithConfig(ctx, demoWasm, args); err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			fmt.Fprintf(os.Stderr, "exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			check(err)
		}
	}
}

func check(err error) {
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
