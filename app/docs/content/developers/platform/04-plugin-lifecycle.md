---
title: Plugin Lifecycle
section: platform
order: 4
summary: Root plugin, dependency declaration, manifest selection, download, and execution.
---

## What This Is

A plugin moves through a defined lifecycle from declaration in `bldr.yaml` to running code serving RPC requests. Understanding this lifecycle is essential for building plugins that integrate cleanly with the Spacewave runtime.

## How It Works

### Declaration

Plugins are declared as manifest entries in the bldr project configuration. Each manifest specifies a builder (compiler type), entry points, Go packages, web package dependencies, and a config set with controller configurations. The builder compiles the plugin into a content-addressed manifest block.

Controllers are registered via `configSet` entries in the manifest, not by calling `AddFactory` directly. At build time, bldr scans the declared Go packages for `NewFactory`/`BuildFactories` functions and generates a `plugin.go` with a `Factories` array. At runtime, the plugin registers all factories and deserializes the config set, matching each entry's ID to a factory's `ConfigID`.

### Manifest Storage

The compiled manifest is stored as a content-addressed block in the world's block-DAG. The manifest block is linked from the plugin's root object key using a `<manifest>` graph predicate. The plugin host scheduler discovers manifests by traversing these links.

### Discovery and Download

When a space loads, the plugin host scheduler controller reads the space settings for required plugin IDs. For each plugin, it:

1. Resolves the manifest block reference from the world's graph.
2. Selects the best manifest revision for the available plugin hosts (matching platform IDs).
3. Fetches the manifest blocks with configurable concurrency and backoff.
4. Copies the manifest to the local store if needed.

### Execution

Once the manifest is available locally, the scheduler calls `ExecutePlugin` on the appropriate plugin host. The plugin host:

1. Extracts the plugin distribution files (compiled code, assets).
2. Creates a sandbox (Web Worker for TypeScript, WASM instance for Go).
3. Establishes a bidirectional starpc RPC channel between the plugin and the host.
4. The plugin receives a `BackendAPI` handle and registers its capabilities.

For TypeScript plugins, the backend entry point is an async function:

```typescript
export default async function backend(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  // Register object types, viewers, services
}
```

For Go plugins, the entry point is a set of controller factories that are instantiated from the config set.

### Teardown

When a plugin is removed from space settings or the space is unmounted, the scheduler releases the plugin directive. The plugin's context is canceled, the sandbox is torn down, and all registered capabilities are unregistered. Returning an error from the backend function triggers retry with configurable backoff.

## When to Create a Separate Plugin

Create a separate plugin when the module has large dependencies that would bloat the main application bundle. Lightweight viewers and services should be merged into the main application plugin instead.

## Why It Matters

The plugin lifecycle determines how quickly a plugin loads, how it recovers from errors, and how cleanly it shuts down. Plugins that register capabilities eagerly and handle cancellation properly provide the best user experience. Plugins that leak resources or ignore the abort signal cause visible degradation.

## Next Steps

- [Manifest Hosting and Scheduling](/docs/developers/platform/manifest-hosting-and-scheduling) for the scheduler's platform selection and fetch logic.
- [What are Plugins](/docs/developers/plugins/what-are-plugins) for the conceptual overview.
