---
title: Manifest Hosting and Scheduling
section: platform
order: 5
summary: Plugin host scheduling across browser, native, and web runtimes.
---

## Overview

The plugin host scheduler is the controller that bridges plugin manifests and plugin hosts. It manages which manifests are fetched, selects the best host for each plugin based on platform compatibility, handles download with retry, and orchestrates plugin execution. A single scheduler instance coordinates all plugins for a given world engine.

## Scheduler Configuration

The scheduler is configured via the `plugin.host.scheduler.Config` protobuf message:

| Field | Purpose |
|-------|---------|
| `engine_id` | World engine ID to attach to |
| `object_key` | Root object key to search for `<manifest>` links |
| `peer_id` | Peer ID for signing world operations |
| `volume_id` | Volume on the plugin host bus for plugin storage |
| `watch_fetch_manifest` | Watch the FetchManifest directive while a plugin runs |
| `disable_store_manifest` | Skip storing fetched manifests (shared world) |
| `disable_copy_manifest` | Skip copying manifests to the host world bucket |
| `fetch_concurrency` | Max concurrent block fetches per manifest |
| `fetch_backoff` | Backoff config for manifest fetch retries |
| `exec_backoff` | Backoff config for plugin execution retries |

## Plugin Hosts

A plugin host is a runtime environment capable of executing plugins for a specific platform. Each host reports its platform ID and implements the `PluginHost` interface:

```go
type PluginHost interface {
    GetPlatformId() string
    Execute(ctx context.Context) error
    ListPlugins(ctx context.Context) ([]string, error)
    ExecutePlugin(ctx, pluginID, instanceKey, entrypoint string,
        pluginDist, pluginAssets *unixfs.FSHandle,
        hostRpcMux srpc.Mux, rpcInit PluginRpcInitCb) error
    DeletePlugin(ctx context.Context, pluginID string) error
}
```

Built-in host implementations include:

| Host | Platform ID | Runtime |
|------|-------------|---------|
| Web Worker | `js/web` | Browser Web Worker (TypeScript plugins) |
| WASM | `js/wasm` | Browser WASM (Go plugins compiled to WASM) |
| Process | `{os}/{arch}` | Native process (desktop Go plugins) |
| wazero-quickjs | `js/quickjs` | QuickJS in wazero (lightweight JS execution) |

## Manifest Selection

When the scheduler needs to run a plugin, it selects the most appropriate manifest by:

1. Traversing `<manifest>` graph links from the configured `object_key`.
2. Collecting all manifest revisions for the plugin ID.
3. Filtering manifests by platform compatibility with available hosts.
4. Selecting the highest revision number that matches an available host.

If `watch_fetch_manifest` is enabled, the scheduler also monitors the `FetchManifest` directive for newly available manifests and hot-swaps to newer revisions.

## Fetch Pipeline

Manifest blocks are fetched with configurable concurrency. The fetch proceeds breadth-first: fetch a block, discover its references, fetch those references. The concurrency limit bounds the number of in-flight block fetches at any time.

Fetch failures are retried with exponential backoff configured via `fetch_backoff`. If `disable_copy_manifest` is false, fetched manifest blocks are copied to the plugin host's world bucket for local caching.

## Plugin Execution

Once a manifest is locally available, the scheduler calls `ExecutePlugin` on the host:

1. The host extracts the plugin distribution (compiled code) and assets (CSS, images) from the manifest's UnixFS tree.
2. The host creates the execution sandbox and establishes a starpc RPC channel.
3. The `rpcInit` callback is called when the RPC client is ready, providing the host-side service mux.
4. The host RPC mux exposes volume access, resource management, and WebView services to the plugin.

If execution fails, the scheduler retries with `exec_backoff`. If the plugin returns cleanly (nil error), it is considered shut down.

## Plugin References

External code interacts with running plugins through the `PluginHostScheduler` interface:

```go
ref, release := scheduler.AddPluginReference(pluginID, instanceKey)
defer release()
```

The reference keeps the plugin running for as long as at least one reference exists. When all references are released, the plugin is eligible for shutdown.

## Next Steps

- [Plugin Lifecycle](/docs/developers/platform/plugin-lifecycle) for the higher-level lifecycle from declaration to teardown.
- [Project Configuration](/docs/developers/platform/project-configuration) for how manifests are declared in `bldr.yaml`.
