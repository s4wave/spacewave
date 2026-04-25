---
title: Resource System
section: sdk
order: 1
summary: The resource lifecycle model that powers Spacewave plugins.
---

## What is a Resource

A resource represents a long-lived connection to a specific capability in Spacewave. Sessions, spaces, storage volumes, and plugin instances are all resources. Each resource is accessed through a `ResourceClient`, a bidirectional starpc stream that persists for the resource's lifetime.

The resource model replaces one-shot request/response patterns with persistent, stateful connections. A plugin that needs access to a space opens a resource handle once and receives streaming updates for as long as the handle is held.

## Resource Lifecycle

Resources follow a create, watch, release lifecycle. Creating a resource establishes the connection and returns a `ClientResourceRef`. The ref provides a typed client for making RPC calls and a `release()` method for cleanup.

```typescript
const ref = await api.accessRoot(signal)
// Use ref.client for RPC calls
// ref.release() when done
```

In React components, `useResource` manages this lifecycle automatically. The hook creates the resource when the component mounts, releases it on unmount, and recreates it when dependencies change.

## Creating Resources

Resources are created through the SDK's access methods. The root resource is the entry point:

```typescript
const rootRef = await api.accessRoot(signal)
```

From the root, access other resources by mounting them:

```typescript
const sessionRef = await rootRef.mountSession(sessionIndex, signal)
const spaceRef = await sessionRef.mountSpace(spaceId, signal)
```

Each access call returns a `ClientResourceRef` that must be released when no longer needed. In React, `useResource` and `useAccessTypedHandle` handle creation and cleanup.

## Resource Dependencies

Resources form a dependency tree. A space resource depends on a session resource, which depends on the root resource. When a parent resource reconnects (for example, after a network interruption), all child resources are automatically torn down and recreated.

This cascading behavior ensures consistency. If a session reconnects, all spaces under it are refreshed with current state rather than continuing with potentially stale data.

## Streaming and Watching

Most resource interactions use streaming RPCs. A `Watch*` RPC returns a server-streaming response that pushes updates as state changes:

```typescript
for await (const resp of spaceService.WatchState({}, signal)) {
  // Handle each state update
}
```

The `useStreamingResource` hook wraps this pattern for React components, yielding the latest value and re-rendering on each update. Streaming RPCs are preferred over one-shot `Get*` RPCs for any state that can change.

## Error Handling

Resource errors propagate through the dependency tree. If a resource encounters an unrecoverable error, it is released and recreated with backoff. Transient errors (network interruptions, temporary unavailability) are handled by the resource system automatically.

In React, the `Resource<T>` type exposes `loading`, `error`, and `value` fields. Components check these fields to render loading states, error messages, or the resolved value. The `useResource` hook handles retry and cleanup internally.
