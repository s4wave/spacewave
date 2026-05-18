import { afterEach, describe, expect, it, vi } from 'vitest'

import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Session } from '@s4wave/sdk/session/session.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'

import {
  consumeQuickstartSpaceContentsHandoff,
  consumeQuickstartSharedObjectBodyHandoff,
  consumeQuickstartSharedObjectHandoff,
  consumeQuickstartSpaceHandoff,
  consumeQuickstartSpaceWorldHandoff,
  consumeQuickstartSessionHandoff,
  createQuickstartSessionHandoffCleanup,
  getQuickstartInitialObjectHandoff,
  releaseQuickstartSharedObjectHandoff,
  releaseQuickstartSessionHandoffsForTests,
  stageQuickstartSessionHandoff,
} from './session-handoff.js'

function createTestSession(): {
  session: Session
  release: ReturnType<typeof vi.fn>
} {
  let released = false
  const release = vi.fn(() => {
    released = true
  })
  const session = Object.create(Session.prototype) as Session
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

describe('quickstart session handoff', () => {
  afterEach(() => {
    releaseQuickstartSessionHandoffsForTests()
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('stages and consumes a mounted session without releasing it', () => {
    const { session, release } = createTestSession()

    stageQuickstartSessionHandoff({ sessionIndex: 1, session })

    expect(consumeQuickstartSessionHandoff(1)?.session).toBe(session)
    expect(consumeQuickstartSessionHandoff(1)).toBeNull()
    expect(release).not.toHaveBeenCalled()
  })

  it('releases stale staged sessions when a later handoff replaces them', () => {
    const first = createTestSession()
    const second = createTestSession()

    stageQuickstartSessionHandoff({ sessionIndex: 1, session: first.session })
    stageQuickstartSessionHandoff({ sessionIndex: 1, session: second.session })

    expect(first.release).toHaveBeenCalledOnce()
    expect(consumeQuickstartSessionHandoff(1)?.session).toBe(second.session)
    expect(second.release).not.toHaveBeenCalled()
  })

  it('keeps quickstart-owned sessions out of the normal cleanup stack', () => {
    const { session, release } = createTestSession()
    const cleanupCall = vi.fn()
    const cleanup: RegisterCleanup = (resource) => {
      cleanupCall(resource)
      return resource
    }
    const handoff = createQuickstartSessionHandoffCleanup(cleanup)

    expect(handoff.cleanup(session)).toBe(session)
    handoff.stage(1, session)

    expect(cleanupCall).not.toHaveBeenCalled()
    expect(consumeQuickstartSessionHandoff(1)?.session).toBe(session)
    expect(release).not.toHaveBeenCalled()
  })

  it('keeps staged quickstart shared object resources reusable during route handoff', () => {
    vi.useFakeTimers()
    const { session } = createTestSession()
    const sharedObject = createTestSharedObject('space-1')
    const body = createTestSharedObjectBody()
    const space = createTestSpace()
    const contents = createTestSpaceContents()
    const world = createTestSpaceWorld()
    const cleanupCall = vi.fn()
    const cleanup: RegisterCleanup = (resource) => {
      cleanupCall(resource)
      return resource
    }
    const handoff = createQuickstartSessionHandoffCleanup(cleanup)

    handoff.cleanup(session)
    handoff.cleanup(sharedObject.resource)
    handoff.cleanup(body.resource)
    handoff.cleanup(space.resource)
    handoff.cleanup(contents.resource)
    handoff.cleanup(world.resource)
    handoff.stage(1, session, 'space-1', {
      objectKey: 'files',
      objectType: 'unixfs/fs-node',
    })

    expect(cleanupCall).not.toHaveBeenCalled()
    expect(consumeQuickstartSessionHandoff(1)?.session).toBe(session)
    const consumedSharedObject = consumeQuickstartSharedObjectHandoff(
      1,
      'space-1',
    )
    const consumedBody = consumeQuickstartSharedObjectBodyHandoff(1, 'space-1')
    const consumedSpace = consumeQuickstartSpaceHandoff(1, 'space-1')
    const consumedContents = consumeQuickstartSpaceContentsHandoff(1, 'space-1')
    const consumedWorld = consumeQuickstartSpaceWorldHandoff(1, 'space-1')
    expect(consumedSharedObject).toBeInstanceOf(SharedObject)
    expect(consumedSharedObject).not.toBe(sharedObject.resource)
    expect(consumedBody).toBeInstanceOf(SharedObjectBody)
    expect(consumedBody).not.toBe(body.resource)
    expect(consumedSpace).toBeInstanceOf(Space)
    expect(consumedSpace).not.toBe(space.resource)
    expect(consumedContents).toBeInstanceOf(SpaceContents)
    expect(consumedContents).not.toBe(contents.resource)
    expect(consumedWorld).toBeInstanceOf(EngineWorldState)
    expect(consumedWorld).not.toBe(world.resource)
    expect(sharedObject.release).not.toHaveBeenCalled()
    expect(body.release).not.toHaveBeenCalled()
    expect(space.release).not.toHaveBeenCalled()
    expect(contents.release).not.toHaveBeenCalled()
    expect(world.release).not.toHaveBeenCalled()
    expect(getQuickstartInitialObjectHandoff(1, 'space-1', 'files')).toEqual({
      objectKey: 'files',
      objectType: 'unixfs/fs-node',
    })

    const secondSharedObject = consumeQuickstartSharedObjectHandoff(
      1,
      'space-1',
    )
    const secondBody = consumeQuickstartSharedObjectBodyHandoff(1, 'space-1')
    const secondSpace = consumeQuickstartSpaceHandoff(1, 'space-1')
    const secondContents = consumeQuickstartSpaceContentsHandoff(1, 'space-1')
    const secondWorld = consumeQuickstartSpaceWorldHandoff(1, 'space-1')

    expect(secondSharedObject).toBeInstanceOf(SharedObject)
    expect(secondSharedObject).not.toBe(consumedSharedObject)
    expect(secondBody).toBeInstanceOf(SharedObjectBody)
    expect(secondBody).not.toBe(consumedBody)
    expect(secondSpace).toBeInstanceOf(Space)
    expect(secondSpace).not.toBe(consumedSpace)
    expect(secondContents).toBeInstanceOf(SpaceContents)
    expect(secondContents).not.toBe(consumedContents)
    expect(secondWorld).toBeInstanceOf(EngineWorldState)
    expect(secondWorld).not.toBe(consumedWorld)

    releaseQuickstartSharedObjectHandoff(1, 'space-1')
    vi.advanceTimersByTime(4999)

    expect(sharedObject.release).not.toHaveBeenCalled()
    expect(body.release).not.toHaveBeenCalled()
    expect(space.release).not.toHaveBeenCalled()
    expect(contents.release).not.toHaveBeenCalled()
    expect(world.release).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)

    expect(sharedObject.release).toHaveBeenCalledOnce()
    expect(body.release).toHaveBeenCalledOnce()
    expect(space.release).toHaveBeenCalledOnce()
    expect(contents.release).toHaveBeenCalledOnce()
    expect(world.release).toHaveBeenCalledOnce()
    expect(consumeQuickstartSharedObjectHandoff(1, 'space-1')).toBeNull()
    expect(consumeQuickstartSharedObjectBodyHandoff(1, 'space-1')).toBeNull()
    expect(consumeQuickstartSpaceHandoff(1, 'space-1')).toBeNull()
    expect(consumeQuickstartSpaceContentsHandoff(1, 'space-1')).toBeNull()
    expect(consumeQuickstartSpaceWorldHandoff(1, 'space-1')).toBeNull()
    expect(getQuickstartInitialObjectHandoff(1, 'space-1', 'files')).toBeNull()

    consumedSharedObject?.release()
    consumedBody?.release()
    consumedSpace?.release()
    consumedContents?.release()
    consumedWorld?.release()
    secondSharedObject?.release()
    secondBody?.release()
    secondSpace?.release()
    secondContents?.release()
    secondWorld?.release()
  })

  it('releases held sessions when setup fails before staging', () => {
    const { session, release } = createTestSession()
    const cleanupCall = vi.fn()
    const cleanup: RegisterCleanup = (resource) => {
      cleanupCall(resource)
      return resource
    }
    const handoff = createQuickstartSessionHandoffCleanup(cleanup)

    handoff.cleanup(session)
    handoff.releaseHeldResources()

    expect(release).toHaveBeenCalledOnce()
    expect(cleanupCall).not.toHaveBeenCalled()
  })
})
