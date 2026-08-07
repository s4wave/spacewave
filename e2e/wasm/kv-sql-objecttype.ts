import { mountSpace } from '@s4wave/app/space/space.js'
import { KvStore, KvStoreTypeID } from '@s4wave/sdk/kv/kv.js'
import {
  SqlDbTypeID,
  SqlQuery,
  SqlQueryResultObject,
  SqlQueryResultTypeID,
  SqlQueryTypeID,
  SqlWorkbench,
  SqlWorkbenchTypeID,
} from '@s4wave/sdk/sql/index.js'
import { WorkbenchTabKind } from '@s4wave/sdk/sql/workbench/workbench.pb.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { keyToIRI, predToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'

import { withTimeout } from './test-utils.js'

const KV_STORE_KEY = 'kv/store'
const SQL_DB_KEY = 'sql/db'
const SQL_SEED_QUERY_KEY = 'sql/query/example'
const SQL_SECOND_QUERY_KEY = 'sql/query/e2e-second'
const SQL_WORKBENCH_KEY = 'sql/workbench/e2e'

const PRED_SQL_QUERY_AGAINST = 'against'
const PRED_SQL_QUERY_PRODUCED_BY = 'produced-by'

interface RouteTarget {
  sessionIdx: number
  sharedObjectId: string
}

interface MountedWorlds {
  readWorld: EngineWorldState
  writeWorld: EngineWorldState
}

type ActionArgs =
  | {
      action: 'seqno'
      deadlineMs?: number
    }
  | {
      action: 'kv-value'
      key: string
      afterSeqno?: string
      deadlineMs?: number
    }
  | {
      action: 'sql-linkage'
      deadlineMs?: number
    }
  | {
      action: 'prepare-workbench-pins'
      deadlineMs?: number
    }
  | {
      action: 'workbench-pins'
      deadlineMs?: number
    }

type ResourceLike = { release?: () => void }

function currentRouteTarget(): RouteTarget {
  const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
  if (!match) {
    throw new Error('expected current /u/{idx}/so/{id} route')
  }
  return {
    sessionIdx: Number(match[1]),
    sharedObjectId: decodeURIComponent(match[2] ?? ''),
  }
}

function cleanupTracker() {
  const stack: ResourceLike[] = []
  const cleanup = <T extends ResourceLike>(resource: T): T => {
    stack.push(resource)
    return resource
  }
  return {
    cleanup,
    releaseAll() {
      for (;;) {
        const resource = stack.pop()
        if (!resource) return
        resource.release?.()
      }
    },
  }
}

async function withMountedSpace<T>(
  signal: AbortSignal,
  fn: (worlds: MountedWorlds) => Promise<T>,
): Promise<T> {
  const root = globalThis.__s4wave_debug?.root
  if (!root) {
    throw new Error('missing __s4wave_debug.root')
  }
  const target = currentRouteTarget()
  const tracker = cleanupTracker()
  try {
    const mounted = await root.mountSessionByIdx(
      { sessionIdx: target.sessionIdx },
      signal,
    )
    const session = tracker.cleanup(mounted?.session)
    if (!session) {
      throw new Error('mountSessionByIdx returned no session')
    }
    const space = tracker.cleanup(
      await mountSpace({
        session,
        spaceResp: {
          sharedObjectRef: {
            providerResourceRef: { id: target.sharedObjectId },
          },
        },
        abortSignal: signal,
        cleanup: tracker.cleanup,
      }),
    )
    const writeWorld = tracker.cleanup(
      await space.accessWorldState(true, signal),
    )
    const readWorld = new EngineWorldState(writeWorld.getEngine(), false)
    return await fn({ readWorld, writeWorld })
  } finally {
    tracker.releaseAll()
  }
}

async function accessKvStore(
  world: EngineWorldState,
  signal: AbortSignal,
): Promise<KvStore> {
  const access = await world.accessTypedObject(KV_STORE_KEY, signal)
  if (access.typeId !== KvStoreTypeID || !access.resourceId) {
    throw new Error(`expected ${KvStoreTypeID}, got ${access.typeId}`)
  }
  return world.getResourceRef().createResource(access.resourceId, KvStore)
}

async function accessSqlQuery(
  world: EngineWorldState,
  objectKey: string,
  signal: AbortSignal,
): Promise<SqlQuery> {
  const access = await world.accessTypedObject(objectKey, signal)
  if (access.typeId !== SqlQueryTypeID || !access.resourceId) {
    throw new Error(`expected ${SqlQueryTypeID}, got ${access.typeId}`)
  }
  return world.getResourceRef().createResource(access.resourceId, SqlQuery)
}

