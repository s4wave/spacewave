// Quickstart hands the Session and Space it mounted to the route it opens.
//
// The mounts move into the shared mounts under the keys the route's hooks
// use, so the route reuses them instead of mounting again. The route facts,
// the initial object and whether the Space is still missing from the
// resources list, stay here under the route's Session and SharedObject.

import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  sharedResourceKey,
  type SharedResources,
} from '@aptre/bldr-sdk/hooks/SharedResources.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import { Session } from '@s4wave/sdk/session/session.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import {
  bodyMountName,
  sessionMountName,
  sharedObjectMountName,
  spaceContentsMountName,
  spaceMountName,
  spaceWorldMountName,
} from '@s4wave/app/hooks/useSharedMounts.js'

export interface QuickstartInitialObjectHandoff {
  objectKey: string
  objectType: string
}

// QuickstartRouteRecord holds the facts Quickstart staged for one route.
interface QuickstartRouteRecord {
  initialObject?: QuickstartInitialObjectHandoff
  awaitingResourcesList: boolean
  releaseTimer?: ReturnType<typeof setTimeout>
}

const routeRecords = new Map<string, QuickstartRouteRecord>()

// routeReleaseGraceMs is the grace period for route facts across a remount
// of the route.
const routeReleaseGraceMs = 5000

function routeKey(
  sessionIndex: number,
  sharedObjectId: string,
  scope: string,
): string {
  return `${scope}/${sessionIndex}/${sharedObjectId}`
}

// cancelRouteRelease stops a scheduled release of the record at key.
function cancelRouteRelease(key: string): void {
  const record = routeRecords.get(key)
  if (record?.releaseTimer) {
    clearTimeout(record.releaseTimer)
    record.releaseTimer = undefined
  }
}

/**
 * retainQuickstartRoute keeps the route facts while the route is mounted and
 * returns the function that releases them after a grace period.
 */
export function retainQuickstartRoute(
  sessionIndex: number,
  sharedObjectId: string,
  scope = '',
): () => void {
  const key = routeKey(sessionIndex, sharedObjectId, scope)
  cancelRouteRelease(key)
  return () => {
    const record = routeRecords.get(key)
    if (!record) return
    cancelRouteRelease(key)
    record.releaseTimer = setTimeout(() => {
      if (routeRecords.get(key) === record) {
        routeRecords.delete(key)
      }
    }, routeReleaseGraceMs)
  }
}

/**
 * isQuickstartRouteAwaitingResourcesList reports whether Quickstart opened
 * this route before the resources list published its Space.
 */
export function isQuickstartRouteAwaitingResourcesList(
  sessionIndex: number,
  sharedObjectId: string,
  scope = '',
): boolean {
  return !!routeRecords.get(routeKey(sessionIndex, sharedObjectId, scope))
    ?.awaitingResourcesList
}

/** clearQuickstartRouteAwaitingResourcesList records the Space as listed. */
export function clearQuickstartRouteAwaitingResourcesList(
  sessionIndex: number,
  sharedObjectId: string,
  scope = '',
): void {
  const record = routeRecords.get(routeKey(sessionIndex, sharedObjectId, scope))
  if (record) {
    record.awaitingResourcesList = false
  }
}

export function getQuickstartInitialObjectHandoff(
  sessionIndex: number | null | undefined,
  sharedObjectId: string,
  objectKey?: string,
  scope = '',
): QuickstartInitialObjectHandoff | null {
  if (sessionIndex == null || !sharedObjectId || !objectKey) return null
  const handoff = routeRecords.get(
    routeKey(sessionIndex, sharedObjectId, scope),
  )?.initialObject
  if (!handoff || handoff.objectKey !== objectKey) return null
  return handoff
}

type Disposable = { [Symbol.dispose](): void }

