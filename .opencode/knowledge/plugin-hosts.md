# Plugin Host Architecture

**Date:** 2025-12-29

## Overview

Plugin hosts are responsible for executing plugins in their target runtime environment. The plugin host scheduler matches plugins to appropriate hosts based on platform ID, then delegates execution to the selected host.

## Interface

The `PluginHost` interface (`plugin/host/host.go`) defines the contract:

```go
type PluginHost interface {
    // GetPlatformId returns the platform ID this host can execute
    GetPlatformId() string
    
    // Execute runs an optional global management goroutine
    Execute(ctx context.Context) error
    
    // ListPlugins lists loaded plugins
    ListPlugins(ctx context.Context) ([]string, error)
    
    // ExecutePlugin executes a plugin with RPC setup
    ExecutePlugin(ctx, pluginID, entrypoint, pluginDist, pluginAssets, hostMux, rpcInit) error
    
    // DeletePlugin clears cached plugin data
    DeletePlugin(ctx context.Context, pluginID string) error
}
```

## Current Plugin Hosts

| Host | Location | Platform ID | Environment | Description |
|------|----------|-------------|-------------|-------------|
| **ProcessHost** | `process/process.go` | `native/{os}/{arch}` | Native | OS processes via `exec.Command` |
| **WazeroQuickJsHost** | `wazero-quickjs/quickjs.go` | `js` | Native | QuickJS WASI via Wazero |
| **WebHost** | `web/web.go` | `native/js/wasm` | Browser | Go WASM in SharedWorkers |
| **WebQuickJSHost** | `web/web-quickjs.go` | `js` | Browser | QuickJS WASI in SharedWorkers |

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Plugin Host Scheduler                       │
│  (Matches plugins to hosts based on platform ID)                    │
└────────────────────┬────────────────────────────────────────────────┘
                     │
     ┌───────────────┼───────────────┬────────────────┐
     ▼               ▼               ▼                ▼
