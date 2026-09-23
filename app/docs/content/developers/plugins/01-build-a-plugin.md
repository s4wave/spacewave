---
title: Build a Plugin
section: plugins
order: 1
summary: Set up a plugin project, register what it adds to a Space, and test it against a running Space.
---

A plugin adds new kinds of items to a Space, the container that holds one
project in Spacewave. A plugin can register ObjectTypes, viewers, Quickstarts,
resources, and host controllers. An ObjectType is the code that reads and
changes one kind of object. A viewer displays it. A Quickstart creates a new
Space with starter content.

Plugins are built with Bldr, Spacewave's build system and plugin runtime. Bldr
packages a plugin as a manifest: a versioned bundle of compiled code and
assets. PluginHost, the part of Bldr that runs plugins, loads the manifest.

This page covers the project setup and the registrations every plugin needs.
The next pages cover [typed collection
apps](/docs/developers/plugins/typed-collection-apps), [building and editing a
plugin inside a Space](/docs/developers/plugins/build-and-edit-in-a-space),
and [how manifests load](/docs/developers/plugins/manifests-and-plugin-lifecycle).

## Project config

A Bldr project config defines the manifests to build. A project has an ID,
start plugins, manifest configs, build configs, remotes, publish settings, and
a list of other projects it extends. Each manifest config holds a builder
controller config, a revision, and a description.

Choose the compiler that matches where the plugin runs:

- The JS/TS plugin compiler builds backend modules that run in a WebWorker and
  frontend modules that run in the page, with Vite or esbuild.
- The Go plugin compiler scans the Go packages you list for controller
  factories. It applies `config_set` on the plugin's bus and can apply
  `host_config_set` on the plugin host's bus.
- The web plugin compiler packages renderers and native apps.

## Register what the plugin adds

For a new kind of item, the plugin registers its ObjectType, its viewer, and
any Quickstart for it.

Viewers from plugins are added after the base and product viewers. A viewer
registered for an exact type wins over one registered for a prefix or for all
types.

If the plugin's Quickstart creates a Space, return the required plugin IDs and
the index path from the Quickstart. Spacewave saves them in the Space settings,
so the Space loads the plugin and opens the right object.

## Test against a running Space

Build the manifest with Bldr, then import it and add it to a Space with the
CLI:

```sh
spacewave plugin import-manifest --db ./.bldr --manifest-id <id>
spacewave plugin add <manifest-id> --space <space-id>
spacewave plugin list --space <space-id>
```

`plugin import-manifest` copies a built manifest into the local plugin store.
`plugin add` writes the manifest ID into the Space's settings. `plugin list`
shows whether each plugin is loading or loaded.
