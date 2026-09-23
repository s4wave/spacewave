# Spacewave API reference

Spacewave provides typed live collections backed by durable storage. An
application declares a schema of collections and mutations. Clients hold one
WebSocket connection and read full snapshots that update live. A Node server
stores the data, authenticates each connection, and checks access on every
operation.

Package entries:

- `spacewave`: schema, client, and error API for browsers and Node.
- `spacewave/server`: `createServer` for Node applications.
- `spacewave/react`: optional React bindings.

The server requires Node 24.15 or later, below 25. The React bindings require
React 19.2 or later, below 20.

## Schema

```ts
import { defineSchema } from 'spacewave'
import { z } from 'zod'

const todo = z.object({ title: z.string().min(1).max(120), done: z.boolean() })

export const schema = defineSchema({
  id: 'task-board',
  version: 1,
  collections: { todos: todo },
  mutations: {
    completeTodo: {
      input: z.object({ id: z.string().min(1) }),
      output: todo,
    },
  },
})
```

`defineSchema(schema)` checks that `id` is nonempty and `version` is a safe
integer from 1 to 4294967295, then returns the schema with its inferred types.
Collection and mutation values are Standard Schema v1 validators; zod is one
compatible library. `Input<V>` and `Output<V>` infer a validator's input and
output types.

Every stored value is JSON. A write must be JSON-compatible: null, booleans,
finite numbers, strings, arrays, and plain objects. `undefined`, `bigint`,
symbols, functions, non-finite numbers, cycles, and class instances are
rejected. Object keys are stored in a canonical sorted form, so the same value
always produces the same bytes.

The server validates each write against the validator and stores the
validator's output form. Reads and watch snapshots return that stored form, so
client types use `Output<V>`.

## Shared application definitions

`defineApp(schema, handlers)` bundles a schema with the code for its named
mutations. It does not open storage, so the same definition can serve a Node
server and a Space plugin. Spread it into `createServer`:

```ts
import { defineApp } from 'spacewave'

export const app = defineApp(schema, {
  async completeTodo({ collection }, { id }) {
    const todos = collection('todos')
    const todo = await todos.get(id)
    if (!todo) throw new Error('Todo is unavailable')
    const completed = { ...todo, done: true }
    await todos.put(id, completed)
    return completed
  },
})

const server = await createServer({
  ...app,
  directory: './data',
  authenticate,
  authorize,
})
```

The optional `instance` setting in the server configuration picks a dataset
within the World. It defaults to `schema.id`. Each app instance and authorized
scope has its own collection records and its own record of accepted requests.

The Space plugin SDK reads the same definition through `definePlugin` and
`attachApp`. An attachment implements `AppSource`, which offers read-only
collections, named mutations, and function queries. Mutations run inside the
World transaction that the host provides. A mutation must be deterministic: it
may use only its input and that transaction, and it must await every
collection call before it returns.

Plugin code is trusted. Transaction and data limits and cooperative
cancellation bound its work, but they are not a sandbox.

### Function queries in a Space or Node host

A function query is a JavaScript function that reads collections and returns a
value. Declare one with `defineQuery(schema, { name, input, evaluate })`:

```ts
import { defineQuery } from 'spacewave'

const incomplete = defineQuery(schema, {
  name: 'incomplete-todos',
  input: z.null(),
  async evaluate({ collection }) {
    return (await collection('todos').scan())
      .filter(({ value }) => !value.done)
  },
})

for await (const snapshot of attachment.watch(incomplete, null, signal)) {
  if (snapshot.status === 'current') render(snapshot.value)
}
```

While the function runs, the runtime records what it read: the records it
looked up, the records it found missing, the prefixes it scanned, and the
fields it used. Each result comes from one World snapshot. When the World
changes, the runtime compares storage roots to skip unchanged data, then checks
whether any changed record affects what the query read. Only then does it run
the query again. If the old roots are no longer available, it runs the query
again in full.

A query function must await every read before it returns. It must not change
its input or the records it reads, cause outside effects, or depend on the
clock or random numbers. A query that returns a whole record depends on every
field of that record.

Identical queries on the same authorized attachment share one evaluation. It
stops when the last subscriber closes.

On a Node host, `server.as(principal)` implements the same `AppSource`. Call its
`watch` method with a query definition, or pass it to the React hooks below.
Each delivery checks collection permissions again. The scoped object starts no
query work until you read from its iterator. Closing the last iterator releases
the source, and closing the server stops every active source.

