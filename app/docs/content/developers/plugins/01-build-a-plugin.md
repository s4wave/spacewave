---
title: Build a Plugin
section: plugins
order: 1
summary: Build a plugin by owning its manifest, runtime registrations, viewer, and verification path.
---

A Spacewave plugin is packaged as a Bldr Manifest and loaded by PluginHost. The
plugin can register resources, ObjectTypes, viewers, Quickstarts, and host
controllers depending on its compiler config.

## Project config

Bldr project config owns manifest definitions. A project has an ID, start
plugins, manifest configs, build configs, remotes, publish settings, and a
list of other projects it extends. A manifest config wraps a builder
controller config plus revision and description.

Use the compiler that matches the runtime:

- JS/TS plugin config can build backend WebWorker modules and frontend WebView
  modules through Vite or esbuild.
- Go plugin config scans explicit Go packages for controller factories, applies
  `config_set` on the plugin bus, and can apply `host_config_set` on the plugin
  host bus.
- Web plugin config covers renderer or native app packaging.

## App registration path

For a typed app feature, the plugin should register the ObjectType, the viewer,
and any Quickstart it owns. Dynamic viewers are appended after base and product
viewers. Exact type registrations beat prefix and wildcard fallbacks.

If the feature creates a Space on first run, return the required plugin IDs and
index path from the Quickstart execution so SpaceSettings names the plugin and
opens the right object.

## Local verification

Use Bldr to build the manifest, then import or deploy it through the CLI when
testing against a running Space:

```sh
spacewave plugin import-manifest --db ./.bldr --manifest-id <id>
spacewave plugin add <manifest-id> --space <space-id>
spacewave plugin list --space <space-id>
```

`plugin import-manifest` imports a built manifest into the local plugin host
store. `plugin add` writes the manifest ID into Space settings. `plugin list`
shows loaded or loading state.
