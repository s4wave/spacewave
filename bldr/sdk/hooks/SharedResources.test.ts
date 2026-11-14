import React, { act } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRoot, type Root } from 'react-dom/client'
import type {
  Client as ResourceClient,
  ResourceReleasedEvent,
} from '../resource/client.js'
import { ResourcesProvider } from './ResourcesContext.js'
import {
  SharedResources,
  sharedResourceGraceMs,
  type SharedParent,
} from './SharedResources.js'
import { useSharedResource } from './useSharedResource.js'
import type { RegisterCleanup, Resource } from './useResource.js'

// FakeClient delivers resource release events to the registry.
class FakeClient {
  private listeners = new Set<(event: ResourceReleasedEvent) => void>()

  onResourceReleased(cb: (event: ResourceReleasedEvent) => void): () => void {
    this.listeners.add(cb)
    return () => this.listeners.delete(cb)
  }

  emit(resourceId: number): void {
    const event: ResourceReleasedEvent = {
      resourceId,
      reason: 'server-released',
    }
    this.listeners.forEach((cb) => cb(event))
  }
}

// FakeHandle is a mounted value that records its disposal.
class FakeHandle implements SharedParent {
  released = false
  constructor(readonly id: number) {}
  [Symbol.dispose](): void {
    this.released = true
  }
}

// flush lets settled mount promises publish.
async function flush(): Promise<void> {
  for (let i = 0; i < 3; i++) {
    await Promise.resolve()
  }
}

// setup returns a registry whose mounts yield FakeHandles numbered from 100.
function setup() {
  // Build the registry over a fake client and a numbering mount.
  const client = new FakeClient()
  const shared = new SharedResources(client as unknown as ResourceClient)
  let nextId = 100
  const mount = vi.fn((_signal: AbortSignal, cleanup: RegisterCleanup) =>
    Promise.resolve(cleanup(new FakeHandle(nextId++))),
  )
  return { client, shared, mount }
}

