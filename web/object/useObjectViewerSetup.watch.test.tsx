import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import type { WorldQuery } from '@s4wave/sdk/world/world-query.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { useObjectViewerSetup } from './useObjectViewerSetup.js'

const h = vi.hoisted(() => ({
  exists: false,
  root: 'r1',
  push: undefined as (() => void) | undefined,
  rootResource: { value: {}, loading: false, error: null, retry: () => {} },
}))

// The live watch runs the query on a snapshot each time the test commits.
vi.mock('@s4wave/sdk/world/world-query.js', () => ({
  watchWorldQuery: async function* (
    _engine: unknown,
    query: WorldQuery<unknown>,
    signal: AbortSignal,
  ) {
    for (;;) {
      yield { seqno: 0n, value: await query(snapshot(h.exists), signal) }
      await new Promise<void>((resolve) => {
        h.push = resolve
      })
    }
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  RootContext: { useContext: () => h.rootResource },
}))

vi.mock('@s4wave/web/hooks/useViewerRegistry.js', () => ({
  useAllViewers: () => [],
  getViewersForType: () => [],
}))

// Formatting resolves a tick later, as a Root call does.
vi.mock('@s4wave/sdk/world/object-ref.js', () => ({
  formatObjectRef: async (_root: unknown, ref?: { id?: string }) => {
    await new Promise((resolve) => setTimeout(resolve, 0))
    return 'root://' + (ref?.id ?? '')
  },
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  getObjectType: async () => 'type/doc',
}))

const objectKey = 'world/doc'

// object returns a snapshot's handle for key at the current root.
function object(key: string): IObjectState {
  const id = h.root
  return {
    getKey: () => key,
    getRootRef: async () => ({ rootRef: { id } }),
    [Symbol.dispose]: () => {},
  } as unknown as IObjectState
}

// liveWorld returns a live World whose own reads find the object only when
// found is set, standing in for a read that lands on another revision.
function liveWorld(found: () => boolean): Resource<IWorldState> {
  const live = Object.assign(Object.create(EngineWorldState.prototype), {
    engine: { released: false },
    getEngine: () => ({}),
    getObject: async (key: string) => (found() ? object(key) : null),
  }) as IWorldState
  return { value: live, loading: false, error: null, retry: () => {} }
}

// snapshot returns a World snapshot that holds the object when exists is set.
function snapshot(exists: boolean): IWorldState {
  return {
    getObject: async (key: string) => (exists ? object(key) : null),
  } as unknown as IWorldState
}

// commit sets whether the World holds the object and its root, then wakes
// the watch.
function commit(exists: boolean, root = h.root) {
  h.exists = exists
  h.root = root
  act(() => h.push?.())
}

afterEach(() => {
  cleanup()
  h.exists = false
  h.root = 'r1'
  h.push = undefined
})

describe('useObjectViewerSetup', () => {
  it('follows an object the World gains and loses after mount', async () => {
    const world = liveWorld(() => h.exists)
    const { result } = renderHook(() => useObjectViewerSetup(world, objectKey))

    // The first read finds nothing.
    await waitFor(() => expect(result.current.objectState.loading).toBe(false))
    expect(result.current.objectState.value).toBeNull()
    expect(result.current.typeID).toBeUndefined()

    // A commit creates the object: the viewer shows it without a remount.
    commit(true)
    await waitFor(() =>
      expect(result.current.objectState.value?.getKey()).toBe(objectKey),
    )
    expect(result.current.typeID).toBe('type/doc')
    expect(result.current.rootRef).toBe('root://r1')

    // A commit deletes it: the viewer reports it missing.
    commit(false)
    await waitFor(() => expect(result.current.objectState.value).toBeNull())
    expect(result.current.objectState.loading).toBe(false)
  })

  it('keeps the root row while a new root formats', async () => {
    h.exists = true
    const world = liveWorld(() => h.exists)
    const shown: (string | undefined)[] = []
    const { result } = renderHook(() => {
      const setup = useObjectViewerSetup(world, objectKey)
      shown.push(setup.rootRef)
      return setup
    })
    await waitFor(() => expect(result.current.rootRef).toBe('root://r1'))

    // A content write changes the root: the row never disappears.
    const from = shown.length
    commit(true, 'r2')
    await waitFor(() => expect(result.current.rootRef).toBe('root://r2'))
    expect(shown.slice(from)).not.toContain(undefined)
  })

  it('shows an object deleted and re-created between two reads', async () => {
    // The watch finds the object; any other read lands on the revision that
    // deleted it, before the commit that re-creates it unchanged.
    h.exists = true
    const world = liveWorld(() => false)
    const { result } = renderHook(() => useObjectViewerSetup(world, objectKey))
    await waitFor(() => expect(result.current.typeID).toBe('type/doc'))

    // The re-create reads equal to what the watch had, so nothing re-reads.
    commit(true)
    await waitFor(() => expect(result.current.objectState.loading).toBe(false))
    expect(result.current.objectState.value?.getKey()).toBe(objectKey)
  })
})
