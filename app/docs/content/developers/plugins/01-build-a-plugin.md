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

## Typed collection apps

The collection SDK shares declarations with the Node sync library. Import it
from `@go/github.com/s4wave/spacewave/sdk/sync/index.js` in a Bldr project.
Define the collection validators and named mutation handlers with `defineApp`,
then adapt the declaration to a backend entrypoint:

```ts
import { definePlugin } from '@go/github.com/s4wave/spacewave/sdk/sync/index.js'
import { colors } from './app.js'

export default definePlugin({
  app: colors,
  displayName: 'Color votes',
  iconName: 'palette',
  quickstart: {
    objectKey: 'colors',
    initial: { kind: 'mutate', name: 'initialize', input: null },
  },
  viewer: {
    entry: 'ColorViewer.tsx',
    componentId: 'colors/votes',
  },
})
```

The adapter owns ObjectType, operation, viewer, and optional Quickstart
registrations for the backend lifetime. A Quickstart creates the instance and
its initial records atomically. Each instance records its immutable executable;
accepted operations resolve that exact manifest even after a newer module is
installed. An unavailable historical module fails explicitly. Retain installed
manifest history while application data or accepted operations require it.

The ObjectType ID is `sync/app/<schema.id>`. `appObjectTypeID(schema)` returns
that ID for integrations. The reserved namespace identifies the SDK's JSON
instance binding, so Space storage can retain it while the plugin is unloaded.
Collection records and executable manifests remain separate typed World objects.

`definePlugin` prepares its registrations privately. A replacement worker must
finish startup before the scheduler admits its types, operations, viewer, and
Quickstart together. A startup failure leaves the previous worker and viewer
available. ObjectType handlers retain the registering worker's connection, and
Quickstarts and viewer assets name its immutable manifest. Open type lookups
observe later registrations without reopening the Space.

In the viewer, call `useAppAttachment(worldState, colors, objectKey)` from
`@s4wave/web/sync/useAppAttachment.js`. Use `useAppQuery` and `useAppMutation`
from `@s4wave/web/sync/app-hooks.js` to render live results and submit named
operations. The attachment borrows the mounted Space's authenticated Engine and
releases its queries and retained handles when the viewer closes. Component
state holds personal selection and search; collection records hold shared votes.

The `spacewave-colors` manifest and `plugin/colors` sources provide the complete
declaration and viewer example. Plugin callbacks are trusted code with data
limits and cooperative cancellation. They must use their supplied transaction
and input deterministically; forced interruption and a memory sandbox are not
provided by this adapter.

### Updating an application instance

Installing a module and changing a stored application are separate operations.
Call `upgradeAppInstance(nextApp, engine, nextRevision, objectKey, oldBinding,
{ requestId })` explicitly to adopt the new module. The source binding is a
concurrency precondition. Retain it and the request ID for retries. A compatible
schema revision validates all existing records before updating the binding.
An old attachment reports a version conflict; open a new attachment afterward.

For a schema change, increment `schema.version` and pass migrations as the third
argument to `defineApp`. Each key names the stored version it converts:

```ts
const votes = defineApp(nextSchema, handlers, {
  1: async ({ previous, collection }) => {
    const old = await previous.collection('counts').scan()
    for (const { key, value } of old) {
      await collection('colors').put(key, {
        name: key,
        likes: z.number().int().parse(value),
      })
    }
  },
})
```

`previous` reads bounded original JSON; use the old validators to parse it.
Writes use the new collection validators. World accepts the records, binding,
and retry receipt together, or accepts none of them. Collections absent from
the new schema are removed only after the conversion succeeds. A failed update
keeps the previous instance usable. Rolling back code never reverses a schema
migration; moving backward in schema version is rejected.

### Adding collections to an existing plugin

`createAppPluginOperations(api, app, signal)` exposes the same pinned operation
handler for a plugin that already owns its Resource mux. Route matching World
operations through `operations.handler` when `operations.handles(id)` is true.
Install that mux before calling `operations.register(rootRef, retain)`, and
release the retained registration refs with the backend. A historical worker
must serve its handlers without publishing current registrations; check
`const info = await operations.info`, then inspect `info.historical`.

Notes uses this adapter for `notes/saved-views`. Its first explicit save creates
the application and saved definition in one transaction. The picker uses the
common query and mutation hooks. Loading a definition updates the personal
filter draft; only Save changes writes it back. The existing Notebook protobuf,
UnixFS notes, and personal StateAtoms keep their original storage.