describe('SharedResources', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('mounts a key once for every lease', async () => {
    // Lease one key from two tabs.
    const { shared, mount } = setup()
    const releaseA = shared.acquire('1/session/1', mount, null)
    const releaseB = shared.acquire('1/session/1', mount, null)
    await flush()

    // Both share one mount.
    expect(mount).toHaveBeenCalledOnce()
    expect(shared.getSnapshot<FakeHandle>('1/session/1').value?.id).toBe(100)
    releaseA()
    releaseB()
    shared.clear()
  })

  it('mounts different keys separately', async () => {
    // Lease two Session indexes, as tabs on different Sessions do.
    const { shared, mount } = setup()
    shared.acquire('1/session/1', mount, null)
    shared.acquire('1/session/2', mount, null)
    await flush()

    // Each Session has its own mount.
    expect(mount).toHaveBeenCalledTimes(2)
    expect(shared.getSnapshot<FakeHandle>('1/session/1').value?.id).toBe(100)
    expect(shared.getSnapshot<FakeHandle>('1/session/2').value?.id).toBe(101)
    shared.clear()
  })

  it('keeps a value through the grace period after the last lease', async () => {
    // Mount and drop the only lease.
    vi.useFakeTimers()
    const { shared, mount } = setup()
    shared.acquire('k', mount, null)()
    await flush()
    const value = shared.getSnapshot<FakeHandle>('k').value

    // A lease inside the grace period reuses the value.
    vi.advanceTimersByTime(sharedResourceGraceMs - 1)
    const release = shared.acquire('k', mount, null)
    expect(mount).toHaveBeenCalledOnce()
    expect(value?.released).toBe(false)

    // A full grace period without a lease disposes it.
    release()
    vi.advanceTimersByTime(sharedResourceGraceMs)
    expect(value?.released).toBe(true)
    expect(shared.getSnapshot('k').value).toBeNull()
  })

  it('remounts a leased value the server released', async () => {
    // Mount a leased value.
    const { client, shared, mount } = setup()
    shared.acquire('k', mount, null)
    await flush()
    const first = shared.getSnapshot<FakeHandle>('k').value
    if (!first) throw new Error('expected a value')

    // The server releases it.
    first.released = true
    client.emit(first.id)
    expect(shared.getSnapshot('k').loading).toBe(true)
    await flush()

    // The entry mounts a new value.
    expect(mount).toHaveBeenCalledTimes(2)
    expect(shared.getSnapshot<FakeHandle>('k').value?.id).toBe(101)
    shared.clear()
  })

  it('waits for a released parent instead of remounting under it', async () => {
    // Mount a child under a parent.
    const { client, shared, mount } = setup()
    const parent = new FakeHandle(1)
    shared.acquire('1/k', mount, parent)
    await flush()
    const child = shared.getSnapshot<FakeHandle>('1/k').value
    if (!child) throw new Error('expected a value')

    // The server releases both.
    parent.released = true
    child.released = true
    client.emit(parent.id)

    // The child shows loading until its lessees move to the new parent.
    expect(mount).toHaveBeenCalledOnce()
    expect(shared.getSnapshot('1/k').loading).toBe(true)
    shared.clear()
  })

  it('does not keep empty or failed results after the last lease', async () => {
    // Settle an empty and a failed mount, then drop their leases.
    const { shared } = setup()
    const empty = vi.fn(() => Promise.resolve(null))
    shared.acquire('empty', empty, null)()
    const failed = vi.fn(() => Promise.reject(new Error('boom')))
    const releaseFailed = shared.acquire('failed', failed, null)
    await flush()
    expect(shared.getSnapshot('failed').error?.message).toBe('boom')
    releaseFailed()

    // The next lease of each mounts again.
    shared.acquire('empty', empty, null)
    shared.acquire('failed', failed, null)
    expect(empty).toHaveBeenCalledTimes(2)
    expect(failed).toHaveBeenCalledTimes(2)
    shared.clear()
  })

  it('remounts on request and disposes the old value after the new one', async () => {
    // Mount a value.
    const { shared, mount } = setup()
    shared.acquire('k', mount, null)
    await flush()
    const first = shared.getSnapshot<FakeHandle>('k').value

    // A remount keeps showing the old value while loading.
    shared.remount('k')
    expect(shared.getSnapshot('k')).toMatchObject({
      value: first,
      loading: true,
    })
    expect(first?.released).toBe(false)
    await flush()

    // The new value replaces it, and only then is the old one disposed.
    expect(shared.getSnapshot<FakeHandle>('k').value?.id).toBe(101)
    expect(first?.released).toBe(true)
    shared.clear()
  })

  it('remounts once for each new generation', async () => {
    // Mount under the empty generation.
    const { shared, mount } = setup()
    shared.acquire('k', mount, null)
    await flush()

    // Two tabs see a new account; a passive follower passes none.
    shared.acquire('k', mount, null, 'account-b')
    shared.acquire('k', mount, null, 'account-b')
    shared.acquire('k', mount, null)
    await flush()

    // Only the first new generation remounts.
    expect(mount).toHaveBeenCalledTimes(2)
    shared.clear()
  })

  it('hands an adopted value to the first lease without mounting', () => {
    // Adopt a value, then lease it.
    const { shared, mount } = setup()
    const adopted = new FakeHandle(7)
    shared.adopt('k', adopted, [adopted])
    shared.acquire('k', mount, null)

    // The lease reads the adopted value, which the registry now disposes.
    expect(mount).not.toHaveBeenCalled()
    expect(shared.getSnapshot('k').value).toBe(adopted)
    shared.clear()
    expect(adopted.released).toBe(true)
  })

  it('keeps a leased entry over an adopted value', async () => {
    // A tab leases a key whose mount is still in flight.
    vi.useFakeTimers()
    const { shared, mount } = setup()
    shared.acquire('k', mount, null)

    // Adopting under the key disposes the adopted value instead.
    const adopted = new FakeHandle(7)
    shared.adopt('k', adopted, [adopted])
    expect(adopted.released).toBe(true)

    // The lessee's mount lands and outlives the grace period.
    await flush()
    vi.advanceTimersByTime(sharedResourceGraceMs)
    const value = shared.getSnapshot<FakeHandle>('k').value
    expect(value?.id).toBe(100)
    expect(value?.released).toBe(false)
    shared.clear()
  })
})
// SessionProbe renders the shared Session at index under root.
function SessionProbe({
  root,
  index,
  mount,
  seen,
}: {
  root: Resource<FakeHandle>
  index: number
  mount: () => Promise<FakeHandle>
  seen: (FakeHandle | null)[]
}) {
  const session = useSharedResource(root, `session/${index}`, mount)
  seen.push(session.value)
  return null
}

