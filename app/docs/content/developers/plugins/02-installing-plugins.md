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

If the plugin has not been previously approved, it enters a pending state. Approve it with:

```bash
spacewave-cli plugin approve spacewave-notes
```

Once approved, the plugin binary is fetched, instantiated, and begins serving.

## Managing Installed Plugins

List all plugins in a space and their current state:

```bash
spacewave-cli plugin list --watch
```

This displays each plugin's ID, approval status (approved, pending, denied), and whether it is currently loaded. The `--watch` flag streams updates as the state changes.

View detailed space information including installed plugins:

```bash
spacewave-cli space info <space-id>
```

## Plugin Permissions

Plugin approval is per-space. Approving a plugin in one space does not affect other spaces. The approval state is stored in `SpaceSettings` and synchronized across devices.

Built-in plugins are pre-approved and do not require manual approval. Third-party plugins require explicit approval before they can load. This prevents untrusted code from executing without user consent.

## Removing a Plugin

Remove a plugin from a space:

```bash
spacewave-cli plugin remove <plugin-id>
```

This removes the plugin's ID from `SpaceSettings.plugin_ids`. The `plugin/space` controller releases the `LoadPlugin` directive, which terminates the plugin process. Data created by the plugin remains in the space's world state; only the plugin code is unloaded.

To deny a plugin (prevent it from loading even if added):

```bash
spacewave-cli plugin deny <plugin-name>
```
