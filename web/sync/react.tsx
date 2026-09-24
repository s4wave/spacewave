import { createContext, use, useMemo, type ReactNode } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Database, SubscriptionState } from '../../sdk/sync/client.js'
import { SyncError } from '../../sdk/sync/errors.js'
import type { Output, Query, Schema } from '../../sdk/sync/schema.js'

// createSyncContext creates typed React bindings for one sync application schema.
// The schema argument binds collection names and validator output types at compile
// time; it carries no runtime state. SyncProvider supplies an already-connected
// Database and never opens, retries, or closes it: the connection lifecycle stays
// with the caller that owns the db.
export function createSyncContext<S extends Schema>(_schema: S) {
  const SyncContext = createContext<Database<S> | null>(null)

  // SyncProvider makes the supplied connected db available to useDatabase and
  // useCollection below it in the tree.
  function SyncProvider({
    db,
    children,
  }: {
    db: Database<S>
    children: ReactNode
  }) {
    return <SyncContext.Provider value={db}>{children}</SyncContext.Provider>
  }

  // useDatabase returns the Database supplied by the nearest SyncProvider.
  function useDatabase(): Database<S> {
    const db = use(SyncContext)
    if (!db) {
      throw new Error(
        'useDatabase must be called within the SyncProvider returned by createSyncContext',
      )
    }
    return db
  }

  // useCollection streams the named collection and returns its latest
  // SubscriptionState. The subscription restarts only when db, name, or the
  // query prefix changes, so inline query objects do not resubscribe per render.
  // Unmount aborts this watch's signal; the stream itself publishes stale and
  // error transitions from the db's own reconnect handling.
  function useCollection<K extends keyof S['collections'] & string>(
    name: K,
    query: Query = {},
  ): SubscriptionState<Output<S['collections'][K]>> {
    const db = useDatabase()
    const prefix = query.prefix ?? ''
    // Memoized ready parent: the db is already connected, so the parent Resource
    // carries no loading, error, or retry of its own.
    const parent = useMemo<Resource<Database<S>>>(
      () => ({ value: db, loading: false, error: null, retry: () => {} }),
      [db],
    )
    const resource = useStreamingResource(
      parent,
      (db, signal) => db.collection(name).watch({ prefix }, { signal }),
      [db, name, prefix],
    )
    if (resource.loading) return { status: 'loading', data: [] }
    if (resource.value) return resource.value
    if (resource.error) {
      return {
        status: 'error',
        data: [],
        error:
          resource.error instanceof SyncError
            ? resource.error
            : new SyncError(
                'UNAVAILABLE',
                'The collection subscription failed',
              ),
      }
    }
    return { status: 'loading', data: [] }
  }

  return { SyncProvider, useDatabase, useCollection }
}
