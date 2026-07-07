export const driveSpaceOpenTaskName = 'browser/dashboard/drive-space-open'

export type DriveSpaceOpenRegion =
  | 'mount'
  | 'so-fetch'
  | 'world-open'
  | 'first-listing-render'

export interface DriveSpaceOpenTraceStart {
  sessionIndex: number
  sharedObjectId: string
  spaceName: string
  orgId?: string | undefined
  source?: string | undefined
}

export interface DriveSpaceOpenTraceDetail {
  [key: string]: unknown
}

type ActiveRegion = {
  markName: string
  regionName: string
  ended: boolean
}

type ActiveTask = {
  id: number
  sharedObjectId: string
  startMarkName: string
  regions: Map<DriveSpaceOpenRegion, ActiveRegion>
  ended: boolean
}

declare global {
  var __s4waveDriveSpaceOpenTraceSeq: number | undefined
  var __s4waveDriveSpaceOpenTraceTasks: Map<string, ActiveTask> | undefined
}

function getPerformance(): Performance | undefined {
  return typeof globalThis.performance?.mark === 'function'
    ? globalThis.performance
    : undefined
}

function nextTraceId(): number {
  const next = globalThis.__s4waveDriveSpaceOpenTraceSeq ?? 1
  globalThis.__s4waveDriveSpaceOpenTraceSeq = next + 1
  return next
}

function activeTasks(): Map<string, ActiveTask> {
  const tasks = globalThis.__s4waveDriveSpaceOpenTraceTasks ?? new Map()
  globalThis.__s4waveDriveSpaceOpenTraceTasks = tasks
  return tasks
}

function traceDetail(
  kind: 'Task' | 'Region',
  label: string,
  detail: DriveSpaceOpenTraceDetail,
): DriveSpaceOpenTraceDetail {
  return {
    source: 'app',
    traceKind: kind,
    label,
    ...detail,
  }
}

function mark(name: string, detail: DriveSpaceOpenTraceDetail): void {
  const perf = getPerformance()
  if (!perf) return
  try {
    perf.mark(name, { detail })
  } catch {
    perf.mark(name)
  }
}

function measure(
  name: string,
  start: string,
  end: string,
  detail: DriveSpaceOpenTraceDetail,
): void {
  const perf = getPerformance()
  if (!perf || typeof perf.measure !== 'function') return
  try {
    perf.measure(name, { start, end, detail })
  } catch {
    try {
      perf.measure(name, start, end)
    } catch {
      // Some test and embedded runtimes keep a small mark buffer. Tracing must
      // never affect the user-visible open path.
    }
  }
}

function taskFor(sharedObjectId: string): ActiveTask | undefined {
  if (!sharedObjectId) return undefined
  return activeTasks().get(sharedObjectId)
}

function endTaskWithOutcome(
  sharedObjectId: string,
  outcome: string,
  detail: DriveSpaceOpenTraceDetail = {},
): void {
  const task = taskFor(sharedObjectId)
  if (!task || task.ended) return
  for (const region of task.regions.keys()) {
    endDriveSpaceOpenRegion(sharedObjectId, region, { outcome })
  }
  const endMarkName = `spacewave.trace.task.${driveSpaceOpenTaskName}.${task.id}.end`
  const measureName = `spacewave.trace.task.${driveSpaceOpenTaskName}`
  mark(
    endMarkName,
    traceDetail('Task', driveSpaceOpenTaskName, {
      taskId: task.id,
      sharedObjectId,
      outcome,
      ...detail,
    }),
  )
  measure(
    measureName,
    task.startMarkName,
    endMarkName,
    traceDetail('Task', driveSpaceOpenTaskName, {
      taskId: task.id,
      sharedObjectId,
      outcome,
      ...detail,
    }),
  )
  task.ended = true
  activeTasks().delete(sharedObjectId)
}

export function startDriveSpaceOpenTrace({
  sessionIndex,
  sharedObjectId,
  spaceName,
  orgId,
  source,
}: DriveSpaceOpenTraceStart): void {
  if (!sharedObjectId) return
  endTaskWithOutcome(sharedObjectId, 'replaced')
  const id = nextTraceId()
  const startMarkName = `spacewave.trace.task.${driveSpaceOpenTaskName}.${id}.start`
  const task: ActiveTask = {
    id,
    sharedObjectId,
    startMarkName,
    regions: new Map(),
    ended: false,
  }
  activeTasks().set(sharedObjectId, task)
  mark(
    startMarkName,
    traceDetail('Task', driveSpaceOpenTaskName, {
      taskId: id,
      sessionIndex,
      sharedObjectId,
      spaceName,
      orgId,
      source,
    }),
  )
  beginDriveSpaceOpenRegion(sharedObjectId, 'mount', {
    sessionIndex,
    spaceName,
    orgId,
    source,
  })
}

export function beginDriveSpaceOpenRegion(
  sharedObjectId: string,
  region: DriveSpaceOpenRegion,
  detail: DriveSpaceOpenTraceDetail = {},
): void {
  const task = taskFor(sharedObjectId)
  if (!task || task.ended) return
  const existing = task.regions.get(region)
  if (existing && !existing.ended) return
  const regionName = `${driveSpaceOpenTaskName}/${region}`
  const markName = `spacewave.trace.region.${regionName}.${task.id}.start`
  task.regions.set(region, {
    markName,
    regionName,
    ended: false,
  })
  mark(
    markName,
    traceDetail('Region', regionName, {
      taskId: task.id,
      sharedObjectId,
      ...detail,
    }),
  )
}

export function endDriveSpaceOpenRegion(
  sharedObjectId: string,
  region: DriveSpaceOpenRegion,
  detail: DriveSpaceOpenTraceDetail = {},
): void {
  const task = taskFor(sharedObjectId)
  if (!task || task.ended) return
  const activeRegion = task.regions.get(region)
  if (!activeRegion || activeRegion.ended) return
  const endMarkName = `spacewave.trace.region.${activeRegion.regionName}.${task.id}.end`
  mark(
    endMarkName,
    traceDetail('Region', activeRegion.regionName, {
      taskId: task.id,
      sharedObjectId,
      ...detail,
    }),
  )
  measure(
    `spacewave.trace.region.${activeRegion.regionName}`,
    activeRegion.markName,
    endMarkName,
    traceDetail('Region', activeRegion.regionName, {
      taskId: task.id,
      sharedObjectId,
      ...detail,
    }),
  )
  activeRegion.ended = true
}

export function endDriveSpaceOpenTrace(
  sharedObjectId: string,
  detail: DriveSpaceOpenTraceDetail = {},
): void {
  endTaskWithOutcome(sharedObjectId, 'complete', detail)
}

export function abortDriveSpaceOpenTrace(
  sharedObjectId: string,
  detail: DriveSpaceOpenTraceDetail = {},
): void {
  endTaskWithOutcome(sharedObjectId, 'aborted', detail)
}

export function resetDriveSpaceOpenTraceForTest(): void {
  globalThis.__s4waveDriveSpaceOpenTraceSeq = undefined
  globalThis.__s4waveDriveSpaceOpenTraceTasks = undefined
}
