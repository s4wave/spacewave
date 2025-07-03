# Concurrent I/O Demo (concio)

This demonstrates concurrent I/O operations in JavaScript running within a WebAssembly QuickJS environment using Go's wazero runtime.

## What it shows:

- **Concurrent timers**: Multiple `setInterval` timers running simultaneously at different intervals (1s and 2s)
- **Asynchronous stdin handling**: Non-blocking stdin reader that processes user input without blocking the timers
- **Event-driven programming**: Using handlers and callbacks for I/O operations in JavaScript
- **WASI integration**: Full system access (stdin/stdout/stderr) through WebAssembly System Interface

## How it works:

1. The Go program instantiates a QuickJS WebAssembly module using wazero
2. JavaScript code runs inside the WASM environment with access to system calls
3. Timers fire concurrently while stdin remains responsive
4. Type 'quit' to exit the program

This showcases how JavaScript can run in a sandboxed WebAssembly environment while still having controlled access to system resources.
