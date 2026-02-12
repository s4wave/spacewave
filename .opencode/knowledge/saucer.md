# Saucer: Native Webview Runtime

## Repository Layout

The saucer implementation spans multiple sibling repositories:

| Repository      | Path               | Purpose                                                      |
|-----------------|--------------------|--------------------------------------------------------------|
| **bldr**        | `./`               | Go controller, TS client, build system                       |
| **bldr-saucer** | `../bldr-saucer/`  | C++ webview process (main.cpp, scheme handling, pipe client) |
| **cpp-yamux**   | `../cpp-yamux/`    | C++ yamux stream multiplexer                                 |
| **saucer**      | `../saucer/`       | C++ webview library (WebKit/WebView2)                        |
| **starpc**      | (vendored)                               | C++ SRPC RPC framework                                       |
|                 |                                          |                                                              |

## Key Source Locations

### C++ Process (bldr-saucer)

Source: `../bldr-saucer/src/`

- `main.cpp` - Entry point; creates saucer app, registers bldr:// scheme, runs accept_thread for Go-initiated streams
- `pipe_client.h/cpp` - Bridges HTTP scheme requests to yamux streams
- `fetch_proto.h/cpp` - Fetches assets from Go ServiceWorkerHost
- `scheme_forwarder.h/cpp` - Implements bldr:// scheme handling
- `pipe_connection.h` - Yamux Connection over Unix domain sockets
- `CMakeLists.txt` - Build system (CPM for deps)

Vendored in bldr at: `vendor/github.com/aperturerobotics/bldr-saucer/`

### Go Controller (bldr)

Source: `web/plugin/saucer/`

- `controller.go` - Saucer runtime controller; starts C++ process, sets up yamux pipe. The `execListener` goroutine blocks on `ctx.Done()` after `AcceptStreams` returns `io.EOF`.
- `document-manager.go` - DocumentManager: manages stream state per document, bridges JS HTTP streams to yamux, handles control channel
- `saucer.go` - `RunSaucer()`: creates pipes, starts subprocess, passes config via env vars
- `factory.go` - Controller factory for controllerbus
- `config.go` - Config validation
- `saucer.proto` - Protobuf definitions (includes SaucerDebugService with EvalJS RPC)
- `saucer_srpc.pb.go` - Generated SRPC code for SaucerDebugService
- `debug-bridge.go` - Debug bridge: listens on Unix socket, forwards EvalJS RPCs to C++ accept loop

### TypeScript Client (bldr)

Source: `web/saucer/`

- `saucer.ts` - `SaucerRuntimeClient`: HTTP-based stream transport for saucer mode
- `index.ts` - Re-exports `isSaucer`, `getDocId`, `SaucerRuntimeClient`

### Runtime Support (bldr)

- `web/runtime/remote.go` - Remote runtime management, `monitorWebDocuments()`
- `web/runtime/accept-rpcstream.go` - Accepts fetch yamux streams
- `web/runtime/renderer.go` - `WebRenderer` enum (ELECTRON vs SAUCER)
- `web/document/remote.go` - WebDocument status monitoring
- `web/bldr/web-document.ts` - Detects saucer mode, creates `SaucerRuntimeClient`
- `util/framedstream/framedstream.go` - Length-prefix framing for RPC streams

### Build System (bldr)

- `web/entrypoint/saucer/bundle/bundle.go` - `BuildSaucerFromSource()`, `BuildSaucerJSBundle()`
- `dist.go` - Embeds `web/saucer/*.ts` into `DistSources`

### CLI (bldr)

- `cmd/bldr/debug.go` - `bldr debug eval` subcommand; sends EvalJS RPCs to saucer debug bridge over Unix socket

### E2E Tests (bldr)

- `web/plugin/saucer/e2e/e2e_test.go` - Integration tests for saucer controller lifecycle
- `web/plugin/saucer/e2e/fetch_test.go` - Tests for fetch pipeline

## Architecture

Single-pipe yamux connection over Unix domain sockets:

- **Main pipe** (`.pipe-{uuid}`): Carries all traffic -- JS-initiated RPC streams, Go-initiated RPC streams, and C++ SchemeForwarder fetch requests.

