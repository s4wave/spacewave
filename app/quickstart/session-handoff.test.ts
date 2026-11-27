import { afterEach, describe, expect, it, vi } from 'vitest'

import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Session } from '@s4wave/sdk/session/session.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'

import { SharedResources } from '@aptre/bldr-sdk/hooks/SharedResources.js'
import type { Root } from '@s4wave/sdk/root/root.js'

import {
  createQuickstartHandoff,
  getQuickstartInitialObjectHandoff,
  isQuickstartRouteAwaitingResourcesList,
  releaseQuickstartAppHandoffs,
  releaseQuickstartSessionHandoffsForTests,
  retainQuickstartRoute,
} from './session-handoff.js'

// testRoot stands in for the Root the Session entry is keyed under.
const testRoot = { id: 1, released: false } as unknown as Root

function createTestSession(): {
  session: Session
  release: ReturnType<typeof vi.fn>
} {
  // Build a Session with id 10 that records its release.
  let released = false
  const release = vi.fn(() => {
    released = true
  })
  const session = Object.create(Session.prototype) as Session
  Object.defineProperty(session, 'id', { get: () => 10 })
  Object.defineProperty(session, 'released', {
    get: () => released,
  })
  Object.defineProperty(session, 'release', {
    value: release,
  })
  Object.defineProperty(session, Symbol.dispose, {
    value: () => {
      session.release()
    },
  })
  return { session, release }
}

function createTestResourceRef(resourceId: number): {
  ref: SharedObject['resourceRef']
  release: ReturnType<typeof vi.fn>
} {
  let released = false
  const release = vi.fn(() => {
    released = true
  })
  const ref = {
    get resourceId() {
      return resourceId
    },
    get released() {
      return released
    },
    get client() {
      return {}
    },
    createRef: vi.fn((id: number) => createTestResourceRef(id).ref),
    createResource: vi.fn(),
    release,
    [Symbol.dispose]: release,
  } as unknown as SharedObject['resourceRef']
  return { ref, release }
}

function createTestSharedObject(sharedObjectId: string): {
  resource: SharedObject
  release: ReturnType<typeof vi.fn>
} {
  const { ref, release } = createTestResourceRef(11)
  return {
    resource: new SharedObject(ref, { sharedObjectId }),
    release,
  }
}

function createTestSharedObjectBody(): {
  resource: SharedObjectBody
  release: ReturnType<typeof vi.fn>
} {
  const { ref, release } = createTestResourceRef(12)
  return {
    resource: new SharedObjectBody(ref),
    release,
  }
}

function createTestSpace(): {
  resource: Space
  release: ReturnType<typeof vi.fn>
} {
  const { ref, release } = createTestResourceRef(13)
  return {
    resource: new Space(ref),
    release,
  }
}

function createTestSpaceContents(): {
  resource: SpaceContents
  release: ReturnType<typeof vi.fn>
} {
  const { ref, release } = createTestResourceRef(14)
  return {
    resource: new SpaceContents(ref),
    release,
  }
}

function createTestSpaceWorld(): {
  resource: EngineWorldState
  release: ReturnType<typeof vi.fn>
} {
  const { ref, release } = createTestResourceRef(15)
  return {
    resource: new EngineWorldState(new Engine(ref), true, true),
    release,
  }
}

// createCleanup records resources passed to the setup's own cleanup.
function createCleanup(): { cleanup: RegisterCleanup; calls: unknown[] } {
  const calls: unknown[] = []
  return {
    cleanup: (resource) => {
      calls.push(resource)
      return resource
    },
    calls,
  }
}

// stageSpaceRoute stages Session 1 and Space space-1 from a setup in scope.
function stageSpaceRoute(shared: SharedResources, scope = ''): void {
  // Hold the chain a setup mounts, then stage it with an initial object.
  const { session } = createTestSession()
  const handoff = createQuickstartHandoff(
    createCleanup().cleanup,
    shared,
    scope,
  )
  handoff.cleanup(session)
  handoff.cleanup(createTestSharedObject('space-1').resource)
  handoff.cleanup(createTestSharedObjectBody().resource)
  handoff.cleanup(createTestSpace().resource)
  handoff.stage(testRoot, 1, session, 'space-1', {
    objectKey: 'files',
    objectType: 'unixfs/fs-node',
  })
}

