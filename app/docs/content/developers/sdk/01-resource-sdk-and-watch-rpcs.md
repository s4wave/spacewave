---
title: Resource SDK and Watch RPCs
section: sdk
order: 1
summary: Hold server objects through Resource SDK handles, release them on time, and follow changing state with watch RPCs.
---

The Resource SDK is how Spacewave app code talks to the Spacewave backend. The
server hands out resource IDs, one for each object the client holds, such as a
Space or a file. The TypeScript client wraps each ID in a typed class with
methods for that object. Use it whenever app code reads or changes backend
state, and release each handle when you are done with it.

## How it works on the wire

`ResourceService` has three calls:

- `ResourceClient` opens a session with the server, called a generation, and
  carries the `Adopt` and `Release` messages in order.
- `ResourceRpc` routes RPC streams to a resource.
- `ResourceAttach` passes resources the client created to server calls.

The first local reference to a resource sends `Adopt`. The last reference sends
`Release` on the same `ResourceClient` stream. A child resource the server
returns stays pending under its parent until the client adopts it. Releasing a
parent also releases its pending children. Closing the stream releases every
resource in that generation.

## TypeScript resources

Typed SDK classes extend `Resource`. Each one builds its service clients from
`resourceRef.client` and exposes typed methods. Release it with `release()` or
`[Symbol.dispose]`.

Declare each resource you create with `using`, or clean it up in a hook:

```ts
using root = new Root(rootRef)
using child = root.getResourceRef().createResource(id, SomeHandle)
```

## React hooks

Use `useResource` when a component needs a resource handle or a one-time async
load. Register handles that need releasing with the `cleanup` callback passed
to the factory.

Use `useStreamingResource` for watch RPCs. It subscribes to an `AsyncIterable`
and updates its value on each result. When the parent or the dependencies
change, it stops the previous stream.

Use the root-resource hooks that track `connectionGeneration`. After a
reconnect, they drop the old resources and create new ones.

## When to watch

Use a watch RPC for state that can change from another tab, a CLI command, the
background service, or a plugin. Current examples include Canvas state, Chat
messages, Git worktree status, billing state, UnixFS directory entries, and
session sync status.

Use a unary `Get*` call for a one-time read or for data that never changes.
Always pass on the `AbortSignal` you receive from the hook or the caller.