**Note:** `doc/SAUCER.md` describes a dual-pipe design (separate fetch pipe `.pipe-{uuid}-fetch`), but the current implementation uses a single yamux pipe for everything. The C++ `SchemeForwarder` opens yamux streams to Go for `bldr://` fetch requests, and the Go `DocumentManager` HTTP routes handle JS RPC streams -- all multiplexed over the same yamux session.

### Debug Bridge

The debug bridge allows external tools to evaluate JavaScript in the saucer webview and get actual results back (mirrors the alpha-debug pattern in `~/repos/aperture/alpha/`):
- **Proto service**: `SaucerDebugService` with `EvalJS` RPC defined in `web/plugin/saucer/saucer.proto`
- **Go side** (`debug-bridge.go`): Listens on a Unix socket at `.bldr/saucer-debug.sock` (overridable via `BLDR_DEBUG_SOCK` env var). Wraps user code in an async IIFE that evaluates and posts the result back via the saucer message channel (`window.webkit.messageHandlers.saucer.postMessage`). Single expressions are auto-wrapped with `return`. Sends the wrapped code to C++ as a length-prefixed protobuf frame (`EvalJSRequest`).
- **C++ side** (`bldr-saucer/src/main.cpp`): An `EvalRegistry` (condition variable + map) tracks pending evals. A `message` event handler intercepts `__bldr_eval` results from JS and delivers them to waiting threads. The `accept_thread` reads length-prefixed protobuf frames (`EvalJSRequest`), replaces the `__EVAL_ID__` placeholder with a unique ID, executes the JS, waits for the result (30s timeout), and returns a length-prefixed protobuf response (`EvalJSResponse`).
- **CLI**: `bldr debug eval` subcommand (`cmd/bldr/debug.go`) connects to the Unix socket and sends EvalJS RPCs. Single expressions like `document.title` or `1+1` return their value without needing explicit `return`.

**Expression auto-wrapping**: Single-line code without semicolons and without statement-keyword prefixes (`var`, `let`, `const`, `if`, `for`, etc.) is automatically wrapped with `return (expr)`. Multi-statement code needs an explicit `return` for a result. This matches the alpha-debug behavior.

**Eval Protocol Encoding** (Updated 2026-02-10): The Go<->C++ communication over yamux uses protobuf encoding (`EvalJSRequest`/`EvalJSResponse` messages defined in `web/plugin/saucer/saucer.proto`) with 4-byte little-endian length-prefix framing. Previously used JSON encoding. The JS->C++ postMessage path remains a simple string prefix format: `__bldr_eval:<id>:r:<result>` for success or `__bldr_eval:<id>:e:<error>` for errors (not JSON, just string concatenation). This avoids JSON parsing overhead on both sides of the yamux boundary while keeping the JS->C++ path simple since `postMessage` only accepts strings.

**Connection Handling** (Updated 2026-02-10): The debug bridge uses `*SingletonMuxedConn` directly instead of the `network.MuxedConn` interface. It calls `WaitConn()` + `OpenStream()` directly rather than going through `tryConn()`, which aggressively closes the entire yamux connection on any stream error. This provides more robust recovery when individual streams encounter transient issues.

The difference is significant:
- **`tryConn()` approach** (old): Wraps the operation in a loop that closes the entire connection if any error occurs, then waits for a new connection before retrying. A transient stream error takes down the whole connection.
- **`WaitConn()` + `OpenStream()` approach** (new): Gets a handle to the current connection and attempts to open a stream. If the stream open fails, only that operation fails - the underlying yamux connection can remain active and be reused for subsequent operations.

### Communication Protocols

JS communicates with C++ via HTTP endpoints tunneled through the `bldr://` scheme:
- `POST /b/saucer/{doc}/stream/{id}/write` - Write to stream
- `GET /b/saucer/{doc}/stream/{id}/read` - Read from stream (long-lived)
- `GET /b/saucer/{doc}/control` - Control stream for incoming stream notifications

## Fixed Bugs (Audit 2026-02-10)