`spacewave/react` exports `useAppQuery(source, query, args)` and
`useAppMutation(source, name)`. A query reports pending, current, or error. A
mutation reports idle, pending, accepted, or error, and provides
`submit(input)` and `retry()`. `retry()` resends the original input with the
original request ID, so the server applies it at most once. Keep personal
selection and unsaved drafts in component state.

The WebSocket `Database` client below subscribes to collection prefixes. It
does not run function queries.

## Client

```ts
import { connect } from 'spacewave'

const db = await connect({
  url: 'ws://localhost:8787/sync',
  schema,
  getAccessToken: () => 'local-demo',
})
```

`connect(options)` returns `Promise<Database<S>>`. It resolves after
authentication, the wire and schema version check, and the server's acceptance
of the connection. On failure it closes the connection and throws `SyncError`.

`ConnectOptions<S>`:

| Field | Contract |
| --- | --- |
| `url` | WebSocket URL without credentials, query, or fragment. |
| `schema` | The application schema. |
| `getAccessToken(signal)` | Returns the token or a promise of it. Called for each connection attempt. |
| `signal?` | Aborting closes the client. |
| `requestTimeoutMs?` | Per-request timeout. Default 15000. |

`Database<S>`:

| Member | Contract |
| --- | --- |
| `connection` | `ConnectionStatus`: `current` state plus `subscribe(listener)`, which returns a disposer. |
| `collection(name)` | Typed access for one schema collection name. |
| `mutate(name, input, options?)` | Runs a declared mutation and returns its declared output type. |
| `close()` | Idempotent. Also available through `await using` (`AsyncDisposable`). |

`ConnectionState` is `{ status: 'ready' | 'reconnecting' | 'closed', error? }`.
A new client is `reconnecting` until the server accepts the connection, and
`closed` after `close()`.

One collection exposes:

| Member | Contract |
| --- | --- |
| `get(key, options?)` | `Promise<Output \| undefined>`. |
| `scan(query?, options?)` | One full snapshot of the prefix, ordered by key. |
| `put(key, value, options?)` | Writes one record. `value` is the validator's input type. |
| `delete(key, options?)` | Removes one record. |
| `watch(query?, options?)` | `AsyncIterable<SubscriptionState<Output>>`. |
| `subscribe(query, observer, options?)` | Pushes states to `observer.next`; `observer.error?` sees failures. Returns an unsubscribe function. |

`CallOptions` is `{ signal?, requestId? }`. Forward `AbortSignal` into every
call; cancellation propagates to the server.

A record key is nonempty UTF-8 within 1024 bytes.

### Subscriptions

`SubscriptionState<T>` is `{ status, data, error? }` where `data` is a readonly
array of `{ key, value }` entries:

| Status | Meaning |
| --- | --- |
| `loading` | No snapshot has arrived yet. |
| `current` | The connection is live and `data` is its latest received server snapshot. |
| `stale` | The connection was lost; `data` is the last known snapshot. |
| `error` | The subscription ended; `error` says why. |

`Query` is `{ prefix?: string }`. An empty prefix covers the whole collection.

Each state carries the full snapshot of every record matching the prefix,
ordered by key. There are no per-row deltas and no per-row filters. The client
reconnects on its own: a lost connection publishes `stale` and the stream
resumes with `current` once the server accepts the connection again. An
unrecoverable failure publishes `error` and ends the stream.

```ts
const unsubscribe = db.collection('todos').subscribe({}, {
  next: (state) => console.log(state.status, state.data.length),
})
```

### Uncertain writes

Every write carries a request ID. The client generates one when a call omits
`requestId`; the server accepts 1 to 128 characters.

The client retries a write once when a call fails with a recoverable
connection error, using the same request ID. If acceptance still cannot be
confirmed, the call throws `SyncError('UNCERTAIN', ..., requestId)`. The write may or may not have
been accepted. Retry the exact same call with the same request ID and input:
the server recognizes the duplicate and returns the original result. Reusing a
request ID with different input throws `CONFLICT`.

Receipts of accepted writes persist in each scope's dataset for the dataset
lifetime, so a retry stays idempotent across restarts.

The client keeps no offline queue and applies no optimistic writes. If the server is unreachable, operations fail with `UNAVAILABLE`; a write that may have been sent can fail with `UNCERTAIN`. The application decides when to retry. Rendered state comes from server snapshots.

