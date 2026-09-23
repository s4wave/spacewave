---
title: Manifests and Plugin Lifecycle
section: plugins
order: 4
summary: What a manifest contains, how PluginHost starts and keeps a plugin running, and how plugins call each other.
---

This page explains how a built plugin gets loaded and kept running, so you can
tell why a plugin is or is not running and how plugins talk to each other.

A manifest is the versioned package that PluginHost loads. PluginHost is the
part of Bldr that runs plugins. Each manifest targets one platform and points
at the files the plugin needs to run.

## Manifest fields

`ManifestMeta` contains:

- `manifest_id`
- `build_type`
- `platform_id`
- `rev`
- `description`

`Manifest` contains that metadata, the path of the entrypoint inside the dist
filesystem, a reference to the dist filesystem, and a reference to the assets
filesystem. `ManifestRef` pairs the metadata with a reference to the stored
manifest.

## Startup

At startup, the devtool checks each manifest that is both listed in the
project's start plugins and present in the project's manifests. The native and
web clients start the plugin scheduler, attach plugin status, and start
PluginHost. Then they start the project's startup controllers.

## Loading and running

`PluginHost.LoadPlugin` is a streaming RPC. The plugin stays loaded while that
RPC is open. Several loads of the same plugin ID share one running plugin. To
run separate copies of the same plugin, give each load its own
`instance_key`.

A plugin's status is `UNKNOWN`, `REQUESTED`, or `RUNNING`. When available, the
status also includes the last error message and when it happened.

## Calls between plugins

`PluginRpc` forwards RPC streams to another plugin, chosen by plugin ID or by
plugin ID and instance key. `PluginFsRpc` serves a plugin's assets and dist
files under component IDs such as `plugin-assets`, `plugin-dist`,
`plugin-assets/{plugin}`, and `plugin-dist/{plugin}`.

Each plugin runs on its own bus, the message channel its controllers share.
Separate plugin packages have separate buses. For behavior that spans plugins,
use RPC or host config, not shared process state.
