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

// QuickstartHandoffRecord is every fact and resource staged under one
// session+shared-object key. releaseQuickstartSharedObjectHandoffNow walks
// the resource fields child-before-parent.
interface QuickstartHandoffRecord {
  sharedObject?: SharedObject
  sharedObjectBody?: SharedObjectBody
  space?: Space
  spaceContents?: SpaceContents
  spaceWorld?: EngineWorldState
  initialObject?: QuickstartInitialObjectHandoff
  awaitingResourcesList?: boolean
  releaseTimer?: ReturnType<typeof setTimeout>
}

const sharedObjectHandoffsByKey = new Map<string, QuickstartHandoffRecord>()

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

// handoffRecordAt returns the record for key, creating one when create is set.
function handoffRecordAt(
  key: string,
  create: boolean,
): QuickstartHandoffRecord | undefined {
  let record = sharedObjectHandoffsByKey.get(key)
  if (!record && create) {
    record = {}
    sharedObjectHandoffsByKey.set(key, record)
  }
  return record
}

// deleteHandoffRecordIfEmpty drops records with no staged state left.
function deleteHandoffRecordIfEmpty(
  key: string,
  record: QuickstartHandoffRecord,
): void {
  if (
    !record.sharedObject &&
    !record.sharedObjectBody &&
    !record.space &&
    !record.spaceContents &&
    !record.spaceWorld &&
    !record.initialObject &&
    !record.awaitingResourcesList &&
    !record.releaseTimer
  ) {
    sharedObjectHandoffsByKey.delete(key)
  }
}

function cancelScheduledRelease(key: string): void {
  const record = handoffRecordAt(key, false)
  if (!record?.releaseTimer) {
    return
  }
  clearTimeout(record.releaseTimer)
  record.releaseTimer = undefined
  deleteHandoffRecordIfEmpty(key, record)
}

function scheduleSharedObjectHandoffRelease(key: string): void {
  cancelScheduledRelease(key)
  const record = handoffRecordAt(key, true)
  if (!record) return
  const timer = setTimeout(() => {
    if (record.releaseTimer !== timer) {
      return
    }
    record.releaseTimer = undefined
    releaseQuickstartSharedObjectHandoffNow(key)
  }, handoffReleaseGraceMs)
  record.releaseTimer = timer
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

// HandoffField projects one resource field of a handoff record: how to read
// and write it, detect release, clone it for a consumer, and release it.
interface HandoffField<T> {
  get(record: QuickstartHandoffRecord): T | undefined
  set(record: QuickstartHandoffRecord, value: T | undefined): void
  released(resource: T): boolean
  clone(resource: T): T
  release(resource: T): void
}

const SHARED_OBJECT_FIELD: HandoffField<SharedObject> = {
  get: (record) => record.sharedObject,
  set: (record, value) => {
    record.sharedObject = value
  },
  released: (sharedObject) => sharedObject.released,
  clone: cloneSharedObject,
  release: releaseSharedObject,
}

const SHARED_OBJECT_BODY_FIELD: HandoffField<SharedObjectBody> = {
  get: (record) => record.sharedObjectBody,
  set: (record, value) => {
    record.sharedObjectBody = value
  },
  released: (body) => body.released,
  clone: cloneSharedObjectBody,
  release: releaseSharedObjectBody,
}

const SPACE_FIELD: HandoffField<Space> = {
  get: (record) => record.space,
  set: (record, value) => {
    record.space = value
  },
  released: (space) => space.released,
  clone: cloneSpace,
  release: releaseSpace,
}

const SPACE_CONTENTS_FIELD: HandoffField<SpaceContents> = {
  get: (record) => record.spaceContents,
  set: (record, value) => {
    record.spaceContents = value
  },
  released: (contents) => contents.released,
  clone: cloneSpaceContents,
  release: releaseSpaceContents,
}

const SPACE_WORLD_FIELD: HandoffField<EngineWorldState> = {
  get: (record) => record.spaceWorld,
  set: (record, value) => {
    record.spaceWorld = value
  },
  released: (world) => world.getEngine().released,
  clone: cloneSpaceWorld,
  release: releaseSpaceWorld,
}

// consumeHandoffField returns a clone of the staged resource for key,
// canceling any scheduled release and re-arming the grace timer.
function consumeHandoffField<T>(key: string, field: HandoffField<T>): T | null {
  cancelScheduledRelease(key)
  const record = handoffRecordAt(key, false)
  if (!record) return null
  const resource = field.get(record)
  if (!resource) return null
  if (field.released(resource)) {
    field.set(record, undefined)
    deleteHandoffRecordIfEmpty(key, record)
    return null
  }
  scheduleSharedObjectHandoffRelease(key)
  return field.clone(resource)
}

// stageHandoffResource moves one held resource into the handoff record,
// releasing the previously staged value when it differs.
function stageHandoffResource<T>(
  key: string,
  held: T[],
  field: HandoffField<T>,
): void {
  const resource = held.pop()
  if (!resource) return
  const record = handoffRecordAt(key, true)
  if (!record) return
  const existing = field.get(record)
  if (existing && existing !== resource) {
    field.release(existing)
  }
  field.set(record, resource)
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
  return consumeHandoffField(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
    SHARED_OBJECT_FIELD,
  )
}

export function hasQuickstartSharedObjectHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): boolean {
  const record = sharedObjectHandoffsByKey.get(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
  )
  return (
    !!record &&
    !!(
      record.sharedObject ||
      record.sharedObjectBody ||
      record.space ||
      record.spaceContents ||
      record.spaceWorld
    )
  )
}