async function accessSqlWorkbench(
  world: EngineWorldState,
  objectKey: string,
  signal: AbortSignal,
): Promise<SqlWorkbench> {
  const access = await world.accessTypedObject(objectKey, signal)
  if (access.typeId !== SqlWorkbenchTypeID || !access.resourceId) {
    throw new Error(`expected ${SqlWorkbenchTypeID}, got ${access.typeId}`)
  }
  return world.getResourceRef().createResource(access.resourceId, SqlWorkbench)
}

async function accessSqlQueryResult(
  world: EngineWorldState,
  objectKey: string,
  signal: AbortSignal,
): Promise<SqlQueryResultObject> {
  const access = await world.accessTypedObject(objectKey, signal)
  if (access.typeId !== SqlQueryResultTypeID || !access.resourceId) {
    throw new Error(`expected ${SqlQueryResultTypeID}, got ${access.typeId}`)
  }
  return world
    .getResourceRef()
    .createResource(access.resourceId, SqlQueryResultObject)
}

async function maybeWaitSeqno(
  world: EngineWorldState,
  afterSeqno: string | undefined,
  signal: AbortSignal,
): Promise<void> {
  if (!afterSeqno) return
  await world.waitSeqno(BigInt(afterSeqno) + 1n, signal)
}

async function readKvValue(
  key: string,
  afterSeqno: string | undefined,
  signal: AbortSignal,
) {
  return await withMountedSpace(signal, async ({ readWorld }) => {
    await maybeWaitSeqno(readWorld, afterSeqno, signal)
    const store = await accessKvStore(readWorld, signal)
    try {
      const encoder = new TextEncoder()
      const result = await store.get(encoder.encode(key), signal)
      return {
        action: 'kv-value',
        key,
        found: result.found,
        value: new TextDecoder().decode(result.data),
      }
    } finally {
      store.release()
    }
  })
}

async function readSeqno(signal: AbortSignal) {
  return await withMountedSpace(signal, async ({ readWorld }) => {
    const seqno = await readWorld.getSeqno(signal)
    return {
      action: 'seqno',
      seqno: seqno.seqno.toString(),
    }
  })
}

async function createEmptyTypedObject(
  world: EngineWorldState,
  objectKey: string,
  typeID: string,
  signal: AbortSignal,
): Promise<boolean> {
  const existing = await world.getObject(objectKey, signal)
  if (existing) {
    existing.release()
    return false
  }

  const created = await world.createObject(objectKey, {}, signal)
  try {
    await setObjectType(world, objectKey, typeID, signal)
  } finally {
    created.release()
  }
  return true
}

async function prepareSecondQuery(
  world: EngineWorldState,
  signal: AbortSignal,
): Promise<boolean> {
  const created = await createEmptyTypedObject(
    world,
    SQL_SECOND_QUERY_KEY,
    SqlQueryTypeID,
    signal,
  )
  if (!created) {
    return false
  }

  const query = await accessSqlQuery(world, SQL_SECOND_QUERY_KEY, signal)
  try {
    await query.initialize(
      'SELECT name, role FROM quickstart.people ORDER BY id',
      'mysql',
      SQL_DB_KEY,
      signal,
    )
  } finally {
    query.release()
  }
  return true
}

