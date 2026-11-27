import { useMemo } from 'react'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { Execution } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import { Job } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { Pass } from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'
import { Task } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { ForgeDashboard } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { changePattern } from '@s4wave/sdk/world/change-feed.js'
import {
  entitySetKey,
  entitySetSource,
  readKeyedSet,
  watchKeyedSet,
  type EntitySetSpec,
  type KeyedRowPage,
  type KeyedSet,
  type KeyedSetSource,
} from '@s4wave/sdk/world/entity-set.js'
import { iriToKey, isIRI, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { TypePred, typeIDFromIRI } from '@s4wave/sdk/world/types/types.js'
import {
  WorldStateResource,
  type IWorldState,
} from '@s4wave/sdk/world/world-state.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import { PRED_DASHBOARD_FORGE_REF } from '@s4wave/web/forge/predicates.js'
import type { ForgeLinkedEntity } from '@s4wave/web/forge/useForgeLinkedEntities.js'

const entityTypeLabels: Record<string, string> = {
  'forge/task': 'Task',
  'forge/job': 'Job',
  'forge/cluster': 'Cluster',
  'forge/worker': 'Worker',
  'forge/pass': 'Pass',
  'forge/execution': 'Execution',
}

const jobStateLabels: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'PENDING',
  2: 'RUNNING',
  3: 'COMPLETE',
}

const taskStateLabels: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'PENDING',
  2: 'RUNNING',
  3: 'CHECKING',
  4: 'COMPLETE',
  5: 'RETRY',
}

const passStateLabels: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'PENDING',
  2: 'RUNNING',
  3: 'CHECKING',
  4: 'COMPLETE',
}

const executionStateLabels: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'PENDING',
  2: 'RUNNING',
  3: 'COMPLETE',
}

const activitySourceTypeIds = new Set(Object.keys(entityTypeLabels))

interface ForgeDashboardActivitySource {
  objectKey: string
  typeId: string
  timestamp?: Date
  state?: string
  logs?: Array<{
    timestamp: Date
    level: string
    message: string
  }>
}

export interface ForgeDashboardActivityEntry {
  id: string
  objectKey?: string
  typeId?: string
  timestamp: Date
  title: string
  detail: string
}

export function describeActivitySource(
  entity: ForgeLinkedEntity,
  data: Uint8Array,
): ForgeDashboardActivitySource | null {
  switch (entity.typeId) {
    case 'forge/job': {
      const job = Job.fromBinary(data)
      return {
        objectKey: entity.objectKey,
        typeId: entity.typeId,
        timestamp: job.timestamp ?? undefined,
        state: jobStateLabels[job.jobState ?? 0] ?? 'UNKNOWN',
      }
    }
    case 'forge/task': {
      const task = Task.fromBinary(data)
      return {
        objectKey: entity.objectKey,
        typeId: entity.typeId,
        timestamp: task.timestamp ?? undefined,
        state: taskStateLabels[task.taskState ?? 0] ?? 'UNKNOWN',
      }
    }
    case 'forge/pass': {
      const pass = Pass.fromBinary(data)
      return {
        objectKey: entity.objectKey,
        typeId: entity.typeId,
        timestamp: pass.timestamp ?? undefined,
        state: passStateLabels[pass.passState ?? 0] ?? 'UNKNOWN',
      }
    }
    case 'forge/execution': {
      const execution = Execution.fromBinary(data)
      return {
        objectKey: entity.objectKey,
        typeId: entity.typeId,
        timestamp: execution.timestamp ?? undefined,
        state: executionStateLabels[execution.executionState ?? 0] ?? 'UNKNOWN',
        logs: (execution.logEntries ?? []).flatMap((entry) =>
          entry.timestamp
            ? [
                {
                  timestamp: entry.timestamp,
                  level: entry.level ?? 'info',
                  message: entry.message ?? '',
                },
              ]
            : [],
        ),
      }
    }
  }
  return null
}

// ForgeDashboardMember is one object a dashboard links: its type, and the
// activity its body describes when its type has activity.
export interface ForgeDashboardMember {
  readonly typeId: string
  readonly source: ForgeDashboardActivitySource | null
}

