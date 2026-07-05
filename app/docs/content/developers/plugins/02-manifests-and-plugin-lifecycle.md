---
title: Manifests and Plugin Lifecycle
section: plugins
order: 2
summary: Understand Manifest fields, PluginHost retention, RPC routing, and status states.
---

A Manifest is the versioned package that PluginHost can load. It names one
target architecture and points at the files needed to run the plugin.

## Manifest fields

`ManifestMeta` contains:

- `manifest_id`
- `build_type`
- `platform_id`
- `rev`
- `description`

`Manifest` contains that metadata, the entrypoint path inside the dist
filesystem, a dist filesystem ref, and an assets filesystem ref. `ManifestRef`
pairs matching metadata with the stored object ref.

## Startup and preflight

The devtool startup path preflights startup manifests that are both listed in
project start plugins and present in the project manifests map. Native and web
startup paths start the plugin scheduler, attach plugin status, start PluginHost,
then start the project startup controllers.

## Loading lifecycle

`PluginHost.LoadPlugin` is a streaming RPC. The plugin remains loaded while that
RPC is active. Multiple loads of the same plugin ID are deduplicated. An
`instance_key` creates independent instances for the same plugin ID.

Plugin status states are `UNKNOWN`, `REQUESTED`, and `RUNNING`. Status also
includes the last error message and timestamp when available.

## RPC and filesystems

`PluginRpc` forwards RPC streams to another plugin by plugin ID or plugin ID plus
instance key. `PluginFsRpc` exposes plugin assets and dist filesystems through
component IDs such as `plugin-assets`, `plugin-dist`, `plugin-assets/{plugin}`,
and `plugin-dist/{plugin}`.

Each plugin runs on its own bus. Separate plugin packages mean separate buses.
Cross-plugin behavior should use RPC or host config, not shared process state.
