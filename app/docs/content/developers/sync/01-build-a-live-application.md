---
title: Build a Live Application
section: sync
order: 1
summary: Share a typed schema, run a durable Node server, and subscribe to live data from TypeScript or React.
---

The `spacewave` npm package, also called the sync library, keeps application
data in sync between your Node.js server and your TypeScript clients. You
declare typed collections once. Clients read and write them and receive live
updates. The server stores accepted writes durably and decides who may read or
write each collection.

The package has three entry points:

- `spacewave` holds the browser client and the shared schema helpers.
- `spacewave/server` runs the server on Node.js 24.15 or later, below 25.
- `spacewave/react` adds optional React bindings for React 19.2 or later, below
  20.

## Try the task board

The package includes a task board example. Its `schema.ts` declares the task
records and a `completeTodo` mutation with Standard Schema validators, such as
Zod. The server imports that schema, checks tokens, grants access to
collections, and runs the mutation in one transaction. A plain TypeScript client
and a React client import the same schema and subscribe to the tasks.

```sh
npm install spacewave
cp -R node_modules/spacewave/examples/task-board ./task-board
cd task-board
npm install
npm start
```

Use Node.js 24. Open `http://127.0.0.1:8787` and `http://127.0.0.1:8787/react`.
Add a task in one page and complete it in the other. Restart the server: the
tasks are still there, loaded from its `.data` directory. Replace the example's
local token check before other people use the server.

## Read, write, and subscribe

`connect` resolves once the client has authenticated and the server has
confirmed that both sides use the same schema version. `collection(name)`
returns typed `get`, `put`, `delete`, `scan`, `watch`, and `subscribe`
operations.

A subscription delivers the whole matching set of records, ordered by key,
each time it changes. Each delivery has a status: `loading`, `current`,
`stale`, or `error`. After a reconnect, subscriptions load the server's latest
state again. In React, `createSyncContext(schema)` returns a provider and typed
hooks. Your application still opens and closes the connection.

## Retry writes safely

A successful write means the server has stored it. If the connection drops
before the reply arrives, the call fails with `UNCERTAIN` and gives you its
request ID. Retry with the same request ID and the same input to get the
original result. The server does not apply the write twice.

A named mutation can change several collections in one transaction. Effects
outside Spacewave, such as sending an email, are up to your application.

## Current limits

This release is for small datasets and clients that stay online:

- Queries return complete snapshots with a size limit.
- There is no offline write queue and no durable copy of the data in the
  browser.
- Access is granted per collection, not per record, and there is no SQL
  interface.
- A key prefix narrows what a view receives, but access is still checked for
  the whole collection.
- Changing the schema version requires a migration, which runs before the
  server accepts any connection.

## Where to go next

The package [README](https://github.com/s4wave/spacewave/blob/master/packages/spacewave/README.md)
has setup code and measured limits. Its
[API reference](https://github.com/s4wave/spacewave/blob/master/packages/spacewave/API.md)
documents options, typed errors, access policies, migrations, and shutdown.
`AGENTS.md` in the package is a guide for coding assistants. Each release also
ships a qualification report that identifies the exact package and records its
install, browser, React, persistence, and workload checks.

The same app definition can also run inside a Space as a plugin. See [Typed
Collection Apps](/docs/developers/plugins/typed-collection-apps).