// QuickstartHandoff collects what a Quickstart setup mounts.
export interface QuickstartHandoff {
  // cleanup holds Session and Space resources for the handoff and passes
  // anything else to the setup's own cleanup.
  cleanup: RegisterCleanup
  // releaseHeldResources disposes what the handoff holds.
  releaseHeldResources(): void
  // stage moves the held Session and Space resources into the shared mounts
  // and records the route facts.
  stage(
    root: Root,
    sessionIndex: number,
    session: Session,
    sharedObjectId?: string,
    initialObject?: QuickstartInitialObjectHandoff,
  ): void
}

// isHandoffResource reports whether resource is one the route mounts.
function isHandoffResource(resource: unknown): resource is Disposable {
  return (
    resource instanceof Session ||
    resource instanceof SharedObject ||
    resource instanceof SharedObjectBody ||
    resource instanceof Space ||
    resource instanceof SpaceContents ||
    resource instanceof EngineWorldState
  )
}

// takeHeld removes and returns the last held resource of type ctor.
function takeHeld<T>(
  held: Disposable[],
  ctor: abstract new (...args: never[]) => T,
): (T & Disposable) | null {
  for (let i = held.length - 1; i >= 0; i--) {
    const resource = held[i]
    if (resource instanceof ctor) {
      held.splice(i, 1)
      return resource as T & Disposable
    }
  }
  return null
}

/** createQuickstartHandoff starts collecting a setup's mounts. */
export function createQuickstartHandoff(
  cleanup: RegisterCleanup,
  shared: SharedResources,
  scope = '',
): QuickstartHandoff {
  const held: Disposable[] = []

  const releaseHeldResources = () => {
    while (held.length) {
      held.pop()?.[Symbol.dispose]()
    }
  }

  return {
    cleanup: (resource) => {
      if (isHandoffResource(resource)) {
        held.push(resource)
        return resource
      }
      return cleanup(resource)
    },
    releaseHeldResources,
    stage(root, sessionIndex, session, sharedObjectId, initialObject) {
      // Adopt each mount under the key its route hook reads, parent first.
      const sessionIdx = held.indexOf(session)
      if (sessionIdx >= 0) held.splice(sessionIdx, 1)
      shared.adopt(
        sharedResourceKey(root, sessionMountName(sessionIndex)),
        session,
        [session],
      )
      const sharedObject = sharedObjectId && takeHeld(held, SharedObject)
      const body = sharedObject && takeHeld(held, SharedObjectBody)
      const space = body && takeHeld(held, Space)
      if (sharedObjectId && sharedObject && body && space) {
        shared.adopt(
          sharedResourceKey(session, sharedObjectMountName(sharedObjectId)),
          sharedObject,
          [sharedObject],
        )
        shared.adopt(sharedResourceKey(sharedObject, bodyMountName), body, [
          body,
        ])
        shared.adopt(sharedResourceKey(body, spaceMountName), space, [space])
        const world = takeHeld(held, EngineWorldState)
        if (world) {
          shared.adopt(sharedResourceKey(space, spaceWorldMountName), world, [
            world,
          ])
        }
        const contents = takeHeld(held, SpaceContents)
        if (contents) {
          shared.adopt(
            sharedResourceKey(space, spaceContentsMountName),
            contents,
            [contents],
          )
        }

        // Record the route facts until the route mounts and retains them.
        const key = routeKey(sessionIndex, sharedObjectId, scope)
        routeRecords.set(key, { initialObject, awaitingResourcesList: true })
        retainQuickstartRoute(sessionIndex, sharedObjectId, scope)()
      }
      releaseHeldResources()
    },
  }
}

/** releaseQuickstartAppHandoffs drops the route facts of an app scope. */
export function releaseQuickstartAppHandoffs(scope: string): void {
  releaseMatchingRoutes(scope)
}

/** releaseQuickstartSessionHandoffsForTests drops every route fact. */
export function releaseQuickstartSessionHandoffsForTests(): void {
  releaseMatchingRoutes()
}

function releaseMatchingRoutes(scope?: string): void {
  for (const [key, record] of routeRecords) {
    if (scope !== undefined && !key.startsWith(scope + '/')) continue
    if (record.releaseTimer) clearTimeout(record.releaseTimer)
    routeRecords.delete(key)
  }
}