describe('useSharedResource', () => {
  let container: HTMLDivElement | null = null
  let reactRoot: Root | null = null

  afterEach(() => {
    act(() => reactRoot?.unmount())
    container?.remove()
    container = null
    reactRoot = null
  })

  it('renders a held mount on the first pass of a second component', async () => {
    // A Root and a mount that numbers Sessions from 10.
    const client = new FakeClient() as unknown as ResourceClient
    const root: Resource<FakeHandle> = {
      value: new FakeHandle(1),
      loading: false,
      error: null,
      retry: () => {},
    }
    let nextId = 10
    const mount = vi.fn(() => Promise.resolve(new FakeHandle(nextId++)))

    // Record what each tab renders.
    container = document.createElement('div')
    document.body.appendChild(container)
    reactRoot = createRoot(container)
    const firstSeen: (FakeHandle | null)[] = []
    const secondSeen: (FakeHandle | null)[] = []
    const otherSeen: (FakeHandle | null)[] = []

    // Render one, two, or three tabs; the third is on another Session.
    const probe = (index: number, seen: (FakeHandle | null)[]) =>
      React.createElement(SessionProbe, { root, index, mount, seen })
    const render = (tabs: number) =>
      act(() =>
        reactRoot?.render(
          React.createElement(
            ResourcesProvider,
            { client },
            probe(1, firstSeen),
            tabs > 1 ? probe(1, secondSeen) : null,
            tabs > 2 ? probe(2, otherSeen) : null,
          ),
        ),
      )

    // The first tab mounts Session 1.
    render(1)
    await act(flush)
    expect(firstSeen.at(-1)?.id).toBe(10)

    // A second tab on Session 1 sees it on its first render; the third
    // tab mounts Session 2 separately.
    render(3)
    await act(flush)
    expect(secondSeen[0]?.id).toBe(10)
    expect(otherSeen.at(-1)?.id).toBe(11)
    expect(mount).toHaveBeenCalledTimes(2)
  })

  it('keeps its lease and value while the parent reloads its live value', async () => {
    // A Root whose Session mounts once.
    const client = new FakeClient() as unknown as ResourceClient
    const handle = new FakeHandle(1)
    const ready: Resource<FakeHandle> = {
      value: handle,
      loading: false,
      error: null,
      retry: () => {},
    }
    const mount = vi.fn(() => Promise.resolve(new FakeHandle(10)))
    container = document.createElement('div')
    document.body.appendChild(container)
    reactRoot = createRoot(container)
    const seen: (FakeHandle | null)[] = []
    const render = (root: Resource<FakeHandle>) =>
      act(() =>
        reactRoot?.render(
          React.createElement(
            ResourcesProvider,
            { client },
            React.createElement(SessionProbe, { root, index: 1, mount, seen }),
          ),
        ),
      )
    render(ready)
    await act(flush)
    const session = seen.at(-1)
    expect(session?.id).toBe(10)

    // The Root reloads over the same value: the Session stays drawn and held.
    seen.length = 0
    render({ ...ready, loading: true })
    await act(flush)
    expect(seen.every((s) => s === session)).toBe(true)
    render(ready)
    await act(flush)
    expect(seen.at(-1)).toBe(session)
    expect(session?.released).toBe(false)
    expect(mount).toHaveBeenCalledTimes(1)
  })
})
