---
title: Resource SDK and Watch RPCs
section: sdk
order: 1
summary: Use Resource SDK handles, cleanup, and watch RPCs for mutable runtime state.
---

The Resource SDK is the handle-based RPC layer used by Spacewave app code. A
server returns resource IDs. The TypeScript client wraps those IDs in resource
references and typed resource classes.

## Wire model

`ResourceService` exposes:

- `ResourceClient` to create a client handle and root resource ID;
- `ResourceRpc` to route RPC streams to resource handles;
- `ResourceRefRelease` to release handles;
- `ResourceAttach` to attach client-owned resources to server calls.

Ending the ResourceClient stream releases the resources opened by that client.

## TypeScript resources

Typed SDK classes extend `Resource`. They construct service clients from
`resourceRef.client`, expose typed methods, and release their ref through
`release()` or `[Symbol.dispose]`.

Use `using` or hook cleanup for every resource you create:

```ts
using root = new Root(rootRef)
using child = root.getResourceRef().createResource(id, SomeHandle)
```

## React hooks

Use `useResource` when a component needs a resource handle or one-shot async
load. Register disposable handles with the `cleanup` callback passed into the
factory.

Use `useStreamingResource` for watch RPCs. It subscribes to an `AsyncIterable`,
updates value on each yield, and aborts the previous stream when the parent or
dependencies change.

Use root-resource hooks that track `connectionGeneration` so reconnects drop and
recreate stale resource trees.

## When to watch

Use a watch RPC for mutable state that can change from another tab, CLI command,
daemon process, or plugin. Current examples include Canvas state, Chat messages,
Git worktree status, billing state, UnixFS directory entries, and session sync
status.

Use unary `Get*` calls for one-shot reads or immutable data. Always pass the
`AbortSignal` you receive from the hook or caller.
