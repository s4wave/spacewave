---
title: Project Configuration
section: platform
order: 1
summary: Bldr project config, plugins, build targets, and publish configs.
---

## Overview

Every Spacewave project is defined by a bldr project configuration file (`bldr.yaml`). This file declares the project ID, manifests (plugins and application bundles), build targets, remote destinations, and publish pipelines. Bldr reads this config to orchestrate compilation, bundling, and deployment across browser, desktop, and CLI targets.

## Project Config Structure

The `ProjectConfig` protobuf message (defined in `bldr/project/project.proto`) is the schema for `bldr.yaml`. Its top-level fields are:

| Field | Type | Purpose |
|-------|------|---------|
| `id` | string | Project identifier (valid DNS label). Used for storage paths and bundle filenames. |
| `start` | StartConfig | Configuration for `bldr start` commands. |
| `manifests` | map | Manifest ID to builder configuration. Each manifest produces a plugin or app bundle. |
| `build` | map | Named build targets combining manifests with platform IDs. |
| `remotes` | map | Destination definitions for deploying manifests. |
| `publish` | map | Build + publish pipelines to remotes. |
| `extends` | list | Go module paths of other bldr projects to inherit config from. |

## Manifests

A manifest entry defines a single buildable unit (a plugin, the main app, or a CLI tool). Each manifest has:

- **builder** - A `ControllerConfig` specifying the compiler (e.g., `bldr/plugin/compiler/js` for TypeScript plugins, `bldr/plugin/compiler/go` for Go plugins).
- **rev** - Minimum manifest revision number. Incrementing this forces a rebuild even when source has not changed. `bun fecheck` bumps this automatically for frontend manifests.
- **description** - Human-readable label for the manifest.

The builder config contains compiler-specific settings like entry points, Go packages, web packages, and config set entries.

## Start Config

The `start` section configures `bldr start` commands:

- **plugins** - List of plugin IDs to load on startup.
- **disable_build** - Skips running manifest builders during startup.
- **load_web_startup** - Path to a `.tsx` file with a React component used as the app shell. The component should contain a `<WebView />` from `@aptre/bldr-react`.

## Build Targets

Build targets combine manifests with platforms. A target specifies:

- **manifests** - Which manifest IDs to build.
- **platform_ids** - Platform identifiers (e.g., `js/wasm`, `linux/amd64`).
- **targets** - Deployment targets: `browser`, `desktop`, `desktop/{os}/{arch}`.

Example usage from package.json:

```bash
bun run build              # debug build for "app" target
bun run build:release:web  # release build for "release-web" target
```

## Remotes

A remote defines a destination for deploying built manifests. It includes:

- **host_config_set** - ConfigSet applied to the devtool bus for accessing the world.
- **engine_id** - World engine ID to deploy to.
- **peer_id** - Peer ID for signing world transactions.
- **object_key** - Root object key for manifest storage.

A default remote named `devtool` is automatically available for local development.

## Publish Config

Publish entries orchestrate multi-step deployment: gather manifests from source object keys, select platforms, and push to destination remotes. Publish configs support storage transform overrides for adjusting block encoding per-manifest.

## Config Inheritance

The `extends` field references other bldr projects by Go module path (e.g., `github.com/s4wave/spacewave`). The extended project's `bldr.yaml` is resolved via the `vendor/` directory. Configs are merged in order, with the local config taking precedence.

## Next Steps

- [Web Entrypoint and Runtime](/docs/developers/platform/web-entrypoint-and-runtime) for how the browser runtime boots from the project config.
- [Plugin Lifecycle](/docs/developers/platform/plugin-lifecycle) for how manifests become running plugins.
