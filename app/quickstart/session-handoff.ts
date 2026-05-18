import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Session } from '@s4wave/sdk/session/session.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'

export interface QuickstartSessionHandoff {
  sessionIndex: number
  session: Session
}

export interface QuickstartInitialObjectHandoff {
  objectKey: string
  objectType: string
}

const handoffsBySessionIndex = new Map<number, QuickstartSessionHandoff>()
const sharedObjectsBySessionAndId = new Map<string, SharedObject>()
const sharedObjectBodiesBySessionAndId = new Map<string, SharedObjectBody>()
const spacesBySessionAndId = new Map<string, Space>()
const spaceContentsBySessionAndId = new Map<string, SpaceContents>()
const spaceWorldsBySessionAndId = new Map<string, EngineWorldState>()
const initialObjectsBySessionAndId = new Map<
  string,
  QuickstartInitialObjectHandoff
>()
const releaseTimersBySessionAndId = new Map<
  string,
  ReturnType<typeof setTimeout>
>()

const handoffReleaseGraceMs = 5000

function sharedObjectHandoffKey(
  sessionIndex: number,
  sharedObjectId: string,
): string {
  return `${sessionIndex}:${sharedObjectId}`
}

function releaseSession(session: Session): void {
  if (!session.released) {
    session.release()
  }
}

function releaseHandoff(handoff: QuickstartSessionHandoff): void {
  releaseSession(handoff.session)
}

function releaseSharedObject(sharedObject: SharedObject): void {
  if (!sharedObject.released) {
    sharedObject.release()
  }
}

function releaseSharedObjectBody(body: SharedObjectBody): void {
  if (!body.released) {
    body.release()
  }
}

function releaseSpace(space: Space): void {
  if (!space.released) {
    space.release()
  }
}

function releaseSpaceContents(contents: SpaceContents): void {
  if (!contents.released) {
    contents.release()
  }
}

function releaseSpaceWorld(world: EngineWorldState): void {
  world.release()
}

function cancelScheduledRelease(key: string): void {
  const timer = releaseTimersBySessionAndId.get(key)
  if (!timer) {
    return
  }
  clearTimeout(timer)
  releaseTimersBySessionAndId.delete(key)
}

function scheduleSharedObjectHandoffRelease(key: string): void {
  cancelScheduledRelease(key)
  const timer = setTimeout(() => {
    if (releaseTimersBySessionAndId.get(key) !== timer) {
      return
    }
    releaseTimersBySessionAndId.delete(key)
    releaseQuickstartSharedObjectHandoffNow(key)
  }, handoffReleaseGraceMs)
  releaseTimersBySessionAndId.set(key, timer)
}

function cloneSharedObject(sharedObject: SharedObject): SharedObject {
  return new SharedObject(
    sharedObject.resourceRef.createRef(sharedObject.id),
    sharedObject.meta,
  )
}

function cloneSharedObjectBody(body: SharedObjectBody): SharedObjectBody {
  return new SharedObjectBody(body.resourceRef.createRef(body.id))
}

function cloneSpace(space: Space): Space {
  return new Space(space.resourceRef.createRef(space.id))
}

function cloneSpaceContents(contents: SpaceContents): SpaceContents {
  return new SpaceContents(contents.resourceRef.createRef(contents.id))
}

function cloneSpaceWorld(world: EngineWorldState): EngineWorldState {
  const engine = world.getEngine()
  return new EngineWorldState(
    new Engine(engine.resourceRef.createRef(engine.id)),
    !world.getReadOnly(),
    true,
  )
}

function consumeClonedResource<T>(
  resources: Map<string, T>,
  key: string,
  released: (resource: T) => boolean,
  clone: (resource: T) => T,
): T | null {
  cancelScheduledRelease(key)
  const resource = resources.get(key)
  if (!resource) return null
  if (released(resource)) {
    resources.delete(key)
    return null
  }
  const cloned = clone(resource)
  scheduleSharedObjectHandoffRelease(key)
  return cloned
}

export function stageQuickstartSessionHandoff(
  handoff: QuickstartSessionHandoff,
): void {
  const existing = handoffsBySessionIndex.get(handoff.sessionIndex)
  if (existing?.session === handoff.session) {
    handoffsBySessionIndex.set(handoff.sessionIndex, handoff)
    return
  }
  if (existing) {
    releaseHandoff(existing)
  }
  handoffsBySessionIndex.set(handoff.sessionIndex, handoff)
}

export function consumeQuickstartSessionHandoff(
  sessionIndex: number,
): QuickstartSessionHandoff | null {
  const handoff = handoffsBySessionIndex.get(sessionIndex)
  if (!handoff) return null
  handoffsBySessionIndex.delete(sessionIndex)
  if (handoff.session.released) return null
  return handoff
}

export function consumeQuickstartSharedObjectHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): SharedObject | null {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  return consumeClonedResource(
    sharedObjectsBySessionAndId,
    key,
    (sharedObject) => sharedObject.released,
    cloneSharedObject,
  )
}

export function consumeQuickstartSharedObjectBodyHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): SharedObjectBody | null {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  return consumeClonedResource(
    sharedObjectBodiesBySessionAndId,
    key,
    (body) => body.released,
    cloneSharedObjectBody,
  )
}

export function consumeQuickstartSpaceHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): Space | null {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  return consumeClonedResource(
    spacesBySessionAndId,
    key,
    (space) => space.released,
    cloneSpace,
  )
}

export function consumeQuickstartSpaceContentsHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): SpaceContents | null {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  return consumeClonedResource(
    spaceContentsBySessionAndId,
    key,
    (contents) => contents.released,
    cloneSpaceContents,
  )
}

export function consumeQuickstartSpaceWorldHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): EngineWorldState | null {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  return consumeClonedResource(
    spaceWorldsBySessionAndId,
    key,
    (world) => world.getEngine().released,
    cloneSpaceWorld,
  )
}

export function getQuickstartInitialObjectHandoff(
  sessionIndex: number | null | undefined,
  sharedObjectId: string,
  objectKey?: string,
): QuickstartInitialObjectHandoff | null {
  if (sessionIndex == null || !sharedObjectId || !objectKey) return null
  const handoff = initialObjectsBySessionAndId.get(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
  )
  if (!handoff || handoff.objectKey !== objectKey) return null
  return handoff
}

function releaseQuickstartSharedObjectHandoffNow(key: string): void {
  cancelScheduledRelease(key)
  initialObjectsBySessionAndId.delete(key)
  const world = spaceWorldsBySessionAndId.get(key)
  if (world) {
    spaceWorldsBySessionAndId.delete(key)
    releaseSpaceWorld(world)
  }
  const contents = spaceContentsBySessionAndId.get(key)
  if (contents) {
    spaceContentsBySessionAndId.delete(key)
    releaseSpaceContents(contents)
  }
  const space = spacesBySessionAndId.get(key)
  if (space) {
    spacesBySessionAndId.delete(key)
    releaseSpace(space)
  }
  const body = sharedObjectBodiesBySessionAndId.get(key)
  if (body) {
    sharedObjectBodiesBySessionAndId.delete(key)
    releaseSharedObjectBody(body)
  }
  const sharedObject = sharedObjectsBySessionAndId.get(key)
  if (sharedObject) {
    sharedObjectsBySessionAndId.delete(key)
    releaseSharedObject(sharedObject)
  }
}

export function releaseQuickstartSharedObjectHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): void {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  scheduleSharedObjectHandoffRelease(key)
}

export interface QuickstartSessionHandoffCleanup {
  cleanup: RegisterCleanup
  releaseHeldResources(): void
  stage(
    sessionIndex: number,
    session: Session,
    sharedObjectId?: string,
    initialObject?: QuickstartInitialObjectHandoff,
  ): void
}

