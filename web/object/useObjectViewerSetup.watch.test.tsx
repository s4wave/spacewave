import {
  act,
  cleanup,
  render,
  renderHook,
  waitFor,
} from '@testing-library/react'
import { useEffect } from 'react'
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
  formatFails: false,
  typeFails: false,
  breakStream: false,
  opens: 0,
  rootResource: {
    value: { id: 1 } as object,
    loading: false,
    error: null,
    retry: () => {},
  },
}))

// The live watch runs the query on a snapshot each time the test commits.
vi.mock('@s4wave/sdk/world/world-query.js', () => ({
  watchWorldQuery: async function* (
    _engine: unknown,
    query: WorldQuery<unknown>,
    signal: AbortSignal,
  ) {
    h.opens++
    for (;;) {
      yield { seqno: 0n, value: await query(snapshot(h.exists), signal) }
      await new Promise<void>((resolve) => {
        h.push = resolve
      })
      if (h.breakStream) {
        h.breakStream = false
        throw new Error('stream reset')
      }
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
    if (h.formatFails) throw new Error('marshal hash failed')
    return 'root://' + (ref?.id ?? '')
  },
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  getObjectType: async () => {
    if (h.typeFails) throw new Error('read failed')
    return 'type/doc'
  },
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
    getEngine: () => ({ released: false }),
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
  h.formatFails = false
  h.typeFails = false
  h.breakStream = false
  h.opens = 0
  h.rootResource = { ...h.rootResource, value: { id: 1 } }
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

  it('shows a present object whose root fails to format', async () => {
    h.exists = true
    h.formatFails = true
    const world = liveWorld(() => h.exists)
    const { result } = renderHook(() => useObjectViewerSetup(world, objectKey))

    // Only the Root Ref row is lost: the object and its type still show.
    await waitFor(() =>
      expect(result.current.objectState.value?.getKey()).toBe(objectKey),
    )
    expect(result.current.objectState.error).toBeNull()
    expect(result.current.typeID).toBe('type/doc')
    expect(result.current.rootRef).toBeUndefined()
  })

  it('reports a failed watch as an error, not a missing object', async () => {
    h.exists = true
    h.typeFails = true
    const world = liveWorld(() => h.exists)
    const { result } = renderHook(() => useObjectViewerSetup(world, objectKey))

    await waitFor(() => expect(result.current.objectState.error).not.toBeNull())
    expect(result.current.objectState.loading).toBe(false)
    expect(result.current.objectState.value).toBeNull()
  })

  it('keeps the watch and the content through a Root reload', async () => {
    h.exists = true
    const world = liveWorld(() => h.exists)
    let mounts = 0
    function Content() {
      useEffect(() => {
        mounts++
      }, [])
      return null
    }
    // Viewer draws its content once the object arrives, as ObjectViewer does.
    function Viewer() {
      const setup = useObjectViewerSetup(world, objectKey)
      return setup.objectState.value ? <Content /> : null
    }
    const { rerender } = render(<Viewer />)
    await waitFor(() => expect(mounts).toBe(1))

    // The Root reloads: a new instance with the same id.
    h.rootResource = { ...h.rootResource, value: { id: 1 } }
    rerender(<Viewer />)
    commit(true, 'r2')
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10))
    })
    expect(mounts).toBe(1)
    expect(h.opens).toBe(1)
  })

  it('watches a UnixFS-typed object for every viewer', async () => {
    const world = liveWorld(() => h.exists)
    const { result } = renderHook(() =>
      useObjectViewerSetup(world, objectKey, { typeIDHint: 'unixfs/fs-node' }),
    )

    // The watch answers absent: missing is an answer, not a default.
    await waitFor(() => expect(result.current.objectState.loading).toBe(false))
    expect(result.current.objectState.value).toBeNull()
    expect(h.opens).toBe(1)

    commit(true)
    await waitFor(() =>
      expect(result.current.objectState.value?.getKey()).toBe(objectKey),
    )
  })

  it('opens no watch without an object key', async () => {
    const world = liveWorld(() => true)
    const { result } = renderHook(() =>
      useObjectViewerSetup(world, undefined, { typeIDHint: 'unixfs/fs-node' }),
    )

    await waitFor(() => expect(result.current.objectState.loading).toBe(false))
    expect(result.current.objectState.value).toBeNull()
    expect(result.current.objectState.error).toBeNull()
    expect(result.current.typeID).toBe('unixfs/fs-node')
    expect(h.opens).toBe(0)
  })

  it('reopens the watch on Retry after a failure before answering', async () => {
    h.exists = true
    h.typeFails = true
    const world = liveWorld(() => h.exists)
    const { result } = renderHook(() => useObjectViewerSetup(world, objectKey))
    await waitFor(() => expect(result.current.objectState.error).not.toBeNull())
    expect(h.opens).toBe(1)

    h.typeFails = false
    act(() => result.current.objectState.retry())
    await waitFor(() =>
      expect(result.current.objectState.value?.getKey()).toBe(objectKey),
    )
    expect(h.opens).toBe(2)
  })

  it('reports reopening, then missing for an object deleted meanwhile', async () => {
    h.exists = true
    const world = liveWorld(() => h.exists)
    const { result } = renderHook(() => useObjectViewerSetup(world, objectKey))
    await waitFor(() =>
      expect(result.current.objectState.value?.getKey()).toBe(objectKey),
    )

    // The stream fails after answering: the handle stays, marked reopening.
    h.breakStream = true
    act(() => h.push?.())
    await waitFor(() => {
      const state = result.current.objectState
      expect(state.loading && state.reopening?.message).toBe('stream reset')
    })
    expect(result.current.objectState.value?.getKey()).toBe(objectKey)

    // The object is deleted before the watch reopens: it then shows missing.
    h.exists = false
    await waitFor(() => expect(result.current.objectState.loading).toBe(false))
    expect(result.current.objectState.value).toBeNull()
    expect(result.current.objectState.error).toBeNull()
    expect(result.current.typeID).toBeUndefined()
    expect(h.opens).toBe(2)
  })
})