export function markQuickstartSharedObjectHandoffAwaitingResourcesList(
  sessionIndex: number,
  sharedObjectId: string,
): void {
  const record = handoffRecordAt(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
    true,
  )
  if (record) {
    record.awaitingResourcesList = true
  }
}

export function clearQuickstartSharedObjectHandoffAwaitingResourcesList(
  sessionIndex: number,
  sharedObjectId: string,
): void {
  const key = sharedObjectHandoffKey(sessionIndex, sharedObjectId)
  const record = handoffRecordAt(key, false)
  if (!record) return
  record.awaitingResourcesList = false
  deleteHandoffRecordIfEmpty(key, record)
}

export function isQuickstartSharedObjectHandoffAwaitingResourcesList(
  sessionIndex: number,
  sharedObjectId: string,
): boolean {
  return !!sharedObjectHandoffsByKey.get(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
  )?.awaitingResourcesList
}

export function consumeQuickstartSharedObjectBodyHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): SharedObjectBody | null {
  return consumeHandoffField(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
    SHARED_OBJECT_BODY_FIELD,
  )
}

export function consumeQuickstartSpaceHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): Space | null {
  return consumeHandoffField(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
    SPACE_FIELD,
  )
}

export function consumeQuickstartSpaceContentsHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): SpaceContents | null {
  return consumeHandoffField(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
    SPACE_CONTENTS_FIELD,
  )
}

export function consumeQuickstartSpaceWorldHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): EngineWorldState | null {
  return consumeHandoffField(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
    SPACE_WORLD_FIELD,
  )
}

export function getQuickstartInitialObjectHandoff(
  sessionIndex: number | null | undefined,
  sharedObjectId: string,
  objectKey?: string,
): QuickstartInitialObjectHandoff | null {
  if (sessionIndex == null || !sharedObjectId || !objectKey) return null
  const record = sharedObjectHandoffsByKey.get(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
  )
  const handoff = record?.initialObject
  if (!handoff || handoff.objectKey !== objectKey) return null
  return handoff
}

function releaseQuickstartSharedObjectHandoffNow(key: string): void {
  const record = handoffRecordAt(key, false)
  if (!record) return
  record.awaitingResourcesList = false
  record.initialObject = undefined
  const world = record.spaceWorld
  if (world) {
    record.spaceWorld = undefined
    releaseSpaceWorld(world)
  }
  const contents = record.spaceContents
  if (contents) {
    record.spaceContents = undefined
    releaseSpaceContents(contents)
  }
  const space = record.space
  if (space) {
    record.space = undefined
    releaseSpace(space)
  }
  const body = record.sharedObjectBody
  if (body) {
    record.sharedObjectBody = undefined
    releaseSharedObjectBody(body)
  }
  const sharedObject = record.sharedObject
  if (sharedObject) {
    record.sharedObject = undefined
    releaseSharedObject(sharedObject)
  }
  deleteHandoffRecordIfEmpty(key, record)
}

export function releaseQuickstartSharedObjectHandoff(
  sessionIndex: number,
  sharedObjectId: string,
): void {
  scheduleSharedObjectHandoffRelease(
    sharedObjectHandoffKey(sessionIndex, sharedObjectId),
  )
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
      const record = handoffRecordAt(key, true)
      if (!record) return
      record.initialObject = initialObject
      stageHandoffResource(key, heldSharedObjects, SHARED_OBJECT_FIELD)
      stageHandoffResource(
        key,
        heldSharedObjectBodies,
        SHARED_OBJECT_BODY_FIELD,
      )
      stageHandoffResource(key, heldSpaces, SPACE_FIELD)
      stageHandoffResource(key, heldSpaceContents, SPACE_CONTENTS_FIELD)
      stageHandoffResource(key, heldSpaceWorlds, SPACE_WORLD_FIELD)
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
  for (const [key, record] of sharedObjectHandoffsByKey) {
    if (record.releaseTimer) {
      clearTimeout(record.releaseTimer)
      record.releaseTimer = undefined
    }
    record.awaitingResourcesList = false
    record.initialObject = undefined
    if (record.spaceWorld) releaseSpaceWorld(record.spaceWorld)
    if (record.spaceContents) releaseSpaceContents(record.spaceContents)
    if (record.space) releaseSpace(record.space)
    if (record.sharedObjectBody)
      releaseSharedObjectBody(record.sharedObjectBody)
    if (record.sharedObject) releaseSharedObject(record.sharedObject)
    deleteHandoffRecordIfEmpty(key, record)
  }

  for (const handoff of handoffsBySessionIndex.values()) {
    releaseHandoff(handoff)
  }
  handoffsBySessionIndex.clear()
}