async function prepareWorkbenchPins(signal: AbortSignal) {
  return await withMountedSpace(signal, async ({ readWorld, writeWorld }) => {
    const dbAccess = await readWorld.accessTypedObject(SQL_DB_KEY, signal)
    if (dbAccess.typeId !== SqlDbTypeID) {
      throw new Error(`expected ${SqlDbTypeID}, got ${dbAccess.typeId}`)
    }

    const secondQueryCreated = await prepareSecondQuery(writeWorld, signal)
    const workbenchCreated = await createEmptyTypedObject(
      writeWorld,
      SQL_WORKBENCH_KEY,
      SqlWorkbenchTypeID,
      signal,
    )

    const workbench = await accessSqlWorkbench(
      readWorld,
      SQL_WORKBENCH_KEY,
      signal,
    )
    try {
      if (workbenchCreated) {
        await workbench.initialize(
          SQL_DB_KEY,
          'Browser SQL Workbench',
          signal,
        )
      }
      await workbench.addPin(SQL_SEED_QUERY_KEY, signal)
      await workbench.addPin(SQL_SECOND_QUERY_KEY, signal)
      await workbench.setLayout(
        [
          {
            tabId: 'query:seed',
            objectKey: SQL_SEED_QUERY_KEY,
            kind: WorkbenchTabKind.QUERY,
            title: 'example',
            pinned: true,
          },
          {
            tabId: 'query:second',
            objectKey: SQL_SECOND_QUERY_KEY,
            kind: WorkbenchTabKind.QUERY,
            title: 'e2e-second',
            pinned: true,
          },
        ],
        {
          mode: 'split',
          sidebarWidth: 280,
          resultPanelHeight: 360,
          activeTabId: 'query:seed',
        },
        signal,
      )
      const state = await workbench.getWorkbench(signal)
      const pins = state.workbench?.pinnedQueryObjectKeys ?? []
      return {
        action: 'prepare-workbench-pins',
        workbenchObjectKey: SQL_WORKBENCH_KEY,
        targetDbObjectKey: state.workbench?.targetDbObjectKey ?? '',
        pinnedQueryObjectKeys: pins,
        secondQueryCreated,
        workbenchCreated,
      }
    } finally {
      workbench.release()
    }
  })
}

async function readWorkbenchPins(signal: AbortSignal) {
  return await withMountedSpace(signal, async ({ readWorld }) => {
    const workbench = await accessSqlWorkbench(
      readWorld,
      SQL_WORKBENCH_KEY,
      signal,
    )
    try {
      const state = await workbench.getWorkbench(signal)
      return {
        action: 'workbench-pins',
        workbenchObjectKey: SQL_WORKBENCH_KEY,
        targetDbObjectKey: state.workbench?.targetDbObjectKey ?? '',
        pinnedQueryObjectKeys: state.workbench?.pinnedQueryObjectKeys ?? [],
        openTabObjectKeys:
          state.workbench?.openTabs?.map((tab) => tab.objectKey ?? '') ?? [],
      }
    } finally {
      workbench.release()
    }
  })
}

async function readSqlLinkage(signal: AbortSignal) {
  return await withMountedSpace(signal, async ({ readWorld }) => {
    const resultKeys = await readWorld.listObjectsWithType(
      SqlQueryResultTypeID,
      signal,
    )
    if (resultKeys.length !== 1) {
      return {
        action: 'sql-linkage',
        resultObjectKeys: resultKeys,
        resultObjectKey: '',
        producedByQuadCount: 0,
        againstQuadCount: 0,
        sourceQueryObjectKey: '',
        targetDbObjectKey: '',
        rowCount: '',
      }
    }

    const resultKey = resultKeys[0] ?? ''
    const result = await accessSqlQueryResult(readWorld, resultKey, signal)
    try {
      const grid = await result.getResultGrid(signal)
      const producedBy = await readWorld.lookupGraphQuads(
        keyToIRI(resultKey),
        predToIRI(PRED_SQL_QUERY_PRODUCED_BY),
        keyToIRI(SQL_SEED_QUERY_KEY),
        undefined,
        2,
        signal,
      )
      const against = await readWorld.lookupGraphQuads(
        keyToIRI(resultKey),
        predToIRI(PRED_SQL_QUERY_AGAINST),
        keyToIRI(SQL_DB_KEY),
        undefined,
        2,
        signal,
      )
      return {
        action: 'sql-linkage',
        resultObjectKeys: resultKeys,
        resultObjectKey: resultKey,
        producedByQuadCount: producedBy.quads?.length ?? 0,
        againstQuadCount: against.quads?.length ?? 0,
        sourceQueryObjectKey: grid.sourceQueryObjectKey ?? '',
        targetDbObjectKey: grid.targetDbObjectKey ?? '',
        rowCount: grid.rowCount?.toString() ?? '',
      }
    } finally {
      result.release()
    }
  })
}

export default async function (args: ActionArgs) {
  const deadlineMs = args.deadlineMs ?? 120000
  return await withTimeout(deadlineMs, async (signal) => {
    switch (args.action) {
      case 'seqno':
        return await readSeqno(signal)
      case 'kv-value':
        return await readKvValue(args.key, args.afterSeqno, signal)
      case 'sql-linkage':
        return await readSqlLinkage(signal)
      case 'prepare-workbench-pins':
        return await prepareWorkbenchPins(signal)
      case 'workbench-pins':
        return await readWorkbenchPins(signal)
    }
  })
}