describe('quickstart session handoff', () => {
  afterEach(() => {
    releaseQuickstartSessionHandoffsForTests()
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('moves the session into the shared mounts without releasing it', () => {
    // Hold a Session in the handoff and stage it.
    const shared = new SharedResources(null)
    const { session, release } = createTestSession()
    const { cleanup, calls } = createCleanup()
    const handoff = createQuickstartHandoff(cleanup, shared)
    expect(handoff.cleanup(session)).toBe(session)
    handoff.stage(testRoot, 1, session)

    // The shared mounts own it under the route's Session key.
    expect(calls).toEqual([])
    expect(shared.getSnapshot('1/session/1').value).toBe(session)
    expect(release).not.toHaveBeenCalled()
  })

  it('adopts the space chain and releases it when no route leases it', () => {
    // Start a handoff.
    vi.useFakeTimers()
    const shared = new SharedResources(null)
    const { cleanup, calls } = createCleanup()
    const handoff = createQuickstartHandoff(cleanup, shared)

    // Build the Space chain a Quickstart setup mounts.
    const { session } = createTestSession()
    const sharedObject = createTestSharedObject('space-1')
    const body = createTestSharedObjectBody()
    const space = createTestSpace()
    const contents = createTestSpaceContents()
    const world = createTestSpaceWorld()

    // Hold each mount in the handoff, then stage the route.
    for (const resource of [
      session,
      sharedObject.resource,
      body.resource,
      space.resource,
      contents.resource,
      world.resource,
    ]) {
      handoff.cleanup(resource)
    }
    handoff.stage(testRoot, 1, session, 'space-1', {
      objectKey: 'files',
      objectType: 'unixfs/fs-node',
    })

    // Each mount sits under the key its route hook reads.
    expect(calls).toEqual([])
    expect(shared.getSnapshot('10/so/space-1').value).toBe(
      sharedObject.resource,
    )
    expect(shared.getSnapshot('11/body').value).toBe(body.resource)
    expect(shared.getSnapshot('12/space').value).toBe(space.resource)
    expect(shared.getSnapshot('13/world').value).toBe(world.resource)
    expect(shared.getSnapshot('13/contents').value).toBe(contents.resource)

    // The route facts wait for the route.
    expect(getQuickstartInitialObjectHandoff(1, 'space-1', 'files')).toEqual({
      objectKey: 'files',
      objectType: 'unixfs/fs-node',
    })
    expect(isQuickstartRouteAwaitingResourcesList(1, 'space-1')).toBe(true)

    // Unleased mounts and facts last the grace period.
    vi.advanceTimersByTime(4999)
    expect(sharedObject.release).not.toHaveBeenCalled()
    expect(world.release).not.toHaveBeenCalled()

    // Then the shared mounts release them.
    vi.advanceTimersByTime(1)
    expect(sharedObject.release).toHaveBeenCalledOnce()
    expect(body.release).toHaveBeenCalledOnce()
    expect(space.release).toHaveBeenCalledOnce()
    expect(contents.release).toHaveBeenCalledOnce()
    expect(world.release).toHaveBeenCalledOnce()
    expect(getQuickstartInitialObjectHandoff(1, 'space-1', 'files')).toBeNull()
  })

  it('keeps route facts while the route retains them', () => {
    // Stage a route.
    vi.useFakeTimers()
    const shared = new SharedResources(null)
    stageSpaceRoute(shared)

    // A mounted route keeps its facts past the grace period.
    const release = retainQuickstartRoute(1, 'space-1')
    vi.advanceTimersByTime(10000)
    expect(isQuickstartRouteAwaitingResourcesList(1, 'space-1')).toBe(true)

    // Leaving the route drops them after the grace period.
    release()
    vi.advanceTimersByTime(5000)
    expect(isQuickstartRouteAwaitingResourcesList(1, 'space-1')).toBe(false)
    shared.clear()
  })

  it('isolates equal Session indexes and drops only the reset app', () => {
    // Stage the same route in two app scopes.
    const shared = new SharedResources(null)
    stageSpaceRoute(shared, 'left')
    stageSpaceRoute(shared, 'right')

    // Resetting one app drops only its facts.
    releaseQuickstartAppHandoffs('left')
    expect(isQuickstartRouteAwaitingResourcesList(1, 'space-1', 'left')).toBe(
      false,
    )
    expect(isQuickstartRouteAwaitingResourcesList(1, 'space-1', 'right')).toBe(
      true,
    )
    shared.clear()
  })

  it('releases held resources when setup fails before staging', () => {
    // Hold a Session, then fail the setup.
    const shared = new SharedResources(null)
    const { session, release } = createTestSession()
    const { cleanup, calls } = createCleanup()
    const handoff = createQuickstartHandoff(cleanup, shared)
    handoff.cleanup(session)
    handoff.releaseHeldResources()

    // The Session is released without reaching the setup's cleanup.
    expect(release).toHaveBeenCalledOnce()
    expect(calls).toEqual([])
  })
})
