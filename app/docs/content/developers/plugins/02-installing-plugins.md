---
title: Installing Plugins
section: plugins
order: 2
summary: Find and install plugins to extend your spaces.
---

## Plugin Sources

Plugins are identified by manifest IDs stored in the content-addressed block-DAG. Built-in plugins are bundled with Spacewave and available immediately. Third-party plugins are referenced by their manifest block ID and fetched on demand.

## Installing a Plugin

Plugins are added to a space through its settings or via the CLI:

```bash
spacewave-cli plugin add spacewave-notes
```

This adds the plugin's manifest ID to the space's `SpaceSettings.plugin_ids` list. The `plugin/space` controller detects the change and begins the loading process.
The plugin binary is fetched, instantiated, and begins serving once its Manifest
is available from the configured distribution sources.

## Managing Installed Plugins

List all plugins in a space and their current state:

```bash
spacewave-cli plugin list --watch
```

This displays each plugin's ID and whether it is currently loaded. The
`--watch` flag streams updates as the state changes.

View detailed space information including installed plugins:

```bash
spacewave-cli space info <space-id>
```

## Removing a Plugin

Remove a plugin from a space:

```bash
spacewave-cli plugin remove <plugin-id>
```

This removes the plugin's ID from `SpaceSettings.plugin_ids`. The `plugin/space` controller releases the `LoadPlugin` directive, which terminates the plugin process. Data created by the plugin remains in the space's world state; only the plugin code is unloaded.
