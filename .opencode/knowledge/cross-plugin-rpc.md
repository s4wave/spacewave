---
created: 2026-02-20
---

# Cross-Plugin RPC Communication

## Overview

Plugins communicate via SRPC proxied through the plugin host scheduler.
The topology is **star (hub-and-spoke)** -- all cross-plugin calls route
through the host. There is no direct mesh connectivity between plugins.

**Critical:** Each plugin has its own controller bus. Directives on one
bus CANNOT reach another plugin's bus. Cross-plugin communication MUST
use SRPC (via the Resource SDK or direct service calls).

## Service ID Prefix Scheme

Defined in `plugin/host.go`:

| Prefix | Routes To | Example |
|--------|-----------|---------|
| `plugin-host/` | Host scheduler | `plugin-host/my.Service` |
| `plugin/{id}/` | Another plugin | `plugin/spacewave-core/s4wave.resource.ResourceService` |
| `host-volume/` | Host volume proxy | `host-volume/VolumeService` |
| `plugin-dist/` | Plugin dist FS | `plugin-dist/unixfs.rpc.FSCursorService` |
| `plugin-assets/` | Plugin assets FS | `plugin-assets/unixfs.rpc.FSCursorService` |

## Cross-Plugin Call Flow

```
Plugin A bus
  -> LookupRpcService("plugin/B/my.Service")
    -> entrypoint controller matches prefix
    -> WaitPluginClient("B")
      -> ExPluginLoadWaitClient("B")
        -> LoadPlugin directive -> host loads B
      -> builds rpcstream.NewRpcStreamClient(PluginHost.PluginRpc, "B")
    -> clientForwardingInvoker strips "plugin/B/" prefix
    -> SRPC stream tunnels through PluginHost.PluginRpc
      -> Host proxies to B via HandleProxyRpcStream
        -> B's PluginServer.PluginRpc routes to B's bus
          -> B's controllers handle the call
```

## Key Files

| File | Purpose |
|------|---------|
| `plugin/host.go` | Service ID prefix constants |
| `plugin/res-lookup-rpc-service.go` | LookupRpcService resolver, clientForwardingInvoker |
| `plugin/res-lookup-rpc-client.go` | LookupRpcClient resolver |
| `plugin/host/plugin-host-server.go` | PluginHostServer.PluginRpc proxy handler |
| `plugin/plugin-server.go` | PluginServer.PluginRpc incoming handler |
| `plugin/entrypoint/controller/controller.go` | Plugin entrypoint, BuildRemotePluginClient |
| `plugin/entrypoint/common.go` | ExecutePluginEntrypoint setup |
| `plugin/host/scheduler/controller.go` | Scheduler, buildPluginMux, LoadPlugin handling |
| `plugin/dir-load-plugin.go` | LoadPlugin directive, ExPluginLoadWaitClient |
| `plugin/forward-rpc-service/controller.go` | ForwardRpcService controller |

## Getting an SRPC Client to Another Plugin (Go)

From within a plugin's Go code:

```go
// Option 1: Via directive (recommended)
client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, bus, "target-plugin-id", nil)
defer ref.Release()
// client is an srpc.Client connected to the target plugin

// Option 2: Via entrypoint controller
client := entrypointCtrl.BuildRemotePluginClient("target-plugin-id", false)
```

## PluginRpc Bidirectional Stream

The `PluginRpc` RPC is the core cross-plugin tunnel:
- `plugin.proto` defines `Plugin.PluginRpc(stream RpcStreamPacket) returns (stream RpcStreamPacket)`
- The init packet's `componentID` identifies the target (or source) plugin
- `rpcstream.HandleProxyRpcStream` on the host copies packets between two plugin streams
- Entire SRPC sessions are nested inside a single PluginRpc stream

## WebPlugin Cross-Plugin Forwarding

The web plugin controller (`web/plugin/controller/controller.go`) supports:
- `HandleRpcViaPlugin(pluginID, serviceID)` - forward RPC service to another plugin
- `HandleWebViewViaPlugin(pluginID, webViewIDRegex)` - forward web views
- `HandleWebPkgViaPlugin(pluginID, webPkgID)` - forward web packages

These create `ForwardRpcService` controllers that proxy matching calls.

## webPkg Sharing Between Plugins

Defined in `web/bundler/bundler.proto` as `WebPkgRefConfig`:

```protobuf
message WebPkgRefConfig {
  string id = 1;
  bool exclude = 2;  // true = consumer (don't bundle, another plugin provides)
  repeated string imports = 3;
}
```

- **Provider** plugin: declares webPkg without `exclude` (default false)
- **Consumer** plugin: declares webPkg with `exclude: true`
- Merge logic in `web/bundler/web-pkg-refs.go`: if any ref marks excluded, stays excluded

Example in bldr.yaml:
```yaml
# Provider (alpha/spacewave-web):
webPkgs:
  - id: '@s4wave/sdk'

# Consumer:
webPkgs:
  - id: '@s4wave/sdk'
    exclude: true
```

## Plugin Bus Isolation

Each plugin runs `ExecutePluginEntrypoint()` which creates an independent bus:
- Own controller registry
- Own directive system
- Services registered on the plugin mux are accessible externally via PluginRpc
- The entrypoint controller handles LookupRpcService/LookupRpcClient with prefix routing
