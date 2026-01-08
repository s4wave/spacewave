# QuickJS Browser Support

This package enables running JS plugins (platform `"js"`) in browser SharedWorkers using QuickJS WASI.

## Architecture

```
Browser Tab
├── WebDocument
│   └── createWebWorker(workerType=QUICKJS)
│       └── SharedWorker: shw.mjs#s=...&t=quickjs
│           └── shared-worker.ts
│               └── plugin-host-quickjs.ts
│                   ├── QuickJS WASM (qjs-wasi.wasm)
│                   │   └── plugin-quickjs.esm.js (boot harness)
│                   │       └── plugin.mjs (user plugin)
│                   │           └── backendAPI (yamux over stdin/devout)
│                   └── yamux StreamConn (direction: 'outbound')
│                       └── WebRuntimeClient
│                           └── WebRuntime (Go WASM)
```

## How It Works

1. **WebQuickJSHost** (`plugin/host/web/web-quickjs.go`) handles plugins with platform `"js"`. When executing a plugin, it creates a WebWorker with `WorkerType: WEB_WORKER_TYPE_QUICKJS`.

2. **WebDocument** (`web/bldr/web-document.ts`) passes the worker type in the URL hash (`#s=<path>&t=quickjs`). All workers use the same `shw.mjs` entry point.

3. **shared-worker.ts** (`web/bldr/shared-worker.ts`) parses the URL hash and dispatches to the appropriate runner based on the `t` parameter.

4. **plugin-host-quickjs.ts** (`web/runtime/quickjs/plugin-host-quickjs.ts`) handles QuickJS plugins:
   - Fetches QuickJS WASM from `/b/qjs/qjs-wasi.wasm`
   - Fetches the boot harness from `/b/qjs/plugin-quickjs.esm.js`
   - Sets up a WASI environment with stdin/stdout/`/dev/out`
   - Runs the boot harness which imports and executes the plugin

5. **Boot Harness** (`plugin/host/wazero-quickjs/plugin-quickjs.ts`) runs inside QuickJS and:
   - Sets up yamux connection over stdin (input) and `/dev/out` (output)
   - Provides `backendAPI` and `abortSignal` to the plugin
   - Handles RPC via starpc over yamux

6. **WASI Shim** (from `quickjs-wasi-reactor` npm package) provides the WASI syscalls needed by QuickJS:
   - `poll_oneoff` with FD polling support for async stdin
   - `PollableStdin` for receiving yamux data from host
   - `DevOut` for sending yamux data to host

## Stream Flow

```
Plugin -> WebRuntime (plugin making RPC calls):
  1. Plugin calls backendAPI.openStream()
  2. Boot harness opens stream via runtimeConn (yamux inbound)
  3. Data written to /dev/out
  4. plugin-host-quickjs receives via devOutStream -> hostConn (yamux outbound)
  5. handleStreamCtr forwards to api.openStream()
  6. WebRuntime receives the RPC call

WebRuntime -> Plugin (WebRuntime calling plugin):
  1. WebRuntime opens stream
  2. api.handleStreamCtr in plugin-host-quickjs receives it
  3. Opens stream via hostConn.openStream()
  4. Data sent to QuickJS stdin
  5. Boot harness receives via runtimeConn
  6. Plugin's handler is invoked
```

## Files

- `web/bldr/shared-worker.ts` - Unified SharedWorker entry point
- `web/runtime/quickjs/plugin-host-quickjs.ts` - QuickJS runtime host
- `web/runtime/controller/controller.go` - `/b/qjs/` HTTP routes (serves QuickJS WASM and boot harness)
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
