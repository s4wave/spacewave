import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { Execution } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import { Job } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { Pass } from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'
import { Task } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { ForgeDashboard } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
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

interface ExecutionLogEntry {
  timestamp?: Date
  level?: string
  message?: string
}

export interface ForgeDashboardActivityEntry {
  id: string
  objectKey?: string
  typeId?: string
  timestamp: Date
  title: string
  detail: string
}

function describeActivitySource(
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
        logs: (execution.logEntries ?? [])
          .filter(
            (entry): entry is ExecutionLogEntry & { timestamp: Date } =>
              !!entry.timestamp,
          )
          .map((entry) => ({
            timestamp: entry.timestamp,
            level: entry.level ?? 'info',
            message: entry.message ?? '',
          })),
      }
    }
  }
  return null
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

export function useForgeDashboardActivity(
  worldState: Resource<IWorldState>,
  dashboard: ForgeDashboard | undefined,
  entities: ForgeLinkedEntity[],
): { entries: ForgeDashboardActivityEntry[]; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world, signal) => {
      if (!world) return []

      const sources = await Promise.all(
        entities.map(async (entity) => {
          using objectState = await world.getObject(entity.objectKey, signal)
          if (!objectState) return null
          using cursor = await objectState.accessWorldState(undefined, signal)
          const resp = await cursor.unmarshal({}, signal)
          if (!resp.found || !resp.data?.length) return null
          return describeActivitySource(entity, resp.data)
        }),
      )

      return buildForgeDashboardActivityEntries(
        dashboard,
        sources.filter(
          (source): source is ForgeDashboardActivitySource => source !== null,
        ),
      )
    },
    [dashboard?.createdAt?.toISOString(), dashboard?.name, entities],
  )

  return useMemo(
    () => ({
      entries: resource.value ?? [],
      loading: resource.loading,
    }),
    [resource.loading, resource.value],
  )
}