┌─────────┐   ┌───────────────┐  ┌─────────┐   ┌──────────────┐
│Process  │   │WazeroQuickJs  │  │ Web     │   │WebQuickJS    │
│Host     │   │Host           │  │ Host    │   │Host          │
├─────────┤   ├───────────────┤  ├─────────┤   ├──────────────┤
│Platform:│   │Platform: "js" │  │Platform:│   │Platform: "js"│
│native/* │   │               │  │native/  │   │              │
│         │   │               │  │js/wasm  │   │              │
├─────────┤   ├───────────────┤  ├─────────┤   ├──────────────┤
│Native   │   │Native only    │  │Browser  │   │Browser only  │
│processes│   │(Wazero WASM)  │  │Workers  │   │(QuickJS WASI)│
└─────────┘   └───────────────┘  └─────────┘   └──────────────┘
     ▲               ▲               ▲                ▲
     │               │               │                │
     └───── Native Builds ──────────┘└──── Browser Builds ────┘
         (!js && !wasip1)              (js || wasip1 || wasm)
```

## Host Details

### ProcessHost

**File:** `plugin/host/process/process.go`

Executes native binary plugins as OS processes:

- Syncs plugin dist to disk via `unixfs_sync.Sync()`
- Creates plugin state and dist directories
- Starts process via `exec.Command` with `exec-plugin` argument
- Uses Unix socket (pipesock) for IPC
- Establishes yamux connection for bidirectional RPC
- Kills process on context cancellation

**IPC Flow:**
```
Plugin Process ←→ Unix Socket ←→ Yamux ←→ Host RPC
```

### WazeroQuickJsHost

**File:** `plugin/host/wazero-quickjs/quickjs.go`

Runs JS plugins in QuickJS WASI via Wazero (Go WASM runtime):

- Compiles QuickJS WASM once (shared via refcount)
- Mounts plugin dist at `/dist`, assets at `/assets`
- Mounts boot harness at `/boot/plugin-quickjs.esm.js`
- Uses stdin for yamux input, `/dev/out` for yamux output
- Boot harness creates `BackendApiImpl` for plugin

**WASI Mounts:**
```
/boot   - Boot harness (plugin-quickjs.esm.js)
/dist   - Plugin distribution files
/assets - Plugin assets
/dev    - Device files (/dev/out for yamux output)
```

**RPC Flow:**
```
Plugin (QuickJS) ←→ stdin/dev-out ←→ Yamux ←→ Host RPC
```

### WebHost

**File:** `plugin/host/web/web.go`

Runs Go WASM plugins in browser SharedWorkers:

- Creates SharedWorker via `WebDocument.CreateWebWorker()`
- Worker loads plugin from HTTP path (`/b/p/{pluginID}/{entrypoint}`)
- Uses standard SharedWorker (`shw.mjs`) for Go WASM
- RPC via WebRuntimeClient message passing

**Browser Flow:**
```
WebDocument → CreateWebWorker() → SharedWorker (shw.mjs)
                                       ↓
                              Plugin.mjs (Go WASM)
                                       ↓
                              BackendApiImpl
                                       ↓
                              WebRuntimeClient ←→ WebRuntime
```

### WebQuickJSHost

**File:** `plugin/host/web/web-quickjs.go`

Runs JS plugins in QuickJS WASI in browser SharedWorkers:

- Creates SharedWorker with `WorkerType: WEB_WORKER_TYPE_QUICKJS`
- Worker uses QuickJS SharedWorker (`shw-quickjs.mjs`)
- Loads QuickJS WASM from `/b/qjs/qjs-wasi.wasm`
- Boot harness provides `BackendApiImpl` to plugin
- Yamux over stdin/`/dev/out` bridged to WebRuntimeClient

**Browser Flow:**
```
WebDocument → CreateWebWorker(QUICKJS) → SharedWorker (shw-quickjs.mjs)
                                              ↓
                                         QuickJS WASM
                                              ↓
                                         Boot Harness
                                              ↓
                                         Plugin.mjs
                                              ↓
                                         BackendApiImpl
                                              ↓
                              Yamux (stdin/dev-out) ←→ WebRuntimeClient
```

## Build Tag Selection

Host registration is controlled by build tags in `plugin/host/default/`:

**Native Builds** (`plugin-host-process.go`):
```go
//go:build !js && !wasip1

PluginHostControllerFactories = []{
    ProcessHost,
    WazeroQuickJsHost,
}
```

**Browser Builds** (`plugin-host-web.go`):
```go
//go:build js || wasip1 || wasm

PluginHostControllerFactories = []{
    WebHost,
}
```

**Note:** `WebQuickJSHost` is not in default factories. It's manually instantiated in `devtool/web/entrypoint/controller/controller.go`.

## RPC Architecture

All hosts establish bidirectional RPC via starpc:

1. **Host → Plugin**: `rpcInit(pluginRpcClient)` provides client to call plugin
2. **Plugin → Host**: `hostMux` receives calls from plugin

The RPC service is registered on the bus with a pattern matching the plugin:
```go
regexp.MustCompile("^" + regexp.QuoteMeta("web-worker/" + pluginID) + "$")
```

## Key Files

| File | Purpose |
|------|---------|
| `plugin/host/host.go` | PluginHost interface |
| `plugin/host/controller/controller.go` | Generic host controller wrapper |
| `plugin/host/scheduler/controller.go` | Schedules plugins to hosts |
| `plugin/host/default/*.go` | Default host registration |
| `plugin/host/process/` | Native process host |
| `plugin/host/wazero-quickjs/` | Native QuickJS host |
| `plugin/host/web/web.go` | Browser Go WASM host |
| `plugin/host/web/web-quickjs.go` | Browser QuickJS host |

## Platform ID Matching

The scheduler matches plugins to hosts by platform ID:

| Plugin Platform | Native Host | Browser Host |
|-----------------|-------------|--------------|
| `native/linux/amd64` | ProcessHost | - |
| `native/darwin/arm64` | ProcessHost | - |
| `native/js/wasm` | - | WebHost |
| `js` | WazeroQuickJsHost | WebQuickJSHost |
