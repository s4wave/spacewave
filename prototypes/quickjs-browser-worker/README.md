# QuickJS Browser Worker Prototype

This prototype demonstrates running QuickJS WASI (qjs-wasi.wasm) inside a browser WebWorker using `@bjorn3/browser_wasi_shim`.

## Goals

1. Load the same QuickJS WASM binary we use in Wazero
2. Run it in a browser WebWorker with WASI shim
3. Validate that stdio works (stdout/stderr)
4. Test that our existing polyfills and boot script can work

## Setup

```bash
# Install JS dependencies
yarn install

# Run the Go server (serves index.html, worker.js, and qjs-wasi.wasm)
go run main.go
```

Then open http://localhost:8090 in your browser.

## How it Works

1. `main.go` serves:
   - `/qjs-wasi.wasm` - The QuickJS WASI binary from `github.com/aperturerobotics/go-quickjs-wasi-reactor`
   - `/index.html` - The test page
   - `/worker.js` - The WebWorker that loads QuickJS
   - `/node_modules/` - The browser_wasi_shim package

2. `worker.js`:
   - Fetches `/qjs-wasi.wasm`
   - Sets up WASI environment with `@bjorn3/browser_wasi_shim`
   - Creates a virtual filesystem with a test script
   - Runs QuickJS with `--std` flag to execute the script

3. `index.html`:
   - Creates the WebWorker
   - Displays stdout/stderr output from QuickJS

## Next Steps

Once this basic prototype works:

1. Add stdin support for bidirectional communication
2. Add `/dev/out` pipe support (like our Wazero implementation)
3. Test with our existing `plugin-quickjs.ts` boot script
4. Integrate with SharedWorker infrastructure