export function createQuickstartSessionHandoffCleanup(
  cleanup: RegisterCleanup,
): QuickstartSessionHandoffCleanup {
  const heldSessions: Session[] = []
  const heldSharedObjects: SharedObject[] = []
  const heldSharedObjectBodies: SharedObjectBody[] = []
  const heldSpaces: Space[] = []
  const heldSpaceContents: SpaceContents[] = []
  const heldSpaceWorlds: EngineWorldState[] = []

  const handoffCleanup: RegisterCleanup = (resource) => {
    if (resource instanceof Session) {
      heldSessions.push(resource)
      return resource
    }
    if (resource instanceof SharedObject) {
      heldSharedObjects.push(resource)
      return resource
    }
    if (resource instanceof SharedObjectBody) {
      heldSharedObjectBodies.push(resource)
      return resource
    }
    if (resource instanceof Space) {
      heldSpaces.push(resource)
      return resource
    }
    if (resource instanceof SpaceContents) {
      heldSpaceContents.push(resource)
      return resource
    }
    if (resource instanceof EngineWorldState) {
      heldSpaceWorlds.push(resource)
      return resource
    }
    return cleanup(resource)
  }

  function releaseHeldSpaceResources(): void {
    while (heldSpaceWorlds.length) {
      const world = heldSpaceWorlds.pop()
      if (world) {
        releaseSpaceWorld(world)
      }
    }
    while (heldSpaceContents.length) {
      const contents = heldSpaceContents.pop()
      if (contents) {
        releaseSpaceContents(contents)
      }
    }
    while (heldSpaces.length) {
      const space = heldSpaces.pop()
      if (space) {
        releaseSpace(space)
      }
    }
    while (heldSharedObjectBodies.length) {
      const body = heldSharedObjectBodies.pop()
      if (body) {
        releaseSharedObjectBody(body)
      }
    }
    while (heldSharedObjects.length) {
      const sharedObject = heldSharedObjects.pop()
      if (sharedObject) {
        releaseSharedObject(sharedObject)
      }
    }
  }

  return {
    cleanup: handoffCleanup,
    releaseHeldResources() {
      releaseHeldSpaceResources()
      while (heldSessions.length) {
        const session = heldSessions.pop()
        if (session) {
          releaseSession(session)
        }
      }
    },
    stage(sessionIndex, session, sharedObjectId, initialObject) {
      const index = heldSessions.indexOf(session)
      if (index >= 0) {
        heldSessions.splice(index, 1)
      }
      stageQuickstartSessionHandoff({ sessionIndex, session })

      if (!sharedObjectId) {
        releaseHeldSpaceResources()
        return
      }
      const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
      cancelScheduledRelease(key)
      if (initialObject) {
        initialObjectsBySessionAndId.set(key, initialObject)
      } else {
        initialObjectsBySessionAndId.delete(key)
      }
      const sharedObject = heldSharedObjects.pop()
      if (sharedObject) {
        const existing = sharedObjectsBySessionAndId.get(key)
        if (existing && existing !== sharedObject && !existing.released) {
          existing.release()
        }
        sharedObjectsBySessionAndId.set(key, sharedObject)
      }
      const body = heldSharedObjectBodies.pop()
      if (body) {
        const existing = sharedObjectBodiesBySessionAndId.get(key)
        if (existing && existing !== body && !existing.released) {
          existing.release()
        }
        sharedObjectBodiesBySessionAndId.set(key, body)
      }
      const space = heldSpaces.pop()
      if (space) {
        const existing = spacesBySessionAndId.get(key)
        if (existing && existing !== space && !existing.released) {
          existing.release()
        }
        spacesBySessionAndId.set(key, space)
      }
      const contents = heldSpaceContents.pop()
      if (contents) {
        const existing = spaceContentsBySessionAndId.get(key)
        if (existing && existing !== contents && !existing.released) {
          existing.release()
        }
        spaceContentsBySessionAndId.set(key, contents)
      }
      const world = heldSpaceWorlds.pop()
      if (world) {
        const existing = spaceWorldsBySessionAndId.get(key)
        if (existing && existing !== world) {
          existing.release()
        }
        spaceWorldsBySessionAndId.set(key, world)
      }
      while (heldSharedObjects.length) {
        const extra = heldSharedObjects.pop()
        if (extra) {
          releaseSharedObject(extra)
        }
      }
      while (heldSharedObjectBodies.length) {
        const extra = heldSharedObjectBodies.pop()
        if (extra) {
          releaseSharedObjectBody(extra)
        }
      }
      while (heldSpaces.length) {
        const extra = heldSpaces.pop()
        if (extra) {
          releaseSpace(extra)
        }
      }
      while (heldSpaceContents.length) {
        const extra = heldSpaceContents.pop()
        if (extra) {
          releaseSpaceContents(extra)
        }
      }
      while (heldSpaceWorlds.length) {
        const extra = heldSpaceWorlds.pop()
        if (extra) {
          releaseSpaceWorld(extra)
        }
      }
    },
  }
}

export function releaseQuickstartSessionHandoffsForTests(): void {
  for (const timer of releaseTimersBySessionAndId.values()) {
    clearTimeout(timer)
  }
  releaseTimersBySessionAndId.clear()

  for (const world of spaceWorldsBySessionAndId.values()) {
    world.release()
  }
  spaceWorldsBySessionAndId.clear()
  initialObjectsBySessionAndId.clear()

  for (const contents of spaceContentsBySessionAndId.values()) {
    if (!contents.released) {
      contents.release()
    }
  }
  spaceContentsBySessionAndId.clear()

  for (const space of spacesBySessionAndId.values()) {
    if (!space.released) {
      space.release()
    }
  }
  spacesBySessionAndId.clear()

  for (const body of sharedObjectBodiesBySessionAndId.values()) {
    if (!body.released) {
      body.release()
    }
  }
  sharedObjectBodiesBySessionAndId.clear()

  for (const sharedObject of sharedObjectsBySessionAndId.values()) {
    if (!sharedObject.released) {
      sharedObject.release()
    }
  }
  sharedObjectsBySessionAndId.clear()

  for (const handoff of handoffsBySessionIndex.values()) {
    releaseHandoff(handoff)
  }
  handoffsBySessionIndex.clear()
}
