---
title: Typed Collection Apps
section: plugins
order: 2
summary: Run a typed collection app as a Space plugin, update its data safely, and add collections to an existing plugin.
---

A typed collection app keeps its shared data in validated collections and
changes that data through named mutations. The same app definition runs in a
Node server with the [sync
library](/docs/developers/sync/build-a-live-application) and in a Space as a
plugin. A Space is the container for one project in Spacewave, and its data is
stored in a World, a database of typed objects.

In a Bldr project, import the SDK from
`@go/github.com/s4wave/spacewave/sdk/sync/index.js`.

## Declare the plugin

Declare the collections and mutation handlers with `defineApp`. Then pass that
definition to `definePlugin` in the plugin's backend entrypoint:

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

While the backend runs, `definePlugin` registers the app's ObjectType, its
World operations, its viewer, and the optional Quickstart. The Quickstart
creates the app object and its initial records in one transaction.

The ObjectType ID is `sync/app/<schema.id>`, and `appObjectTypeID(schema)`
returns it. The Space keeps objects of this type while the plugin is unloaded.
Collection records and plugin manifests are stored as separate World objects.

## Which plugin version runs

Each app object records the exact plugin manifest it runs on. Accepted
operations run that manifest's code, even after a newer version is installed.
If that manifest is no longer available, the operation fails with an error.
Keep old manifests installed while app data or accepted operations still use
them.

When a new plugin version starts, the scheduler waits for it to finish
starting. Then it switches the ObjectTypes, operations, viewer, and Quickstart
to the new version together. If startup fails, the previous version and its
viewer stay available. Spaces that are already open see the new registrations
without reopening.

## Build the viewer

In the viewer, open the app with `useAppAttachment(worldState, colors,
objectKey)` from `@s4wave/web/sync/useAppAttachment.js`. Render live results
with `useAppQuery` and submit mutations with `useAppMutation`, both from
`@s4wave/web/sync/app-hooks.js`. The attachment uses the signed-in session of
the open Space. Closing the viewer stops its queries and releases its handles.

Keep personal state, such as the current selection or search text, in
component state. Keep shared state, such as the votes, in collections.

The `spacewave-colors` manifest and the `plugin/colors` sources contain the
complete example.

## Trust and determinism

Plugin code is trusted. Its mutations get data limits and cooperative
cancellation, but no memory sandbox and no forced interruption. A mutation must
be deterministic: it may use only its input and the transaction it is given.

## Update an app object

Installing a new plugin version does not change existing app objects. To move
an object to the new version, call:

```ts
upgradeAppInstance(nextApp, engine, nextRevision, objectKey, from, {
  requestId,
})
```

`from` is the object's current version record. If another client updated the
object first, the call fails instead of overwriting that update. Keep `from`
and `requestId` so you can retry the same update safely. The call returns a new
attachment at the new version.

When the schema version is unchanged, the update validates every existing
record before switching versions. Viewers attached to the old version then
report a version conflict.

To change the schema, increase `schema.version` and pass migrations as the
third argument to `defineApp`. Each key is the stored version that the
migration converts from:

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

`previous` returns the old records as unvalidated JSON, so parse them with the
old validators. Writes through `collection` use the new validators.

The migration commits as one transaction. The new records, the new version, and
the retry record are saved together or not at all. Collections that the new
schema drops are removed only after the migration succeeds. If it fails, the
object stays usable at the old version. Installing older plugin code does not
undo a migration, and moving an object to a lower schema version fails.

## Add collections to an existing plugin

A plugin that already serves its own Resource mux can add collections with
`createAppPluginOperations(api, app, signal)`:

1. Route World operations to `operations.handler` when `operations.handles(id)`
   returns true.
2. Install the mux, then call `operations.register(rootRef, retain)`.
3. Release the registration refs when the backend stops.

The host also starts workers for older manifests, to run operations on objects
that still use them. Such a worker must serve these handlers but skip the
plugin's current registrations, such as its ObjectTypes and viewer.
`(await operations.info).historical` is true in that worker.

Notes uses this approach for saved views (`notes/saved-views`). The first save
creates the app object and the saved view in one transaction. The view picker
uses the same query and mutation hooks. Choosing a saved view loads it into the
personal filter draft, and only **Save** writes the draft back. Notebooks,
notes files, and personal state keep their existing storage.

## Watch any object

To watch a typed projection of any existing object, use
`watchObjectQuery(engine, objectKey, query, signal)` from the plugin SDK. In
React, use `useObjectQuery(worldState, objectKey, query)` from
`@s4wave/web/sync/useObjectQuery.js`.

The query function receives a read-only snapshot of the object. Each result
comes from a single World revision, and edits to other objects do not rerun the
query. Abort the signal to stop watching. The hook stops watching on unmount
and clears its data when its World, object key, or query changes.