```ts
import { SyncError } from 'spacewave'

const requestId = crypto.randomUUID()
try {
  await db.collection('todos').put(id, todo, { requestId })
} catch (error) {
  if (error instanceof SyncError && error.code === 'UNCERTAIN') {
    // Same request ID, same input.
    await db.collection('todos').put(id, todo, { requestId })
  } else {
    throw error
  }
}
```

## Errors

`SyncError` extends `Error` with a stable `code` and, for writes, the
`requestId`.

| Code | Meaning |
| --- | --- |
| `VALIDATION` | Input, key, or configuration is invalid. |
| `AUTHENTICATION` | The token or principal was rejected, expired, or retired. |
| `DENIED` | `authorize` returned false. |
| `MISSING_COLLECTION` | The collection is not declared in the schema. |
| `SCHEMA_MISMATCH` | Application ID, schema version, or wire version differs. |
| `QUERY_LIMIT` | A record, snapshot, or scan limit was exceeded. |
| `CONFLICT` | A request ID was reused with different input. |
| `UNAVAILABLE` | The connection or storage is temporarily unavailable. |
| `UNCERTAIN` | A write's acceptance could not be confirmed. Retry the same request ID and input. |
| `STORAGE` | Durable storage failed. |
| `CLOSED` | The client or server is closed. |

## React

```ts
import { createSyncContext } from 'spacewave/react'

const { SyncProvider, useDatabase, useCollection } = createSyncContext(schema)
```

`createSyncContext(schema)` binds collection names and validator output types
at compile time. The schema argument carries no runtime state.

| Binding | Contract |
| --- | --- |
| `SyncProvider({ db, children })` | Supplies an already-connected `Database<S>`. It never opens, retries, or closes the db; the application does that. |
| `useDatabase()` | Returns the typed `Database<S>`. Throws when no provider is above in the tree. |
| `useCollection(name, query?)` | Returns `SubscriptionState<Output>` for the collection. |

The subscription restarts only when the db, the collection name, or the query
prefix changes, so an inline query object does not resubscribe on every render.
While a subscription loads or restarts, the state is `loading`; the previous
query's data is not shown as current. Unmounting a component aborts its watch
signal. Closing the db remains the application's job, for example on
`pagehide`.

```tsx
function TodoList() {
  const todos = useCollection('todos')
  if (todos.status === 'loading') return <p>Loading tasks…</p>
  if (todos.status === 'error') return <p>{todos.error?.message}</p>
  return (
    <ul>
      {todos.data.map((entry) => (
        <li key={entry.key}>{entry.value.title}</li>
      ))}
    </ul>
  )
}

createRoot(document.getElementById('app')!).render(
  <SyncProvider db={db}>
    <TodoList />
  </SyncProvider>,
)
```

## Server

```ts
import { createServer } from 'spacewave/server'

const server = await createServer({
  directory: './data',
  schema,
  authenticate: async (token) => {
    if (token !== 'local-demo') throw new Error('unknown token')
    return { subject: 'demo', scope: 'demo' }
  },
  authorize: ({ collection }) => collection === 'todos',
  mutations: {
    completeTodo: async (context, input) => {
      const todo = await context.collection('todos').get(input.id)
      if (!todo) throw new Error('unknown task')
      const updated = { ...todo, done: true }
      await context.collection('todos').put(input.id, updated)
      return updated
    },
  },
})
```

`createServer(options)` returns `Promise<SyncServer<S, P>>`. It opens the
dataset, checks the stored application ID and version, runs any migration, and
only then returns a server that can accept callers. Failures throw `SyncError`.

`ServerOptions<S, P>`:

| Field | Contract |
| --- | --- |
| `directory` | Dataset directory. This is the public storage option. |
| `schema` | The application schema. |
| `authenticate(token, signal)` | Returns `Promise<Principal>`. Called once per connection after the wire and schema version check. Throwing rejects the connection; each connection gets one attempt. |
| `authorize(access)` | Returns `boolean \| Promise<boolean>`. Called for every operation and again for every watch delivery. |
| `mutations` | One handler per declared mutation. |
| `limits?` | `maxRecords` (default 10000), `maxSnapshotBytes` (default 8 MiB), `maxRecordBytes` (default 256 KiB). Exceeding a bound fails that query or write with `QUERY_LIMIT`. |
| `migration?` | Runs before the server accepts any caller when the stored schema version differs. |

Storage uses built-in SQLite in a dedicated worker. A directory permits one open server writer at a time.

