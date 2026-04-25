---
title: What are Plugins
section: plugins
order: 1
summary: Plugins extend Spacewave with new object types and capabilities.
---

## Plugin Model

A plugin is an isolated program that extends Spacewave with new object types, viewers, and services. Plugins are declared in `bldr.yaml` with a builder configuration that specifies the compiler, entry points, and shared package dependencies. The runtime loads plugins on demand when a space contains objects of a type the plugin handles.

Plugins communicate with the host runtime through bidirectional streaming RPC (starpc). A plugin backend receives a `BackendAPI` handle and an `AbortSignal`, registers its capabilities, and then serves requests for the lifetime of the space.

## What Plugins Can Do

Plugins register **object types** and **viewers**. An object type defines a category of data and how it is stored in the space's world state. A viewer is a React component that renders objects of a given type in the UI.

Plugins can also implement **world operation handlers** to define custom mutations on space data, and **services** that expose RPC methods to other plugins or to the frontend.

The file browser, Git viewer, and Canvas use the same registration model available to plugins.

## Plugin Isolation

Each plugin runs in its own sandbox. TypeScript plugins execute in a dedicated Web Worker. Go plugins compile to WebAssembly and run in their own WASM instance. Plugins cannot access data outside their assigned space without explicit permission grants.

Communication between plugins and the host is constrained to the starpc RPC interface. A plugin cannot read memory from other plugins or from the core runtime. This isolation model prevents a misbehaving plugin from affecting the rest of the system.

## Built-in vs Third-Party Plugins

Built-in plugins ship with Spacewave and are declared in the main `bldr.yaml` manifest. Built-in plugins are pre-approved and load automatically when a space requires them.

Third-party plugins are distributed as content-addressed manifests stored in the block-DAG. Installing a third-party plugin requires approval. The approval state is tracked per-space in `SpaceSettings`.

## Plugin Lifecycle

A plugin's lifecycle follows this sequence:

1. The `plugin/space` controller watches `SpaceSettings` for changes to `plugin_ids`.
2. When a new plugin ID appears, the controller issues a `LoadPlugin` directive.
3. The runtime fetches the plugin manifest, validates it, and instantiates the plugin binary.
4. The plugin backend receives `BackendAPI`, registers object types and viewers via RPC, and begins serving.
5. When the plugin is removed from `SpaceSettings`, its directive is released and the process terminates.

Returning `nil` from the backend function signals clean shutdown. Returning an error triggers retry with backoff.
