---
title: Manifests and Plugin Lifecycle
section: plugins
order: 2
summary: Understand how a Manifest becomes loaded plugin behavior in a Space.
---

A Manifest describes plugin code and resources. The PluginHost uses manifests to select, fetch, and run plugin packages.

Space settings decide which plugin IDs belong to a Space. The PluginHost then resolves those IDs to manifests and loads the corresponding runtime code.

## Lifecycle Shape

The lifecycle is:

1. A plugin is built and described by a Manifest.
2. The Manifest is stored or published through the configured path.
3. A Space includes the plugin ID in its settings.
4. The PluginHost loads the plugin.
5. The app registers viewers, Quickstarts, or resources exposed by that plugin.

## Operational Risk

Plugin lifecycle is a runtime boundary. Loading, failing, retrying, and removing a plugin changes what a Space can render. Keep those behaviors owned by the plugin and host path rather than scattering caller-side fallbacks.

Use `spacewave plugin list` to inspect plugin state for a Space.
