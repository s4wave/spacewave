# QuickJS Browser Support

This package enables running JS plugins (platform `"js"`) in browser SharedWorkers using QuickJS WASI.

## Architecture

```
Browser Tab
├── WebDocument
│   └── createWebWorker(workerType=QUICKJS)
│       └── SharedWorker: shw-quickjs.mjs
│           ├── QuickJS WASM (qjs-wasi.wasm)
│           │   └── plugin-quickjs.esm.js (boot harness)
│           │       └── plugin.mjs (user plugin)
│           │           └── backendAPI (yamux over stdin/devout)
│           └── yamux StreamConn (direction: 'outbound')
│               └── WebRuntimeClient
│                   └── WebRuntime (Go WASM)
```

## How It Works

1. **WebQuickJSHost** (`plugin/host/web/web-quickjs.go`) handles plugins with platform `"js"`. When executing a plugin, it creates a WebWorker with `WorkerType: WEB_WORKER_TYPE_QUICKJS`.

2. **WebDocument** (`web/bldr/web-document.ts`) routes QUICKJS workers to `shw-quickjs.mjs` instead of the normal `shw.mjs`.

3. **shw-quickjs.ts** (`web/bldr/shw-quickjs.ts`) is the SharedWorker entry point that:
   - Fetches QuickJS WASM from `/b/qjs/qjs-wasi.wasm`
   - Fetches the boot harness from `/b/qjs/plugin-quickjs.esm.js`
   - Sets up a WASI environment with stdin/stdout/`/dev/out`
   - Runs the boot harness which imports and executes the plugin

4. **Boot Harness** (`plugin/host/wazero-quickjs/plugin-quickjs.ts`) runs inside QuickJS and:
   - Sets up yamux connection over stdin (input) and `/dev/out` (output)
   - Provides `backendAPI` and `abortSignal` to the plugin
   - Handles RPC via starpc over yamux

5. **WASI Shim** (`web/wasi-shim/`) provides the WASI syscalls needed by QuickJS:
   - `poll_oneoff` with FD polling support for async stdin
   - `PollableStdin` for receiving yamux data from host
   - `DevOut` for sending yamux data to host

## Stream Flow

```
Plugin → WebRuntime (plugin making RPC calls):
  1. Plugin calls backendAPI.openStream()
  2. Boot harness opens stream via runtimeConn (yamux inbound)
  3. Data written to /dev/out
  4. shw-quickjs receives via devOutStream → hostConn (yamux outbound)
  5. handleStreamFromPlugin forwards to webRuntimeClient.openStream()
  6. WebRuntime receives the RPC call

WebRuntime → Plugin (WebRuntime calling plugin):
  1. WebRuntime opens stream
  2. handleIncomingStream in shw-quickjs receives it
  3. handleStreamToPlugin opens stream via hostConn.openStream()
  4. Data sent to QuickJS stdin
  5. Boot harness receives via runtimeConn
  6. Plugin's handler is invoked
```

## Files

- `web/bldr/shw-quickjs.ts` - SharedWorker entry point
- `web/wasi-shim/` - Custom WASI shim with fd polling
- `web/quickjs/http/quickjs.go` - Serves QuickJS WASM and boot harness
- `web/runtime/controller/controller.go` - `/b/qjs/` HTTP routes
- `web/document/document.proto` - `WebWorkerType` enum
- `plugin/host/web/web-quickjs.go` - WebQuickJSHost controller
- `plugin/host/wazero-quickjs/plugin-quickjs.ts` - Boot harness

## Testing

```bash
# Run all QuickJS browser tests (11 tests)
cd bldr
bun run vitest run --project browser prototypes/quickjs-browser-worker/
```

Tests include:
- Basic QuickJS WASM execution
- stdin/stdout/`/dev/out` I/O
- Async stdin with `os.setReadHandler`
- Boot harness integration
- E2E yamux connection verification
- startInfo passing to plugins