// forgeDashboardSource is the keyed set of the objects a dashboard links.
// Each read lists the members' types, then reads the bodies of only those
// whose type has activity, so other linked objects cost no body read.
export function forgeDashboardSource(
  objectKey: string,
): KeyedSetSource<ForgeDashboardMember> {
  const spec: EntitySetSpec = {
    case: 'link',
    origin: objectKey,
    predicate: PRED_DASHBOARD_FORGE_REF,
    direction: 'out',
  }
  const bodies = entitySetSource(spec, {
    name: 'bytes',
    decode: (body) => body,
  })
  // A retype commits only the type quad, so each member is followed by its
  // type quads as well as its key, and such a quad routes to its subject
  // when that is an IRI.
  const typePred = changePattern({ predicate: TypePred }).predicate
  return {
    ...bodies,
    names: (keys) => ({
      keys,
      patterns: keys.map((key) =>
        changePattern({ subject: keyToIRI(key), predicate: TypePred }),
      ),
    }),
    route: (change, members) => {
      const route = bodies.route(change, members)
      const retyped = (change.quads ?? []).flatMap((quad) =>
        quad.predicate === typePred && isIRI(quad.subject)
          ? [iriToKey(quad.subject)]
          : [],
      )
      if (retyped.length === 0) return route
      return { list: route.list, keys: [...route.keys, ...retyped] }
    },
    read: async function* (tx, keys, signal) {
      if (keys.length === 0) return
      const types = await tx.listGraphEdgeBuckets([...keys], 1, {
        predicate: TypePred,
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      })
      const typeIds = new Map(
        (types.buckets ?? []).map((bucket) => [
          bucket.originObjectKey ?? '',
          typeIDFromIRI(bucket.outgoing?.[0]?.obj),
        ]),
      )
      const typeOf = (key: string) => typeIds.get(key) ?? ''
      const active = keys.filter((key) =>
        activitySourceTypeIds.has(typeOf(key)),
      )
      const rows: KeyedRowPage<ForgeDashboardMember>['rows'] = keys
        .filter((key) => !activitySourceTypeIds.has(typeOf(key)))
        .map((key) => [key, { typeId: typeOf(key), source: null }] as const)
      if (active.length === 0) {
        yield { rows }
        return
      }
      let first = true
      for await (const page of bodies.read(tx, active, signal)) {
        const decoded = page.rows.map(([key, body]) => {
          const typeId = typeOf(key)
          return [
            key,
            body && {
              typeId,
              source: describeActivitySource({ objectKey: key, typeId }, body),
            },
          ] as const
        })
        yield {
          seqno: page.seqno,
          rows: first ? [...rows, ...decoded] : decoded,
        }
        first = false
      }
    },
  }
}

export function buildForgeDashboardActivityEntries(
  dashboard: ForgeDashboard | undefined,
  sources: ForgeDashboardActivitySource[],
): ForgeDashboardActivityEntry[] {
  const entries: ForgeDashboardActivityEntry[] = []

  if (dashboard?.createdAt) {
    entries.push({
      id: 'dashboard-created',
      timestamp: dashboard.createdAt,
      title: 'Dashboard created',
      detail: dashboard.name || 'Forge Dashboard',
    })
  }

  for (const source of sources) {
    if (source.timestamp) {
      const label = entityTypeLabels[source.typeId] ?? source.typeId
      entries.push({
        id: `${source.objectKey}:snapshot`,
        objectKey: source.objectKey,
        typeId: source.typeId,
        timestamp: source.timestamp,
        title: `${label} ${source.state ?? 'UPDATED'}`,
        detail: source.objectKey,
      })
    }

    for (const logEntry of source.logs ?? []) {
      const level = logEntry.level.toUpperCase()
      entries.push({
        id: `${source.objectKey}:log:${logEntry.timestamp.toISOString()}:${logEntry.message}`,
        objectKey: source.objectKey,
        typeId: source.typeId,
        timestamp: logEntry.timestamp,
        title: `Execution log ${level}`,
        detail: logEntry.message,
      })
    }
  }

  entries.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime())
  return entries.slice(0, 20)
}

// useForgeDashboardWork follows the objects a dashboard links and derives
// the dashboard's activity from their bodies. A live World wakes the hook only
// on a change to the links or to a linked object. A World snapshot is read
// once.
export function useForgeDashboardWork(
  worldState: Resource<IWorldState>,
  objectKey: string,
  dashboard: ForgeDashboard | undefined,
): {
  members: Resource<KeyedSet<ForgeDashboardMember>>
  entities: ForgeLinkedEntity[]
  entries: ForgeDashboardActivityEntry[]
} {
  const members = useStreamingResource(
    worldState,
    async function* (world, signal) {
      const source = forgeDashboardSource(objectKey)
      const engine = world.getEngine?.()
      if (engine) {
        yield* watchKeyedSet(engine, source, signal)
        return
      }
      if (!(world instanceof WorldStateResource)) {
        throw new Error('This World can not list its objects.')
      }
      yield await readKeyedSet(world, source, signal)
    },
    [
      entitySetKey({
        case: 'link',
        origin: objectKey,
        predicate: PRED_DASHBOARD_FORGE_REF,
        direction: 'out',
      }),
    ],
    { reconnect: true },
  )
  const set = resourceContent(members)
  const entities = useMemo(
    () =>
      set?.keys.flatMap((key) => {
        const member = set.rows.get(key)
        return member ? [{ objectKey: key, typeId: member.typeId }] : []
      }) ?? [],
    [set],
  )
  const createdAt = dashboard?.createdAt?.getTime()
  const name = dashboard?.name
  const entries = useMemo(
    () =>
      buildForgeDashboardActivityEntries(
        {
          createdAt: createdAt === undefined ? undefined : new Date(createdAt),
          name,
        },
        set?.keys.flatMap((key) => set.rows.get(key)?.source ?? []) ?? [],
      ),
    [createdAt, name, set],
  )
  return { members, entities, entries }
}
