---
title: Hooks and Patterns
section: sdk
order: 2
summary: React hooks and common patterns for building plugin UIs.
---

## useResource

`useResource` is the fundamental hook for managing resource lifecycle in React. It creates a resource when the component mounts, releases it on unmount, and recreates it when dependencies change:

```typescript
const handle = useResource(
  useCallback(async (signal, cleanup) => {
    const ref = await api.accessRoot(signal)
    cleanup(() => ref.release())
    return ref
  }, [api]),
)
```

The hook returns a `Resource<T>` with `loading`, `error`, and `value` fields. The callback receives an `AbortSignal` that fires on cleanup and a `cleanup` function for registering release handlers.

## useStreamingResource

`useStreamingResource` watches a streaming RPC and yields the latest value:

```typescript
const state = useStreamingResource(
  handle,
  useCallback(async function* (h, signal) {
    for await (const resp of h.watchState({}, signal)) {
      yield resp.state
    }
  }, []),
)
```

The generator runs for the lifetime of the parent resource. When the parent reconnects, the stream restarts automatically. Always forward the `AbortSignal` into the underlying RPC call to prevent leaked streams.

## useWatchStateRpc

`useWatchStateRpc` is a convenience hook for the common pattern of watching a single streaming RPC:

```typescript
const value = useWatchStateRpc(rpc, request, requestEq, responseEq)
```

It handles request deduplication (only restarts the stream if the request changes per `requestEq`) and response deduplication (only triggers re-renders if the response changes per `responseEq`).

## State Management

Spacewave uses `Resource<T>` as the primary state container. Avoid raw `useState` + `useEffect` for async operations. The resource hooks handle cleanup, retry, and cascading updates automatically.

For UI-only state that should persist across page reloads, use `useStateAtom` from `@s4wave/web/state/persist.js`:

```typescript
const ns = useStateNamespace(['myPlugin'])
const [mode, setMode] = useStateAtom<'list' | 'grid'>(ns, 'mode', 'list')
```

## Common Patterns

**Typed handle access**: Use `useAccessTypedHandle` to get a typed SDK handle for a specific object in a space:

```typescript
const handle = useAccessTypedHandle(
  worldState,
  objectInfo.objectKey,
  GitRepoHandle,
)
```

**Mapped resources**: Use `useMappedResource` to derive a value from a resource without creating a new stream:

```typescript
const title = useMappedResource(
  handle,
  useCallback((h) => h.getTitle(), []),
)
```

**Avoid raw useEffect**: Raw `useEffect` + `useState` for async operations is an anti-pattern in this codebase. Use the provided hooks instead. Raw `useEffect` is acceptable only for DOM side effects (focus, scroll, event listeners) that do not involve data loading.
