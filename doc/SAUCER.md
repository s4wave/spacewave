# Saucer: Native Webview Runtime for Bldr

## Overview

Saucer replaces Electron as the native desktop webview runtime for Bldr. Instead
of bundling a full Chromium browser, Saucer uses the platform's native webview
(WebKit on macOS/Linux, WebView2 on Windows) via the [saucer](https://github.com/aperturerobotics/saucer)
C++ library. The Go runtime communicates with the C++ webview process over Unix
domain sockets using yamux multiplexing and the SRPC RPC framework.

The implementation spans three repositories:

- **bldr** (`saucer-v3` branch) -- Go controller, C++ webview process, TypeScript client
- **cpp-yamux** -- C++ implementation of the yamux stream multiplexer protocol
- **starpc** -- C++ implementation of the SRPC RPC framework and RpcStream protocol

## Architecture

```
+------------------------------------------------------------------+
|                        Go Process (bldr)                         |
|                                                                  |
|  +------------------+    +-------------------+                   |
|  | Saucer           |    | Runtime           |                   |
|  | Controller       |--->| Controller        |                   |
|  | (plugin/saucer)  |    | (web/runtime)     |                   |
|  +--------+---------+    +--------+----------+                   |
|           |                       |                              |
|           | RunSaucer()           | Remote                       |
|           |                       |                              |
|  +--------v---------+   +--------v----------+                   |
|  | SingletonMuxedCon|   | SRPC Server       |                   |
|  | (main pipe)      |   | AcceptMuxedConn   |                   |
|  +--------+---------+   +-------------------+                   |
|           |                                                      |
|  +--------v---------+   +-------------------+                   |
|  | SingletonMuxedCon|   | ServiceWorkerHost |                   |
|  | (fetch pipe)     |   | AcceptRpcStreams   |                   |
|  +--------+---------+   +--------+----------+                   |
|           |                       |                              |
+-----------|------ Unix Sockets ---|------------------------------+
            |                       |
  .pipe-{uuid}            .pipe-{uuid}-fetch
            |                       |
+-----------|------ Unix Sockets ---|------------------------------+
|           |                       |                              |
|  +--------v---------+   +--------v----------+                   |
|  | Yamux Session    |   | Yamux Session     |                   |
|  | (client mode)    |   | (client mode)     |                   |
|  +--------+---------+   +--------+----------+                   |
|           |                       |                              |
|  +--------v---------+   +--------v----------+                   |
|  | ConnectionManager|   | FetchClient       |                   |
|  | (main RPC)       |   | (pkg/asset fetch) |                   |
|  +--------+---------+   +-------------------+                   |
|           |                                                      |
|  +--------v---------+                                            |
|  | Saucer Webview   |                                            |
|  | (bldr:// scheme) |                                            |
|  +------------------+                                            |
|                                                                  |
|                  C++ Process (bldr-saucer)                       |
+------------------------------------------------------------------+
```

## Changed Files (saucer-v3 vs origin/master)

**127 files changed, ~11,500 insertions, ~300 deletions.**

### New C++ Webview Process (`web/saucer/`)

| File                           | Purpose                                                                           |
| ------------------------------ | --------------------------------------------------------------------------------- |
| `main.cpp`                     | Entry point; creates saucer app, registers `bldr://` scheme, routes HTTP requests |
| `connection_manager.h/cpp`     | Bridges HTTP scheme requests to yamux streams; manages documents and RPC streams  |
| `fetch_client.h/cpp`           | Fetches assets from Go ServiceWorkerHost over the fetch yamux pipe                |
| `web_runtime_impl.h/cpp`       | Implements `SRPCWebRuntime` service (Go calls into C++ WebRuntime)                |
| `yamux_rpc_stream.h/cpp`       | Adapts yamux streams to the RpcStream interface with length-prefix framing        |
| `unix_socket_connection.h/cpp` | Implements yamux `Connection` over Unix domain sockets                            |
| `CMakeLists.txt`               | Build system; fetches saucer via CPM, links cpp-yamux + starpc from vendor        |
| `saucer.ts`                    | TypeScript client: `SaucerRuntimeClient` communicates with C++ via HTTP endpoints |
| `index.ts`                     | Re-exports `isSaucer`, `getDocId`, `SaucerRuntimeClient`                          |

### New Go Controller (`web/plugin/saucer/`)

| File                      | Purpose                                                                         |
| ------------------------- | ------------------------------------------------------------------------------- |
| `controller.go`           | Saucer runtime controller; starts process, sets up Remote with dual yamux pipes |
| `saucer.go`               | `RunSaucer()`: creates pipes, starts C++ subprocess, passes config via env vars |
| `factory.go`              | Controller factory for controllerbus                                            |
| `config.go`               | Config validation                                                               |
| `saucer.proto`            | Protobuf: `Config`, `SaucerInit`, `ExternalLinks`                               |
| `saucer.pb.go/ts/cc/h/rs` | Generated protobuf code                                                         |

### New Runtime Support

| File                                | Purpose                                                                              |
| ----------------------------------- | ------------------------------------------------------------------------------------ |
| `web/runtime/accept-rpcstream.go`   | Accepts fetch yamux streams, wraps in framedstream, routes to ServiceWorkerHost      |
| `web/runtime/renderer.go`           | `WebRenderer` enum (ELECTRON vs SAUCER), env var selection                           |
| `web/runtime/runtime.proto`         | Added `WebRuntimeClientInit` message, `WebRenderer` enum                             |
| `util/framedstream/framedstream.go` | `rpcstream.RpcStream` over `io.ReadWriteCloser` with LE uint32 length-prefix framing |

### New Build System (`web/entrypoint/saucer/bundle/`)

| File             | Purpose                                                                              |
| ---------------- | ------------------------------------------------------------------------------------ |
| `bundle.go`      | `BuildSaucerJSBundle()`: esbuild JS bundle; `CompileSaucerBinary()`: cmake C++ build |
| `bundle_test.go` | Tests for bundle/compile functions                                                   |

### Modified Files

| File                              | Change                                                                                                                         |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `web/bldr/web-document.ts`        | Detects saucer mode, creates `SaucerRuntimeClient` instead of `WebRuntimeClient`, skips WebAssembly/ServiceWorker/SharedWorker |
| `web/plugin/compiler/compiler.go` | Reads `BLDR_WEB_RENDERER` env var; adds `BundleSaucerHook` alongside `BundleElectronHook`                                      |
| `dist.go`                         | Embeds `web/saucer/*.ts` into `DistSources`                                                                                    |
| `web/deps.go`                     | Imports `cpp-yamux` for vendored C++ sources                                                                                   |

### E2E Tests (`web/plugin/saucer/e2e/`)

| File            | Purpose                                                                |
| --------------- | ---------------------------------------------------------------------- |
| `e2e_test.go`   | Integration tests for the saucer controller lifecycle                  |
| `fetch_test.go` | Tests for the fetch pipeline (C++ FetchClient -> Go ServiceWorkerHost) |

## Protocol Stack

```
+------------------------------------------------------------------+
|                        Application Layer                         |
|  Go: ServiceWorkerHost.Fetch, WebRuntime RPCs                    |
|  C++: FetchClient, WebRuntimeImpl                                |
|  JS: SaucerRuntimeClient, WebDocument                            |
+------------------------------------------------------------------+
|                         SRPC Layer                                |
|  Packet { CallStart | CallData | CallCancel }                    |
|  CallStart: service + method + optional first data               |
|  CallData: data + complete flag + error string                   |
+------------------------------------------------------------------+
|                      RpcStream Layer                              |
|  RpcStreamPacket { RpcStreamInit | RpcAck | bytes data }         |
|  Handshake: Init(component_id) -> Ack(error)                     |
|  Then: data packets containing serialized SRPC Packets           |
+------------------------------------------------------------------+
|                    Length-Prefix Framing                           |
|  [4-byte LE uint32 length] [message bytes]                       |
|  Go: framedstream.Stream                                         |
|  C++: YamuxRpcStream.WriteLengthPrefixed/ReadLengthPrefixed      |
+------------------------------------------------------------------+
|                      Yamux Multiplexer                            |
|  Frame header: [version:1][type:1][flags:2][streamID:4][len:4]   |
|  Types: Data(0), WindowUpdate(1), Ping(2), GoAway(3)            |
|  Flags: SYN(0x1), ACK(0x2), FIN(0x4), RST(0x8)                 |
|  Flow control: 256 KiB initial window per stream                 |
|  Client streams: odd IDs; Server streams: even IDs               |
+------------------------------------------------------------------+
|                     Unix Domain Socket                            |
|  SOCK_STREAM, path: .pipe-{runtimeUuid} / .pipe-{uuid}-fetch    |
+------------------------------------------------------------------+
```

## Yamux Stream State Machine (cpp-yamux)

```
                     OpenStream()        Accept()
                         |                   |
                         v                   v
                    +---------+         +-----------+
                    | SYNSent |         | SYNRecvd  |
                    +---------+         +-----------+
                         |                   |
                    recv ACK             send ACK
                         |                   |
                         v                   v
                    +----------------- --------+
                    |        Established        |
                    +---------------------------+
                    /                           \
               send FIN                     recv FIN
                  /                               \
                 v                                 v
          +------------+                   +-------------+
          | LocalClose |                   | RemoteClose |
          +------------+                   +-------------+
                  \                               /
               recv FIN                     send FIN
                    \                           /
                     v                         v
                    +---------------------------+
                    |          Closed           |
                    +---------------------------+

  At any point: send/recv RST --> Reset (hard close)
```

**Flow Control:**

- Each stream starts with a 256 KiB send and receive window
- Sender blocks when `send_window` reaches 0
- Receiver sends `WindowUpdate` frames when >50% of window is consumed
- WindowUpdate delta is added to sender's `send_window`

## Dual-Pipe Connection Architecture

The C++ process connects to Go over two separate yamux pipes:

```
  Pipe 1: Main RPC (.pipe-{uuid})
  ================================
  C++ is yamux client (opens streams)
  Go is yamux server (accepts streams)

  Direction 1: C++ -> Go (JS-initiated RPC streams)
    JS -> HTTP POST /b/saucer/{doc}/stream/{id}/write
       -> ConnectionManager writes to yamux stream
       -> Go framedstream reads, routes to SRPC server

  Direction 2: Go -> C++ (Go-initiated RPC calls)
    Go SRPC client opens yamux stream
       -> C++ AcceptLoop accepts stream
       -> ConnectionManager.HandleRpcStream routes to WebRuntimeImpl

  Pipe 2: Fetch (.pipe-{uuid}-fetch)
  ===================================
  C++ is yamux client (opens streams)
  Go is yamux server (accepts streams)

  Direction: C++ -> Go only
    C++ FetchClient opens yamux stream
       -> RpcStream handshake (Init -> Ack)
       -> SRPC call to ServiceWorkerHost.Fetch
       -> Go resolves pkg/asset request
       -> Response flows back on same stream
```

## Data Flow: JS RPC Call (JS -> Go)

This is the path for JS making an RPC call to the Go runtime (e.g., watching
state, creating documents):

```
 JS (in webview)                 C++ (bldr-saucer)              Go (bldr)
 ===============                 =================              =========

 1. SaucerRuntimeClient
    .openStream()
        |
 2. POST /b/saucer/{doc}/
    stream/{id}/write
    [WebRuntimeClientInit]
        |
        +---- bldr:// scheme ---->
                                  3. ConnectionManager
                                     .HandleStreamWrite()
                                         |
                                  4. Write to yamux stream
                                     [raw framed bytes]
                                         |
                                         +--- yamux pipe 1 --->
                                                                5. SRPC Server
                                                                   AcceptMuxedConn()
                                                                       |
                                                                6. framedstream
                                                                   reads packet
                                                                       |
                                                                7. ServerRPC
                                                                   dispatches to
                                                                   handler
                                                                       |
                                                                8. Handler sends
                                                                   response
                                                                       |
                                         <--- yamux pipe 1 ----+
                                  9. YamuxReadLoop reads
                                     response bytes
                                         |
                                  10. PushToJS(data)
                                         |
        <---- bldr:// scheme ----+
 11. GET /b/saucer/{doc}/
     stream/{id}/read
     returns streamed data
         |
 12. parseLengthPrefix
     Transform() decodes
     SRPC packets
         |
 13. RPC client processes
     response
```

## Data Flow: Go -> JS RPC Call (Go-initiated)

This is the path for Go making an RPC call to the JS WebRuntime (e.g.,
`CreateWebDocument`, `WebDocumentRpc`):

```
 Go (bldr)                       C++ (bldr-saucer)              JS (in webview)
 =========                       =================              ===============

 1. SRPC client opens
    yamux stream
        |
        +--- yamux pipe 1 --->
                                  2. AcceptLoop() accepts
                                     incoming stream
                                         |
                                  3. HandleRpcStream()
                                     reads length-prefixed
                                     SRPC packets
                                         |
                                  4. ServerRPC dispatches
                                     to WebRuntimeImpl
                                         |
                                  5. WebRuntimeImpl handles
                                     (e.g. WebDocumentRpc
                                      bridges to JS via
                                      StreamState queues)
                                         |
                                  6. For WebDocumentRpc:
                                     Push data to JS queue
                                     via PushToJS()
                                         |
                                         +--- control stream -->
                                                                 7. Control stream
                                                                    notifies JS of
                                                                    new stream ID
                                                                        |
                                                                 8. JS creates
                                                                    PacketStream
                                                                    for stream ID
                                                                        |
                                                                 9. Read/write via
                                                                    HTTP endpoints
```

## Data Flow: Fetch Request (Webview -> Go -> Webview)

This is the path for the webview loading a JavaScript module (e.g.,
`bldr:///b/pkg/react/index.mjs`):

```
 Webview                         C++ (bldr-saucer)              Go (bldr)
 =======                         =================              =========

 1. <script> or import()
    requests bldr:///b/pkg/
    react/index.mjs
        |
 2. handle_stream_scheme
    callback fires
        |
 3. URL doesn't match
    saucer API routes
        |
 4. Falls through to
    fetch catch-all
        |
        +---->
               5. FetchClient.fetch()
                      |
               6. OpenYamuxRpcStream()
                  on fetch pipe
                      |                    +--- yamux fetch pipe --->
                                                                 7. AcceptService
                                                                    WorkerRpcStreams()
                                                                        |
               8. OpenRpcStream()                                8. framedstream
                  handshake:                                        wraps stream
                  Init("") -> Ack                                       |
                      |                                          9. HandleRpcStream()
               9. SRPC NewStream()                                  reads Init,
                  "ServiceWorkerHost"                                sends Ack,
                  "Fetch"                                            creates ServerRPC
                      |                                                 |
               10. Send FetchRequest                             10. ServerRPC
                   {method, url,                                     dispatches to
                    headers}                                         ServiceWorkerHost
                      |                                              .Fetch()
               11. Send FetchRequest                                    |
                   {done: true}                                  11. Resolves pkg
                      |                                              request
               12. CloseSend()                                          |
                      |                                          12. Sends
               13. MsgRecv() blocks                                  FetchResponse
                   waiting for                                       {status, headers,
                   response...           <--- yamux fetch pipe ---    body, done}
                      |
               14. Receives response
                   (status, headers,
                    body)
                      |
        <-----+
 15. writer.start()
     writes response
     to webview
```

## Data Flow: JS -> Go Stream Detail (Connection Manager)

Detailed view of how the ConnectionManager bridges HTTP to yamux:

```
  JS POST /write                     ConnectionManager
  ==============                     =================

  1. HTTP body bytes
     arrive at handler
         |
  2. HandleStreamWrite()
     looks up DocumentState
     -> StreamState
         |
  3. If yamux_stream is
     null, OpenYamuxStream()
     and start YamuxReadLoop
         |
  4. yamux_stream->Write()
     sends raw bytes to Go
         |

  JS GET /read                       ConnectionManager
  ============                       =================

  1. HandleStreamRead()
     looks up stream state
         |
  2. If no yamux_stream,
     OpenYamuxStream() and
     start YamuxReadLoop
         |
  3. Loop: PopForJS(100ms)
     waits on to_js_cv
         |
  4. When data arrives
     (from YamuxReadLoop),
     write_cb sends to
     HTTP response stream
         |

  YamuxReadLoop (background thread)
  ==================================

  1. Reads raw bytes from
     yamux_stream
         |
  2. PushToJS(data) ->
     notifies to_js_cv
         |
  3. HandleStreamRead
     wakes up, sends
     to HTTP client
```

## Startup Sequence

```
  Go Process                                      C++ Process
  ==========                                      ===========

  1. Factory creates Controller
     with saucer config
         |
  2. Controller.Execute()
     acquires semaphore
         |
  3. RunSaucer():
     a. Create .pipe-{uuid} listener
     b. Create .pipe-{uuid}-fetch listener
     c. Start SingletonMuxedConn for each (server mode)
     d. Write bootstrap HTML + entrypoint JS to temp files
     e. Set env: BLDR_RUNTIME_ID, BLDR_SAUCER_INIT
     f. cmd.Start() launches C++ process
         |                                         |
         |                                    4. main():
         |                                       a. Read env vars
         |                                       b. Connect to .pipe-{uuid}
         |                                       c. Create yamux client session
         |                                       d. Create ConnectionManager
         |                                       e. Create WebRuntimeImpl + SRPC Mux
         |                                       f. Register WebRuntime service
         |                                       g. SetRpcMux on ConnectionManager
         |                                       h. StartAcceptLoop (Go -> C++ RPCs)
         |                                       i. Connect FetchClient to
         |                                          .pipe-{uuid}-fetch
         |                                       j. Register bldr:// scheme
         |                                       k. Create app + webview
         |                                       l. Navigate to bldr:///index.html
         |                                          ?webDocumentId={id}
         |                                         |
  5. runtime_controller runs                  5. Webview loads HTML
     Remote with dual accept:                    -> loads /entrypoint.mjs
     a. AcceptMuxedConn (main pipe)              -> JS creates WebDocument
     b. AcceptServiceWorkerRpcStreams             -> detects isSaucer
        (fetch pipe)                             -> creates SaucerRuntimeClient
         |                                       -> POST /b/saucer/{doc}/connect
         |                                       -> GET /b/saucer/{doc}/control
         |                                       -> opens RPC streams via HTTP
```

## SRPC Packet Protocol (starpc)

```
  Packet (protobuf oneof):
  ========================

  CallStart {            CallData {             CallCancel = true
    rpc_service: str       data: bytes
    rpc_method: str        data_is_zero: bool
    data: bytes            complete: bool
    data_is_zero: bool     error: string
  }                      }

  Unary RPC Flow:
  ===============

  Client                          Server
  ------                          ------
  CallStart{service,method,data}
  ---------------------------------->
                                  Invoke handler
                                  <process request>
                        CallData{data,complete=true}
  <----------------------------------

  Streaming RPC Flow:
  ===================

  Client                          Server
  ------                          ------
  CallStart{service,method}
  ---------------------------------->
  CallData{data}
  ---------------------------------->
  CallData{data}
  ---------------------------------->
  CallData{complete=true}
  ---------------------------------->
                            CallData{data}
  <----------------------------------
                            CallData{data,complete=true}
  <----------------------------------
```

## RpcStream Protocol (starpc/rpcstream)

```
  RpcStreamPacket (protobuf oneof):
  =================================

  RpcStreamInit {        RpcAck {             data: bytes
    component_id: str      error: string        (raw SRPC packet)
  }                      }

  Handshake Flow:
  ===============

  Client (C++ FetchClient)            Server (Go ServiceWorkerHost)
  ========================            ============================

  RpcStreamInit{component_id=""}
  ---------------------------------->
                                      Lookup invoker for component_id
                              RpcAck{error=""}
  <----------------------------------

  Then SRPC packets flow as `data` in RpcStreamPacket:

  RpcStreamPacket{data=<CallStart>}
  ---------------------------------->
  RpcStreamPacket{data=<CallData>}
  ---------------------------------->
                        RpcStreamPacket{data=<CallData>}
  <----------------------------------
```

## Fetch Protocol (web.runtime.sw.ServiceWorkerHost.Fetch)

```
  FetchRequest (protobuf oneof):       FetchResponse (protobuf oneof):
  ==============================       ===============================

  FetchRequestInfo {                   FetchResponseInfo {
    method: str                          status: uint32
    url: str                             status_text: str
    headers: map<str,str>                ok: bool
    has_body: bool                       headers: map<str,str>
    client_id: str                     }
  }
                                       FetchResponseData {
  FetchRequestData {                     data: bytes
    data: bytes                          done: bool
    done: bool                         }
  }

  Fetch Streaming RPC:
  ====================

  C++ FetchClient                     Go ServiceWorkerHost
  ================                    ====================

  FetchRequest{request_info}
  ---------------------------------->
  FetchRequest{request_data{done=true}}
  ---------------------------------->
  CloseSend()
                              FetchResponse{response_info}
  <----------------------------------
                              FetchResponse{response_data{data,done=true}}
  <----------------------------------
  Stream completes
```

## Build System

### JavaScript Bundle

```bash
# Build JS bundle for saucer (in bundle.go)
esbuild entrypoint.tsx \
  --bundle \
  --format=esm \
  --define:BLDR_SAUCER=true \
  --external:react --external:react-dom ...
  # External deps served via import map at /b/pkg/
```

The bootstrap HTML includes an import map that maps bare specifiers like `react`
to `/b/pkg/react/index.mjs`. These URLs go through the `bldr://` scheme handler
and are fetched via the FetchClient -> ServiceWorkerHost path.

### C++ Binary

```bash
# Compile saucer binary (in bundle.go)
cmake -G Ninja \
  -DCMAKE_BUILD_TYPE=Release \
  -DCPM_SOURCE_CACHE=<cache_dir> \
  <source_dir>/web/saucer

ninja -C <build_dir>
```

Dependencies fetched via CPM.cmake:

- `saucer` (aperturerobotics/saucer fork with `handle_stream_scheme` API)
- `protobuf` (system)
- `abseil` (system)
- `cpp-yamux` (vendored via `go:embed` in Go module)
- `starpc` (vendored via `go:embed` in Go module)

## Key Design Decisions

1. **Dual yamux pipes** rather than a single multiplexed connection: separates
   main RPC traffic (bidirectional Go<->C++) from fetch traffic (C++->Go only)
   to avoid protocol interference and simplify routing.

2. **HTTP tunneling for JS<->C++**: The saucer webview's `handle_stream_scheme`
   API only supports HTTP request/response semantics. Bidirectional streaming is
   achieved by splitting into separate read (long-lived GET) and write (repeated
   POST) endpoints per stream.

3. **No WebAssembly, no ServiceWorker, no SharedWorker**: In saucer mode, the Go
   runtime runs natively (not in WASM). The C++ process replaces the browser
   infrastructure that Electron provided. JS communicates with Go through C++ as
   a bridge rather than through browser APIs.

4. **Length-prefix framing everywhere**: All stream protocols use 4-byte
   little-endian uint32 length prefixes. This is consistent across Go
   (`framedstream`), C++ (`YamuxRpcStream`), and JS (`parseLengthPrefixTransform`).

5. **C++ as yamux client**: The C++ process always initiates yamux streams
   (client mode, odd stream IDs). Go always accepts (server mode, even stream
   IDs). For Go->C++ calls, Go's SRPC server opens streams on the main muxed
   connection where C++ has its `AcceptLoop` running.

## Saucer Framework Features Reference

The saucer C++ framework provides a rich set of features beyond basic webview
rendering. This section documents the full feature surface and how bldr can
leverage each capability.

### Modules

Saucer supports a module system for extending functionality. Modules are
separate CMake packages that link against the saucer target and use existing
event mechanisms and native interfaces.

**Official modules:**

| Module      | Package           | Version | Purpose                                     |
| ----------- | ----------------- | ------- | ------------------------------------------- |
| **desktop** | `saucer::desktop` | 4.2.0   | File dialogs, mouse position, URI launching |
| **pdf**     | `saucer::pdf`     | 3.0.0   | Export current page as PDF                  |
| **loop**    | `saucer::loop`    | -       | Legacy loop implementation                  |

Modules are added via CPM in CMakeLists.txt:

```cmake
CPMFindPackage(
  NAME saucer-desktop
  VERSION 4.2.0
  GIT_REPOSITORY "https://github.com/saucer/desktop"
)
target_link_libraries(${PROJECT_NAME} PRIVATE saucer::desktop)
```

### saucer-desktop Module

The `saucer::modules::desktop` class provides three capabilities with
platform-native implementations:

**File/Folder Picker:**

```cpp
#include <saucer/modules/desktop.hpp>

saucer::modules::desktop desktop{app};

// Single file
auto file = desktop.pick<saucer::picker::type::file>({
  .initial = "/home/user",
  .filters = {"*.txt", "*.md"},
});

// Multiple files
auto files = desktop.pick<saucer::picker::type::files>();

// Folder
auto folder = desktop.pick<saucer::picker::type::folder>();

// Save dialog
auto save = desktop.pick<saucer::picker::type::save>({
  .filters = {"*.pdf"},
});
```

Returns `saucer::result<T>` which contains either the result or
`std::errc::operation_canceled` if the user cancels.

**Mouse Position:**

```cpp
auto pos = desktop.mouse_position(); // returns {x, y}
```

**URI/File Launching:**

```cpp
desktop.open("https://example.com");       // opens in browser
desktop.open("/path/to/file.pdf");         // opens with default app
desktop.open("mailto:user@example.com");   // opens mail client
```

Platform backends:

- macOS: `NSOpenPanel`/`NSSavePanel`, `NSEvent.mouseLocation`, `NSWorkspace.openURL`
- Windows: `IFileOpenDialog` (COM), `GetCursorPos`, `ShellExecuteW`
- Linux (GTK): `GtkFileDialog`, `GtkUriLauncher`/`GtkFileLauncher`
- Linux (Qt): `QFileDialog`, `QCursor::pos`, `QDesktopServices::openUrl`

### saucer-pdf Module

```cpp
#include <saucer/modules/pdf.hpp>

saucer::modules::pdf pdf{webview};
auto data = co_await pdf.print(); // returns PDF bytes
```

### Native API

Saucer exposes underlying platform objects through `native()` for features not
directly wrapped by the framework. Requires linking `saucer::private`.

**Stable natives** (guaranteed stable layout):

```cpp
#include <saucer/modules/stable/webkit.hpp>
auto native = webview->native();
// native.webview -> platform-specific webview object
// macOS: WKWebView*, Linux: WebKitWebView*
```

**Unstable natives** (internal implementation details):

```cpp
auto *native = webview->native<false>();
// Access to internal platform implementation
```

### JavaScript Interop (smartview)

`saucer::smartview` extends `saucer::webview` with JS interoperability:

**Expose C++ functions to JS:**

```cpp
webview->expose("multiply", [](double a, double b) {
    return a * b;
});
// JS: const result = await saucer.exposed.multiply(5, 10);
```

Supports `std::expected` returns, exceptions, and `saucer::executor<T>` for
async resolution.

**Evaluate JS from C++:**

```cpp
auto val = *co_await webview->evaluate<double>("Math.random()");
// With interpolation:
auto pow = *co_await webview->evaluate<double>("Math.pow({}, {})", 2, 5);
```

**Execute JS (fire-and-forget):**

```cpp
webview->execute("console.log({})", saucer::make_args(1, "Test"));
```

### Script Injection

Inject JavaScript that runs on page load:

```cpp
auto id = webview->inject({
    .code    = "console.log('injected')",
    .run_at  = saucer::script::time::creation, // or ::ready
    .no_frames = true,   // skip sub-frames
    .clearable = true,   // removable via uninject()
});

webview->uninject(id);   // remove specific script
webview->uninject();     // remove all clearable scripts
```

### Webview Events

Events are subscribed via `webview->on<saucer::webview::event::EVENT>(callback)`.

| Event        | Parameters                        | Return   | Notes                                                |
| ------------ | --------------------------------- | -------- | ---------------------------------------------------- |
| `permission` | `shared_ptr<permission::request>` | `status` | Call `request->accept(true/false)`; ignored = denied |
| `fullscreen` | `bool fullscreened`               | `policy` | Block/allow fullscreen requests                      |
| `dom_ready`  | -                                 | -        | Fires when DOM is ready                              |
| `navigated`  | `const uri&`                      | -        | Fires after navigation completes                     |
| `navigate`   | `const navigation&`               | `policy` | Block/allow navigation requests                      |
| `message`    | `string_view`                     | `status` | RPC message; handled/unhandled                       |
| `request`    | `const uri&`                      | -        | Backend-dependent frequency (see below)              |
| `favicon`    | `const icon&`                     | -        | Icon has raw bytes, saveable to disk                 |
| `title`      | `string_view`                     | -        | Page title changed                                   |
| `load`       | `const state&`                    | -        | `state::started` or `state::finished`                |

**Permission types** (`request->type()`): `unknown`, `audio_media`,
`video_media`, `desktop_media`, `mouse_lock`, `device_info`, `location`,
`clipboard`, `notification`.

**Navigate properties** (`navigation&`): `url()`, `new_window()`,
`redirection()`, `user_initiated()`.

**Request event note:** Fires less frequently on WebKitGtk than on WebView2.
Behavior varies across backends.

**Example:**

```cpp
webview->on<saucer::webview::event::navigate>(
    [](const saucer::navigation& nav) -> saucer::policy {
        if (nav.new_window() || !is_internal(nav.url())) {
            return saucer::policy::block;
        }
        return saucer::policy::allow;
    });

webview->on<saucer::webview::event::permission>(
    [](const std::shared_ptr<saucer::permission::request>& req) -> saucer::status {
        if (req->type() == saucer::permission::clipboard) {
            req->accept(true);
            return saucer::status::handled;
        }
        return saucer::status::unhandled; // default denial
    });
```

### Window Events

Events are subscribed via `window->on<saucer::window::event::EVENT>(callback)`.

| Event       | Parameters              | Return   | Notes                                       |
| ----------- | ----------------------- | -------- | ------------------------------------------- |
| `decorated` | `window::decoration`    | -        | Decoration style changed                    |
| `maximize`  | `bool`                  | -        | true = maximized                            |
| `minimize`  | `bool`                  | -        | true = minimized                            |
| `resize`    | `int width, int height` | -        | Window dimensions changed                   |
| `focus`     | `bool`                  | -        | true = focused                              |
| `closed`    | -                       | -        | Window was closed                           |
| `close`     | -                       | `policy` | Block/allow close; gate for unsaved changes |

**Event registration returns an ID** for later removal:

```cpp
const auto id = window->on<saucer::window::event::resize>(
    [](int width, int height) { /* ... */ });
window->off(saucer::window::event::resize, id);  // remove specific handler
window->off(saucer::window::event::resize);       // remove all handlers
```

**Non-clearable events** persist through wildcard `off()` calls:

```cpp
window->on<saucer::window::event::resize>({{
    .func = [](int width, int height) { /* ... */ },
    .clearable = false,
}});
```

**Awaitable events** for coroutine-based control flow:

```cpp
// Block until close event fires, automatically allowing it
co_await window->await<saucer::window::event::close>(saucer::policy::allow);
```

### Window Decorations

Controls the non-client area (titlebar, resize borders) of the window:

```cpp
window->set_decorations(saucer::window::decoration::partial);
```

| Type      | Titlebar | Resizable | Aero-Snap | Shadows | Notes                            |
| --------- | -------- | --------- | --------- | ------- | -------------------------------- |
| `full`    | Yes      | Yes       | Yes       | Yes     | Default system decorations       |
| `partial` | No       | Yes       | Yes       | Yes     | Recommended for custom titlebars |
| `none`    | No       | Yes       | No        | No      | Bare window; avoid if possible   |

**HTML data attributes** (when `saucer::webview::options{.attributes = true}`):

| Attribute                      | Effect                                                |
| ------------------------------ | ----------------------------------------------------- |
| `data-webview-close`           | Closes window on click                                |
| `data-webview-minimize`        | Minimizes window on click                             |
| `data-webview-maximize`        | Toggles maximize/restore on click                     |
| `data-webview-drag`            | Enables window dragging when held                     |
| `data-webview-resize="<edge>"` | Resizes from edge(s): `t`, `b`, `l`, `r` combinations |
| `data-webview-ignore`          | Prevents operations on child elements                 |

**Exposed JS functions** (available globally on `saucer`):

```javascript
saucer.close()
saucer.startDrag()
saucer.startResize(saucer.windowEdge)
saucer.minimize(bool) / saucer.minimized() // get/set
saucer.maximize(bool) / saucer.maximized() // get/set
```

### Custom URL Schemes

Schemes are registered before webview creation and handled per-instance:

```cpp
saucer::webview::register_scheme("myapp");
// ...
webview->handle_scheme("myapp", [](const saucer::scheme::request& req) {
    return saucer::scheme::response{
        .data   = saucer::stash::from_str("Hello"),
        .mime   = "text/plain",
        .status = 200,
    };
});
```

Request object exposes: URL, method, body, headers.
Response requires: data, mime, status; optional headers.

### Embedding

Static files can be embedded into the binary via CMake:

```cmake
saucer_embed("out" TARGET ${PROJECT_NAME})
target_link_libraries(${PROJECT_NAME} PRIVATE saucer::embedded)
```

```cpp
#include <saucer/embedded/all.hpp>
webview->embed(saucer::embedded::all());
```

## Plans: Leveraging Saucer Features in Bldr

### Plan 1: Desktop Module Integration (File Dialogs, URI Launch)

**Goal:** Expose native file dialogs and URI launching to the JS frontend via
RPC, replacing Electron's `dialog` and `shell` APIs.

**Implementation:**

1. Add `saucer-desktop` as a CPM dependency in the bldr-saucer CMakeLists.txt:

   ```cmake
   CPMFindPackage(
     NAME saucer-desktop
     VERSION 4.2.0
     GIT_REPOSITORY "https://github.com/saucer/desktop"
   )
   target_link_libraries(bldr-saucer PRIVATE saucer::desktop)
   ```

2. Create a `DesktopService` protobuf service exposing the desktop module:

   ```protobuf
   service DesktopService {
     rpc PickFile(PickFileRequest) returns (PickFileResponse);
     rpc PickFiles(PickFilesRequest) returns (PickFilesResponse);
     rpc PickFolder(PickFolderRequest) returns (PickFolderResponse);
     rpc PickSave(PickSaveRequest) returns (PickSaveResponse);
     rpc OpenURI(OpenURIRequest) returns (OpenURIResponse);
   }

   message PickFileRequest {
     string initial_path = 1;
     repeated string filters = 2;
   }

   message PickFileResponse {
     string path = 1;
     bool canceled = 2;
   }
   ```

3. Implement the service in the C++ process. The desktop module requires
   running on the main thread (UI thread), so use `saucer::utils::invoke<>`
   to dispatch calls from the RPC thread to the main thread.

4. Register the service on the existing SRPC mux so Go can call it, or
   alternatively handle it directly in C++ and expose it to JS via the
   existing HTTP endpoint pattern.

5. On the Go side, create a `DesktopDirective` that resolves via the saucer
   RPC connection, allowing any controller to request file dialogs.

6. On the JS side, create a `DesktopClient` that wraps the SRPC client for
   the DesktopService, callable from React components.

**Electron comparison:** Electron provides `electron.dialog.showOpenDialog()` and
`electron.dialog.showSaveDialog()` on the main process, plus `shell.openExternal()`
for URI launching. The current bldr Electron app uses `shell.openExternal()` for
external links (`web/electron/main/app.ts:230,240`) but does not yet use file
dialogs. In saucer, `saucer::modules::desktop` replaces both APIs with a single
cross-platform module, but requires explicit RPC bridging since there is no
built-in IPC like Electron's `ipcMain`/`ipcRenderer`.

**Use cases in bldr:**

- "Open project" dialog for selecting workspace directories
- "Save as" for exporting documents or configurations
- Opening external links (clicking URLs in rendered content)
- File import for drag-and-drop alternatives

### Plan 2: Navigate Event for External Link Handling

**Goal:** Intercept navigation requests to handle external links (open in
system browser) vs internal links (route within bldr).

**Implementation:**

1. Subscribe to the `Navigate` webview event in the C++ process:

   ```cpp
   webview->on<saucer::webview_event::navigate>(
       [](const saucer::navigation& nav) {
           if (nav.new_window() || !is_bldr_url(nav.url())) {
               desktop.open(nav.url().string());
               return saucer::policy::block;
           }
           return saucer::policy::allow;
       });
   ```

2. This replaces Electron's `shell.openExternal` pattern and handles
   `<a target="_blank">` links, `window.open()` calls, and redirects to
   external domains.

3. No Go-side changes needed -- this is purely a C++ webview concern.

**Electron comparison:** Electron handles this with two separate handlers in
`web/electron/main/app.ts`: `will-navigate` (line 218) intercepts in-page
navigation and checks `isInternalUrl()` before calling `shell.openExternal()`,
while `setWindowOpenHandler()` (line 236) handles `window.open()` calls and
also supports creating popout windows for internal URLs with hash routing.
The Electron implementation also respects the `ExternalLinks` config enum
(ALLOW/DENY). Saucer's `Navigate` event consolidates both handlers into a
single event with richer metadata (`new_window()`, `user_initiated()`,
`redirection()`), but lacks Electron's multi-window popout capability -- that
would need separate implementation if desired.

### Plan 3: Window Events for Lifecycle Management

**Goal:** Use saucer window events for proper lifecycle management instead of
relying solely on process exit.

**Implementation:**

1. Subscribe to `Close` event to implement "unsaved changes" confirmation:

   ```cpp
   webview->on<saucer::window_event::close>([&]() {
       if (has_unsaved_changes()) {
           return saucer::policy::block; // prevent close
       }
       return saucer::policy::allow;
   });
   ```

2. Subscribe to `Focus` event to notify Go when the window gains/loses focus,
   enabling Go controllers to deprioritize background work:

   ```cpp
   webview->on<saucer::window_event::focus>([&](bool focused) {
       // Send focus state to Go via RPC
       notify_focus_state(focused);
   });
   ```

3. Subscribe to `Resize` event to persist window geometry for session restore.

4. Subscribe to `Minimize` event to reduce resource usage when minimized
   (complementing the existing Page Visibility API detection in JS).

**Electron comparison:** Electron's `BrowserWindow` emits `closed` events which
bldr uses for cleanup (`web/electron/main/app.ts:256,293`) -- removing the
document from `browserWindows` and calling `webRuntime.removeConnection()`. The
Electron app also sets `backgroundThrottling: true` in webPreferences (line 199)
and relies on the JS-side `document.visibilitychange` event for
visibility-aware reconnect with exponential backoff. Electron does not currently
use close-prevention or focus/minimize tracking. Saucer's window events provide
the same capabilities but at the C++ level rather than JS, which means
lifecycle management happens closer to the native layer without depending on
JS execution context.

### Plan 4: smartview Interop for Direct JS Communication

**Goal:** Use saucer's native JS interop (`expose`/`evaluate`) as a faster
communication channel for latency-sensitive operations, bypassing the HTTP
tunneling overhead.

**Implementation:**

1. Switch from `saucer::webview` to `saucer::smartview` in main.cpp.

2. Expose control functions directly to JS:

   ```cpp
   webview->expose("__bldr_push", [&](std::string stream_id, std::string data) {
       // Push data directly to a stream, bypassing HTTP POST
       connection_manager.handle_push(stream_id, data);
   });
   ```

3. Use `evaluate` to push data to JS without waiting for HTTP polling:

   ```cpp
   // Instead of waiting for GET /read, push directly
   webview->execute("__bldr_recv({}, {})",
       saucer::make_args(stream_id, base64_data));
   ```

4. Keep the HTTP endpoints as fallback for large transfers (scheme handler
   supports streaming; `evaluate` is better for small messages).

**Benefits:**

- Eliminates HTTP request/response overhead for small RPC messages
- Reduces latency for interactive operations (typing, cursor movement)
- Control stream notifications become instant instead of long-poll

**Trade-offs:**

- `evaluate`/`execute` serialize through the main thread
- Large payloads are better served through the scheme handler
- A hybrid approach (smartview for control + small messages, HTTP for bulk
  data) provides the best balance

**Electron comparison:** Electron uses `contextBridge` + `ipcMain`/`ipcRenderer`
with `MessagePort` for direct bidirectional communication. The preload script
(`web/electron/main/preload.ts`) exposes `openClientPort()` which sends a
`WebRuntimeClientInit` message and a `MessagePort` to the main process via
`BLDR_ELECTRON_CLIENT_OPEN` IPC (app.ts:132). The main process wraps the
MessagePort into the WebRuntime client handler. This gives Electron zero-copy
structured clone transfer of binary data. Saucer's smartview interop would
provide a similar direct channel but with JSON serialization (via glaze) rather
than structured clone, and all calls serialize through the C++ UI thread. The
HTTP tunneling approach that saucer currently uses is analogous to what would
happen if Electron used `fetch()` to talk to the main process instead of IPC.

### Plan 5: Script Injection for Early Initialization

**Goal:** Use saucer's script injection to run initialization code before the
page loads, eliminating the bootstrap HTML + entrypoint.mjs two-step.

**Implementation:**

1. Inject the saucer runtime client initialization at `creation` time:

   ```cpp
   webview->inject({
       .code = R"(
           window.__BLDR_SAUCER = true;
           window.__BLDR_DOC_ID = ')" + doc_id + R"(';
       )",
       .run_at = saucer::script::time::creation,
       .no_frames = true,
       .clearable = false,
   });
   ```

2. This ensures the saucer detection flag is available before any JS module
   runs, removing the need for URL query parameter parsing.

3. The entrypoint.mjs can be embedded and served via the scheme handler,
   with the injected script providing configuration.

**Electron comparison:** Electron uses a preload script
(`web/electron/main/preload.ts`) that runs in an isolated context before the
page loads. The preload exposes `BLDR_ELECTRON` on the `contextBridge`, which
the renderer uses to detect Electron mode and open the client MessagePort. The
preload is specified via `webPreferences.preload` (app.ts:194) and runs with
`contextIsolation: true`. Saucer's script injection serves a similar purpose
but without context isolation -- injected scripts run in the page's global
scope. The `creation` timing is equivalent to Electron's preload timing. The
saucer approach is simpler (no separate file, no bridge API) but less secure
for untrusted content.

### Plan 6: Permission Request Handling

**Goal:** Handle webview permission requests (camera, microphone, clipboard,
etc.) by routing them through Go for policy decisions.

**Implementation:**

1. Subscribe to the `Permission` event:

   ```cpp
   webview->on<saucer::webview::event::permission>(
       [&](const std::shared_ptr<saucer::permission::request>& req)
           -> saucer::status {
           // Always allow clipboard for bldr:// origins
           if (req->type() == saucer::permission::clipboard) {
               req->accept(true);
               return saucer::status::handled;
           }
           // Route other permissions to Go for policy decision
           bool allowed = ask_permission_via_rpc(req->uri(), req->type());
           req->accept(allowed);
           return saucer::status::handled;
       });
   ```

2. Create a `PermissionService` RPC that Go controllers can use to define
   permission policies (always allow clipboard for bldr:// origins, prompt
   for camera/microphone, etc.).

3. Initially, default to allowing clipboard access (needed for copy/paste in
   the editor) and blocking other permissions.

**Electron comparison:** Electron provides `session.setPermissionRequestHandler()`
for centralized permission control, but the current bldr Electron app does not
use it -- permissions default to Chromium's built-in behavior. Electron's
`webPreferences` enforce `sandbox: true`, `nodeIntegration: false`, and
`contextIsolation: true` (app.ts:191-196) for security. Saucer has no
equivalent of Electron's sandbox or context isolation; the `Permission` event
is the primary mechanism for controlling what the webview can access.

### Plan 7: PDF Export

**Goal:** Enable exporting bldr documents as PDFs using the saucer-pdf module.

**Implementation:**

1. Add `saucer-pdf` as a CPM dependency:

   ```cmake
   CPMFindPackage(
     NAME saucer-pdf
     VERSION 3.0.0
     GIT_REPOSITORY "https://github.com/saucer/pdf"
   )
   target_link_libraries(bldr-saucer PRIVATE saucer::pdf)
   ```

2. Add a `PrintToPDF` RPC method on the existing WebRuntime service:

   ```protobuf
   rpc PrintToPDF(PrintToPDFRequest) returns (stream PrintToPDFResponse);
   ```

3. Implement in C++ using the pdf module, streaming the result back to Go:

   ```cpp
   auto data = co_await pdf.print();
   // Stream PDF bytes back via RPC
   ```

4. Go can then write the PDF to disk or serve it to the user.

**Electron comparison:** Electron provides `webContents.printToPDF(options)`
as a built-in API that returns a `Buffer` with the PDF data. The current bldr
Electron app does not use this feature. The saucer-pdf module provides the same
capability but requires an additional CMake dependency and RPC plumbing to get
the PDF bytes back to Go. Both approaches produce the PDF from the webview's
rendered content.

### Plan 8: Window Title Sync

**Goal:** Sync the native window title with the current document/page title.

**Implementation:**

1. Subscribe to the `Title` webview event:

   ```cpp
   webview->on<saucer::webview_event::title>([&](std::string_view title) {
       // Optionally prefix with "bldr - "
       webview->set_title(std::format("bldr - {}", title));
   });
   ```

2. Alternatively, Go can set the window title via an RPC call when the active
   document changes, giving Go full control over the title.

3. The JS frontend sets `document.title` based on the current route/document,
   and the webview event propagates it to the native title bar.

**Electron comparison:** Electron's `BrowserWindow` automatically syncs
`document.title` to the native window title bar by default -- no additional
code is needed. The current bldr Electron app does not call `setTitle()`
explicitly. Saucer's webview may also auto-sync the title depending on the
platform backend, but the `Title` event gives explicit control to prefix or
transform the title (e.g., "bldr - Document Name") which Electron would
require a `page-title-updated` event handler to achieve.

### Plan 9: Navigate Event for Routing Telemetry

**Goal:** Use the `Navigated` event to track in-app navigation for debugging
and analytics.

**Implementation:**

1. Subscribe to `Navigated` and `Load` events to track page lifecycle:

   ```cpp
   webview->on<saucer::webview_event::load>([&](const saucer::state& state) {
       // Log page load started/finished for performance tracking
       notify_load_state(state);
   });
   ```

2. Feed navigation telemetry to the Go debug bridge, making it visible via
   `bldr debug` CLI commands.

**Electron comparison:** Electron provides `did-navigate`, `did-navigate-in-page`,
`did-start-loading`, and `did-stop-loading` events on `webContents` for the same
purpose. The current bldr Electron app only uses `will-navigate` for link
interception and does not collect navigation telemetry. Saucer's `Load` event
(started/finished) and `Navigated` event provide equivalent signals.

### Plan 10: Custom Titlebar with Window Decorations

**Goal:** Replicate the Electron custom titlebar using saucer's decoration
system and HTML data attributes.

**Implementation:**

1. Set `partial` decorations to remove the native titlebar while keeping
   resize handles, aero-snap, and window shadows:

   ```cpp
   window->set_decorations(saucer::window::decoration::partial);
   ```

2. Enable webview attributes for HTML-driven window controls:

   ```cpp
   auto webview = app->make<saucer::webview>(
       saucer::webview::options{.attributes = true});
   ```

3. Add data attributes to the existing React titlebar component:

   ```html
   <div data-webview-drag class="titlebar">
     <button data-webview-minimize>_</button>
     <button data-webview-maximize>[]</button>
     <button data-webview-close>X</button>
   </div>
   ```

4. On macOS, the native traffic lights (close/minimize/maximize) are
   preserved with `partial` decorations, matching the current Electron
   behavior where `titleBarStyle: 'hidden'` keeps the traffic lights.
   The `data-webview-drag` attribute on the titlebar area provides the
   same drag behavior as Electron's `-webkit-app-region: drag`.

5. On Windows/Linux, the frameless look is achieved by `partial` decorations
   removing the native titlebar entirely. The HTML buttons provide
   close/minimize/maximize via the data attributes, matching the current
   Electron behavior where `frame: false` removes the native frame.

**Electron comparison:** Electron uses `frame: isMac` and
`titleBarStyle: isMac ? 'hidden' : undefined` (`web/electron/main/app.ts:183-185`)
to get platform-appropriate custom titlebars. On macOS, `hidden` keeps traffic
lights but hides the titlebar; on Windows/Linux, `frame: false` removes the
frame entirely. The JS frontend then uses `-webkit-app-region: drag` for window
dragging. Saucer's `decoration::partial` achieves the same result across
platforms with a single setting, and replaces `-webkit-app-region` with the
`data-webview-drag` attribute. The saucer approach also provides
`data-webview-resize` for edge-specific resize handles, which Electron handles
automatically via the native frame.

### Implementation Priority

Plans are prioritized to achieve parity with the existing Electron
implementation first, then add new capabilities. Features that Electron
currently implements are P0/P1; features neither renderer implements yet are
P2/P3.

| Priority | Plan                                  | Effort | Impact                  | Electron Parity                  |
| -------- | ------------------------------------- | ------ | ----------------------- | -------------------------------- |
| P0       | Plan 2: External link handling        | Low    | Prevents broken UX      | Yes -- Electron has this         |
| P0       | Plan 8: Window title sync             | Low    | Desktop polish          | Yes -- Electron auto-syncs       |
| P0       | Plan 10: Custom titlebar              | Low    | Window chrome           | Yes -- Electron has this         |
| P1       | Plan 3: Window lifecycle events       | Medium | Close/minimize handling | Partial -- Electron has `closed` |
| P1       | Plan 5: Script injection              | Low    | Cleaner init            | Yes -- Electron has preload      |
| P1       | Plan 6: Permission handling           | Low    | Security policy         | Parity -- neither has it         |
| P2       | Plan 1: Desktop module (file dialogs) | Medium | Open/save workflows     | No -- neither has it             |
| P2       | Plan 4: smartview interop             | High   | RPC performance         | No -- Electron uses IPC          |
| P2       | Plan 7: PDF export                    | Medium | Document export         | No -- neither has it             |
| P3       | Plan 9: Navigation telemetry          | Low    | Debugging               | No -- neither has it             |

## Known Issues

### Process Lifecycle Mismatch with Electron

**Problem:** The saucer process lifecycle does not match Electron's behavior in
two ways:

1. **Ctrl-C bldr does not kill bldr-saucer.** When the Go process is
   interrupted with Ctrl-C, the Electron process exits (because the IPC socket
   closes and Electron's `sock.on('end', ...)` handler calls `process.exit(0)`).
   The saucer C++ process is left running as an orphan.

2. **Quitting bldr-saucer does not restart the controller.** When the Electron
   window is closed, the controller detects it and gets restarted by
   controllerbus, which re-launches Electron. When the saucer window is closed
   (or the C++ process exits), the controller does not get restarted -- the
   saucer process stays gone.

**Expected behavior (matching Electron):**

- Ctrl-C bldr should also terminate bldr-saucer (signal forwarding or process
  group management).
- Quitting bldr-saucer should cause the saucer controller to exit with an
  error, triggering controllerbus to restart it and re-launch the C++ process.

**Investigation needed:**

- How does the Go saucer controller handle subprocess exit? Does `cmd.Wait()`
  propagate the error back to `Execute()`?
- Is the Go process forwarding SIGINT/SIGTERM to the C++ child process?
- Does the C++ process detect when the Unix domain socket closes and exit
  cleanly?
- Compare with Electron's `sock.on('end')` / `sock.on('error')` handlers in
  `web/electron/main/app.ts:165-175` which call `process.exit()`.