1. **streamBridge.Read data truncation** -- Fixed by adding a `pending` buffer field to `streamBridge` that retains unconsumed bytes across Read calls.
2. **yamuxOnce.Do failure causes nil panic** -- Fixed by storing the error in `streamState.yamuxErr` instead of a local variable, so subsequent callers see the stored error.
3. **Control channel drops** -- Fixed by making the send blocking with context cancellation instead of non-blocking with default case.
4. **yamuxReadLoop goroutine leak** -- Fixed by adding `closeCh` channel to `streamState` and using select in `yamuxReadLoop` to unblock when stream is closed.
5. **Non-blocking fromJS drops data** -- Fixed by making the send blocking with select on `closeCh` and request context.
6. **No stream cleanup** -- Fixed by adding `removeStream` method and calling it from `yamuxReadLoop`'s defer.
7. **HandleWebDocumentRpc leaks srpc.Client** -- Fixed by returning a non-nil release function.
8. **cpp-yamux SYN WindowUpdate doubles send window** -- Fixed by sending delta=0 in SYN (constructor already sets initial_window_size) and skipping delta addition when receiving SYN in HandleWindowUpdate.
9. **C++ use-after-free on shutdown** -- Fixed by making forwarder a shared_ptr (captured by value) and guarding webview_ptr with a mutex+atomic shutdown flag.
10. **cpp-yamux RemoveStream dead code** -- Fixed by making RemoveStream public and calling it when streams reach Closed or Reset state (from Close, HandleData FIN, HandleWindowUpdate FIN, HandleReset, Reset).
11. **cpp-yamux no frame payload validation** -- Fixed by adding FrameTooLarge error and checking header_.length > kMaxFrameSize in FrameReader::Feed before allocating payload buffer.
12. **C++ double writer.finish()** -- Fixed by only calling writer.finish() at end of forward() if started is true (sendError already calls finish).
13. **DocumentManager broadcast missed-wakeup race** -- Fixed by replacing separate dm.mtx and dm.docBcast locks with a single dm.bcast (broadcast.Broadcast). Root cause: WatchWebRuntimeStatus, waitForDoc, and WaitDefaultDoc checked state while holding dm.mtx, then acquired dm.docBcast lock separately. If a broadcast fired between these locks, the wakeup was missed and the method blocked forever. This caused the white screen: WatchWebRuntimeStatus missed the document connect broadcast, so Remote never learned about the document, never created RemoteWebDocument, never sent WatchWebDocumentStatus RPC, Go never discovered WebViews, never called setRenderMode.

## Known Issues and Fixes (Historical)

### PipeClient Mutex Deadlock ("0 web documents")

**Symptom:** Saucer process starts but Go reports "0 web documents" -- no streams
can be opened and the webview never connects.

**Root cause:** `PipeClient` (in `bldr-saucer/src/pipe_client.h/cpp`) originally
used a single `mutex_` for all operations. The yamux `ReadLoop` holds this mutex
while blocking on a pipe read. Any write operation (`OpenStream`, `SendFrame`,
etc.) also needs the mutex, creating a deadlock. C++ cannot send any yamux frames
while `ReadLoop` is blocked waiting for data, so the connection stalls completely.

**Fix:** Split `mutex_` into two separate mutexes in `pipe_client.h/cpp`:
- `read_mtx_` -- protects read-side state and the `ReadLoop`
- `write_mtx_` -- protects write-side state and frame sending

Additionally, `close()` now calls `shutdown()` on the underlying file descriptors
to unblock any readers before acquiring locks, preventing deadlock during teardown.

### Accept Loop Infinite Loop (main.cpp)

**Symptom:** After a stream was closed in the C++ accept loop, the process would
spin in an infinite loop instead of accepting the next stream.

**Root cause:** The inner `while` loop that reads frames from a stream used
`continue` after calling `stream->Close()`. This jumped back to the inner loop's
condition check rather than breaking out to the outer accept loop, so no new
streams could ever be accepted.

**Fix:** Each accepted stream is now handled in a detached `std::thread`, so the
accept loop immediately returns to waiting for the next stream. This also
prevents a slow or stuck stream from blocking acceptance of other streams.

## Go Module Dependencies

From `go.mod`:
```
github.com/aperturerobotics/bldr-saucer v0.2.2
github.com/aperturerobotics/cpp-yamux v0.0.0-20260202024600-0e230930a899
github.com/aperturerobotics/saucer v0.0.0-20260207010621-91123c7138e4
```

## Running

```bash
bun clean && bun start:native  # Launch with saucer webview
```

Set `BLDR_WEB_RENDERER=saucer` env var to select saucer renderer.

## Full Documentation

See `doc/SAUCER.md` for detailed protocol specs, data flows, and design decisions.