`Principal` is `{ subject, scope, expiresAt?, signal? }`. `subject` and `scope`
must be nonempty, with `subject` within 256 UTF-8 bytes. `expiresAt` is a Unix
millisecond time; the connection retires at that time. Aborting `signal`
retires the connection.

`authorize` receives `{ principal, scope, collection, action }` where `action`
is `'read'` or `'write'`. Returning false rejects the operation with `DENIED`.
Authorization is per collection and action; queries cover a whole collection
or a prefix. There are no per-row grants or filters.

A mutation handler receives `(context, input)` and returns the mutation
output's input type. `context` provides `collection(name)` with `get`, `scan`,
`put`, and `delete` inside the same transaction, plus `principal`, `scope`,
and `signal`. The server validates the handler's input and returned output
against the schema and stores the output form.

### Migration

```ts
migration: {
  from: 1,
  async run({ scopes, scope }) {
    for (const name of await scopes()) {
      const todos = scope(name).collection('todos')
      for (const entry of await todos.scan()) {
        await todos.put(entry.key, { title: entry.value.title, done: false })
      }
    }
  },
}
```

`Migration<S>` is `{ from, run(context) }`. `run` receives `scopes()` listing
the stored scopes, `scope(name)` returning a transaction for that scope, and
`signal`. The migration runs inside one maintenance transaction before the
server accepts any caller. If the stored version differs and no matching migration is
supplied, or the dataset belongs to a different application ID, `createServer`
fails with `SCHEMA_MISMATCH`.

### Hosting and lifetime

| Member | Contract |
| --- | --- |
| `listen(options?)` | Opens its own HTTP server. `ListenerOptions` adds `port?` (default 8787) and `host?` (default 127.0.0.1). Returns `Listener` with a `ws://` `url` and `close()`. |
| `attach(http, options?)` | Handles the upgrade event of a supplied HTTP server. Your server keeps handling other routes and upgrade paths. Returns `Attachment` with `close()`. |
| `as(principal)` | `DatabaseAccess<S> & AppSource<S>` for server-side reads, named mutations, and function queries as that principal. Every operation still passes `authorize`. |
| `admin(scope)` | Privileged access as subject `admin`, with only the base principal fields. Handlers that require application-specific principal fields should use `as(principal)`. Collection authorization is bypassed; validation still applies. |
| `close()` | Closes every attachment the server created, then the application, then storage the server opened. Idempotent. Also available through `await using`. |

`AttachmentOptions` for both forms:

| Field | Contract |
| --- | --- |
| `path?` | Upgrade path. Default `/sync`. |
| `allowedOrigins?` | Exact Origin values to accept. |
| `maxRequestBytes?` | Maximum WebSocket message size. Default 1 MiB. |

Origin handling: a request without an Origin header, such as a non-browser
client, is accepted. With `allowedOrigins` set, the Origin must be one of the
listed values. Otherwise the Origin must match the request's own Host header
with an `http` or `https` scheme. Upgrades with a query string are refused.

```ts
import { createServer as createHTTPServer } from 'node:http'

const listener = await server.listen({ port: 8787 })
console.log(listener.url)

const http = createHTTPServer((request, response) => response.end())
server.attach(http, { path: '/sync', allowedOrigins: ['https://example.com'] })
http.listen(3000)
```

## Limits and performance

Queries return complete, ordered snapshots of a collection or prefix. Defaults bound a query to 10,000 records and 8 MiB of encoded keys and values, and a record to 256 KiB. Exceeding a bound produces `QUERY_LIMIT`. These are safety bounds, not throughput guarantees.

A qualification workload on Node 24.21.0 and macOS arm64 used 1,000 numeric records, three subscribers, and ten writes. Identical queries shared one evaluation.

| Measurement | Observed sample |
| --- | --- |
| Write acceptance, p50 / p95 | 234 / 242 ms |
| Delivery to all subscribers, p50 / p95 | 947 / 1,041 ms |
| Initial bulk seeding | 13.57 seconds |
| RSS when ready / after the workload | 336 MB / 1.10 GB |
| Encoded snapshot | 28.9 KB |

In this workload, each changed revision makes a collection-prefix subscription scan its whole collection. A function query first compares storage roots and the fields it read, and runs again only when they changed. RSS includes the compiled runtime and is not a retained-heap measurement. These observations describe one workload; measure representative scan and delivery time, memory, and storage growth before raising limits. Acceptance receipts and World history accumulate for the dataset lifetime.

A qualified release artifact's `qualification.json` records its own sample, source revision, package sizes, and checksum. See the [README](README.md) for supported runtimes and current release scope.
