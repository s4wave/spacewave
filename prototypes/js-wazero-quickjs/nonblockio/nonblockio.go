package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/aperturerobotics/bldr/util/wazerofs"
	quickjswasi "github.com/paralin/go-quickjs-wasi"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// jsScript stores the JavaScript code to run in the QuickJS VM
const jsScript = `
console.log("starting timer and stdin handler");

// Set up periodic timers
os.setInterval(() => console.log("Hello every 1s"), 1000);
os.setInterval(() => console.log("Hello every 2s"), 2000);

// Set up stdin read handler
const stdinFd = 0; // stdin file descriptor
const readBuffer = new Uint8Array(64);

function stdinReadHandler() {
	const bytesRead = os.read(stdinFd, readBuffer.buffer, 0, readBuffer.length);
	if (bytesRead > 0) {
		// Convert bytes to string manually
		let input = "";
		for (let i = 0; i < bytesRead; i++) {
			input += String.fromCharCode(readBuffer[i]);
		}
		console.log("got stdin:", JSON.stringify(input.trim()));
	}
}

// Register the read handler for stdin
os.setReadHandler(stdinFd, stdinReadHandler);
console.log("Listening to stdin with non-blocking I/O.");
`

func main() {
	// Choose the context to use for function calls.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Create the stdin buffer.
	stdinBuf := &wazerofs.StdinBuffer{}
	defer stdinBuf.Close()

	// Asynchronously write to the buffer.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond * 500):
			}

			_, _ = stdinBuf.Write([]byte("Hello at " + time.Now().String()))
		}
	}()

	// Configure the module with stdin, stdout, and stderr.
	// WithStdin will internally call:
	//  - stdinFileEntry => &FileEntry{File: &StdinFile{Reader: r}}
	//  - poll_oneoff -> stdin.File.Poll -> StdinFile.Poll -> f.Reader.(pollable).Poll
	config := wazero.NewModuleConfig().
		WithStdin(stdinBuf).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithSysNanosleep().
		WithSysNanotime().
		WithSysWalltime()

	// Instantiate WASI, which implements system call APIs.
	// This is required for the Wasm module to print to the console.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Instantiate the Wasm module with just the wasm filename as argument.
	// This will automatically run the "_start" function of the module.
	mod, err := r.InstantiateWithConfig(ctx, quickjswasi.QuickJSWASM, config.WithArgs(
		quickjswasi.QuickJSWASMFilename,
		"--std",
		"-e", jsScript,
	))
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
