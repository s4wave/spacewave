---
title: State Atoms and Persistence
section: internals
order: 5
summary: State atom architecture, backend-backed tiers, and persistence model.
---

## Overview

Spacewave uses a tiered state atom system to manage UI state that persists across page reloads, syncs across browser windows, and optionally syncs to the backend. The system is built on a simple `Atom<T>` interface and integrated into React via `useSyncExternalStore`. State atoms avoid raw `useEffect` + `useState` patterns in favor of purpose-built hooks.

## Atom Types

Three atom implementations serve different persistence needs:

**BasicAtom** - In-memory only, lost on page reload. Used for tab-local transient state.

**StorageAtom** - Backed by `localStorage` with cross-window sync via `StorageEvent`. Values are serialized with superjson (supports Date, Map, Set). Used for state that should survive reloads and sync between windows.

**DerivedAtom** - Read-only computed view of another atom. Useful for projections without duplicating source data.

## Root Atoms

Two root atoms defined in `web/state/global.ts` anchor the state tree:

```typescript
// Tab-local, not synced
export const localStateAtom: Atom<StateType> = atom<StateType>({})

// Persistent, synced across windows via localStorage
export const persistentStateAtom: Atom<StateType> =
  atomWithLocalStorage<StateType>('app-persistent', {})
```

## Namespaced State

State atoms are organized into namespaces using `StateNamespaceProvider` and `useStateNamespace`. Namespaces form a path hierarchy that scopes state to specific UI regions. For example, an object viewer at key `abc123` with a git sub-viewer would have the path `['objectViewer', 'abc123', 'git']`.

```typescript
const gitNs = useStateNamespace(['git'])
const [viewMode, setViewMode] = useStateAtom<'files' | 'readme'>(
  gitNs, 'viewMode', 'files'
)
```

The namespace context is passed down the React tree. Child components inherit the parent namespace and extend it with their own segments.

## Backend-Backed State

When a `StateAtomAccessor` is available in context, `useStateAtom` transparently switches from the in-memory atom tree to backend-persisted storage. Each atom becomes a separate `StateAtom` resource identified by a store ID derived from the namespace path plus key.

The `useBackendStateAtom` hook in `web/state/useBackendStateAtom.tsx` handles this:

1. Creates a `StateAtom` resource for the store ID via the accessor.
2. Watches for state changes via a streaming `WatchState` RPC.
3. Parses the superjson-encoded state from the response.
4. Provides a `setValue` callback that sends updates back to the backend.

Backend atoms are synchronized across all connected clients. When one tab updates a value, all other tabs receive the update through the streaming RPC.

## useStateAtomResource

For direct access to a backend `StateAtom` resource without the namespace system, use `useStateAtomResource`:

```typescript
const stateAtomResource = useResource(rootResource, async (root, signal, cleanup) =>
  root ? cleanup(await root.accessStateAtom({}, signal)) : null, [])
const [uiState, setUiState] = useStateAtomResource(stateAtomResource, { tabs: [] })
```

This hook uses `useWatchStateRpc` internally and returns a `[value, setValue]` tuple similar to `useState`.

## Interaction Tracking

The `web/state/interaction.ts` module tracks whether the user has interacted with the app via a `spacewave-has-interacted` localStorage flag. The prerender and hydration system uses this to decide whether to auto-boot the WASM runtime for return visitors.

## Next Steps

- [SDK Hooks and Patterns](/docs/developers/sdk/hooks-and-patterns) for the full hook reference.
- [Resource System](/docs/developers/sdk/resource-system) for how resources and streaming RPCs work.