For a typed projection of an existing object, use `watchObjectQuery(engine,
objectKey, query, signal)` from the plugin SDK or `useObjectQuery(worldState,
objectKey, query)` from `@s4wave/web/sync/useObjectQuery.js`. The query receives a
read-only object snapshot. Its value and wait position belong to one World
revision; unrelated object edits skip the callback. Abort the signal to stop a
pending watch. The hook owns cancellation and clears data when its source changes.

### Building source stored in a Space

The Space Plugins panel accepts a UnixFS source object containing a Bldr project,
a manifest ID, and a registered build device. Build captures an immutable source
snapshot and queues a Forge execution on that device. Open the execution's logs
to follow compilation, then install the successful manifest into the open Space.
The device needs the local Bldr toolchain; the build uses the Space's existing
World and plugin host. Editing source does not change a submitted build.

For a source folder containing `app.ts`, `backend.ts`, and `ColorViewer.tsx`,
use this `bldr.yaml`:

```yaml
id: colors
manifests:
  colors:
    builder:
      id: bldr/plugin/compiler/js
      config:
        webPluginId: web
        webPkgs:
          - id: '@s4wave/web'
            exclude: true
        modules:
          - kind: JS_MODULE_KIND_BACKEND
            path: ./backend.ts
            entrypoint: true
          - kind: JS_MODULE_KIND_FRONTEND
            path: ./ColorViewer.tsx
```

Use the public `@go/github.com/s4wave/spacewave/sdk/` imports in `app.ts` and
`backend.ts`, and set the adapter's viewer entry to `ColorViewer.tsx`. The
device supplies the SDK source and compiler dependencies. Declare additional
dependencies in the source project's `package.json`. `exclude: true` makes
the viewer use the open client's shared web package, including its React
contexts; the custom plugin does not register a second copy.

Once the installed plugin registers its Quickstart, **Create _app name_** runs
that action in the current Space. The source project and created app object stay
in the same World. The action uses the mounted Session's existing authority.

Builds store manifests by their content hash and keep provenance beside each
artifact. Two builds at the same numeric revision retain distinct outputs.
Watched rebuilds advance the revision so the scheduler can select the new code.
For Space source, provenance also retains the original source filesystem in the
artifact's World, independently of the editable directory and Forge execution.

### Editing with live preview

In a watched Bldr development client, select the same source, device, and manifest,
then choose **Edit with live preview**. The workbench retains the source editor
and preview together:

1. Select the configured frontend module and an object created by the plugin.
   The plugin's **Create** action is also available beside the preview.
2. Open a UTF-8 source file, choose **Edit file**, and **Save file**. The editor
   saves complete files up to 512 KiB as one atomic replacement and retains
   unsaved text when a save fails. Accepted source changes reach the device's
   retained Vite compiler through the World. Compatible React and CSS edits
   update the mounted preview without replacing its backend or stored data.
3. Close the workbench when finished. This releases the compiler attachment.
   Choose **Build**, then **Install in this Space**, to publish an immutable
   revision for other clients.

Include `@vitejs/plugin-react` in the source project's Vite configuration for
React Refresh. For the Colors example, add this `package.json`:

```json
{
  "type": "module",
  "dependencies": { "zod": "4.3.6" },
  "devDependencies": { "@vitejs/plugin-react": "6.0.5", "vite": "8.2.2" }
}
```

Add `vite.config.ts` with `import react from '@vitejs/plugin-react'` and
`export default { plugins: [react()] }`. Set `viteConfigPaths: [vite.config.ts]`
and `viteDisableProjectConfig: true` in the builder configuration above.

Live React editing requires the development client's renderer;
the workbench explains this requirement before starting a compiler in a release
client. Every preview uses the document's existing React and Refresh runtime.
Its module override applies only inside that preview. Installed viewers keep
loading their immutable artifacts until a new revision is admitted.

The preview uses `space.openPluginFrontend(request, signal)` and Bldr's existing
frontend RPC service. The returned Resource owns the device execution grant;
its Watch owns the live compiler subscription. Release both when an authoring
view closes. A disconnected compiler reports an error with **Restart preview**;
restarting attaches a new compiler session and resets preview component state.

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
