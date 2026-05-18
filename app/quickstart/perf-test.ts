/* eslint-disable react-doctor/async-await-in-loop */
import type { Root } from '@s4wave/sdk/root'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { createLocalSession } from './create.js'
import { mountSpace } from '@s4wave/app/space/space.js'

export interface PerfTestResult {
  createSessionMs: number
  createSpaceMs: number
  mountSpaceMs: number
  accessWorldMs: number
  opTimingsMs: number[]
  opTotalMs: number
  opAvgMs: number
  opMinMs: number
  opMaxMs: number
  opsPerSec: number
  opCount: number
}

export interface PostLoadSOPerfTarget {
  routePath: string
  sessionIndex: number
  sharedObjectId: string
  indexPath: string
}

export interface AcceptedSOTiming {
  ordinal: number
  startedMs: number
  finishedMs: number
  elapsedMs: number
  seqno: string
}

export interface PostLoadSOPerfResult {
  scenario: 'quickstart-post-load-shared-object-throughput'
  target: PostLoadSOPerfTarget
  operationTypeId: string
  operationSemantics: 'sequential-single-operation'
  opCount: number
  startedMs: number
  finishedMs: number
  totalMs: number
  opAvgMs: number
  opMinMs: number
  opMaxMs: number
  opsPerSec: number
  startingSeqno: string
  endingSeqno: string
  acceptedOperationTimings: AcceptedSOTiming[]
}

function roundMs(value: number): number {
  return Math.round(value * 1000) / 1000
}

function currentRoutePath(): string {
  const hash = window.location.hash
  if (hash.startsWith('#/')) return hash.slice(1)
  return window.location.pathname + window.location.search
}

function currentPostLoadSOTarget(): PostLoadSOPerfTarget {
  const routePath = currentRoutePath()
  const match = routePath.match(
    /^\/u\/([1-9]\d*)\/(?:org\/[^/]+\/)?so\/([^/?#]+)(?:\/-\/([^?#]*))?/,
  )
  if (!match) {
    throw new Error('current route is not a mounted Space route: ' + routePath)
  }
  return {
    routePath,
    sessionIndex: Number(match[1]),
    sharedObjectId: decodeURIComponent(match[2]),
    indexPath: decodeURIComponent(match[3] ?? ''),
  }
}

// runPostLoadSOPerfTest remounts the current quickstart-created SharedObject and
// measures sequential accepted operation latency after the UI has reached ready.
export async function runPostLoadSOPerfTest(
  root: Root,
  opCount: number,
  signal: AbortSignal,
): Promise<PostLoadSOPerfResult> {
  if (opCount <= 0) {
    throw new Error('post-load SO workload opCount must be greater than zero')
  }
  const target = currentPostLoadSOTarget()
  const cleanups: Array<{ [Symbol.dispose](): void }> = []
  const cleanup = <T extends { [Symbol.dispose](): void } | null | undefined>(
    resource: T,
  ): T => {
    if (resource) cleanups.push(resource)
    return resource
  }

  try {
    const mounted = await root.mountSessionByIdx(
      { sessionIdx: target.sessionIndex },
      signal,
    )
    if (!mounted) {
      throw new Error(
        'session not found for post-load SO workload: ' +
          target.sessionIndex.toString(),
      )
    }
    const session = cleanup(mounted.session)
    const space = await mountSpace({
      session,
      spaceResp: {
        sharedObjectRef: {
          providerResourceRef: { id: target.sharedObjectId },
        },
      },
      abortSignal: signal,
      cleanup,
    })
    const spaceWorld = await space.accessWorldState(true, signal)
    const initial = await spaceWorld.getSeqno(signal)
    const startedMs = performance.now()
    const acceptedOperationTimings: AcceptedSOTiming[] = []

    for (let i = 0; i < opCount; i++) {
      const op: SetSpaceSettingsOp = {
        objectKey: 'settings',
        settings: { indexPath: target.indexPath },
        overwrite: true,
        timestamp: new Date(),
      }
      const opData = SetSpaceSettingsOp.toBinary(op)
      const opStarted = performance.now()
      const resp = await spaceWorld.applyWorldOp(
        SET_SPACE_SETTINGS_OP_ID,
        opData,
        '',
        signal,
      )
      const opFinished = performance.now()
      if (resp.sysErr) {
        throw new Error(
          'post-load SO workload returned sysErr at operation ' +
            (i + 1).toString(),
        )
      }
      acceptedOperationTimings.push({
        ordinal: i + 1,
        startedMs: roundMs(opStarted),
        finishedMs: roundMs(opFinished),
        elapsedMs: roundMs(opFinished - opStarted),
        seqno: resp.seqno.toString(),
      })
    }

    const finishedMs = performance.now()
    const elapsed = acceptedOperationTimings.map((op) => op.elapsedMs)
    const opTotalMs = elapsed.reduce((a, b) => a + b, 0)
    const opMinMs = Math.min(...elapsed)
    const opMaxMs = Math.max(...elapsed)
    const ending =
      acceptedOperationTimings[acceptedOperationTimings.length - 1]?.seqno ??
      initial.seqno.toString()

    return {
      scenario: 'quickstart-post-load-shared-object-throughput',
      target,
      operationTypeId: SET_SPACE_SETTINGS_OP_ID,
      operationSemantics: 'sequential-single-operation',
      opCount: acceptedOperationTimings.length,
      startedMs: roundMs(startedMs),
      finishedMs: roundMs(finishedMs),
      totalMs: roundMs(finishedMs - startedMs),
      opAvgMs: roundMs(opTotalMs / acceptedOperationTimings.length),
      opMinMs,
      opMaxMs,
      opsPerSec: roundMs((acceptedOperationTimings.length / opTotalMs) * 1000),
      startingSeqno: initial.seqno.toString(),
      endingSeqno: ending,
      acceptedOperationTimings,
    }
  } finally {
    for (const c of cleanups) {
      try {
        c[Symbol.dispose]()
      } catch (err) {
        console.error('cleanup failed', err)
      }
    }
  }
}

// runSOPerfTest creates a local session, space, and times SO operations.
// Designed to be called from page.evaluate via __s4wave_debug.
export async function runSOPerfTest(
  root: Root,
  opCount: number,
  signal: AbortSignal,
): Promise<PerfTestResult> {
  const cleanups: Array<{ [Symbol.dispose](): void }> = []
  const cleanup = <T extends { [Symbol.dispose](): void } | null | undefined>(
    resource: T,
  ): T => {
    if (resource) cleanups.push(resource)
    return resource
  }

  try {
    // Create local session.
    const t0 = performance.now()
    const { session } = await createLocalSession(root, signal, cleanup)
    const createSessionMs = performance.now() - t0

    // Create space.
    const t1 = performance.now()
    const spaceResp = await session.createSpace(
      { spaceName: 'perf-test' },
      signal,
    )
    const createSpaceMs = performance.now() - t1

    // Mount space.
    const t2 = performance.now()
    const space = await mountSpace({
      session,
      spaceResp,
      abortSignal: signal,
      cleanup,
    })
    const mountSpaceMs = performance.now() - t2

    // Access world state.
    const t3 = performance.now()
    const spaceWorld = await space.accessWorldState(true, signal)
    const accessWorldMs = performance.now() - t3

    // Time individual SO operations.
    const opTimingsMs: number[] = []
    for (let i = 0; i < opCount; i++) {
      const op: SetSpaceSettingsOp = {
        objectKey: 'settings',
        settings: { indexPath: `perf-test-${i}` },
        overwrite: true,
        timestamp: new Date(),
      }
      const opData = SetSpaceSettingsOp.toBinary(op)

      const start = performance.now()
      await spaceWorld.applyWorldOp(
        SET_SPACE_SETTINGS_OP_ID,
        opData,
        '',
        signal,
      )
      opTimingsMs.push(performance.now() - start)
    }

    const opTotalMs = opTimingsMs.reduce((a, b) => a + b, 0)
    const opAvgMs = opTotalMs / opTimingsMs.length
    const opMinMs = Math.min(...opTimingsMs)
    const opMaxMs = Math.max(...opTimingsMs)
    const opsPerSec = (opTimingsMs.length / opTotalMs) * 1000

    return {
      createSessionMs: Math.round(createSessionMs),
      createSpaceMs: Math.round(createSpaceMs),
      mountSpaceMs: Math.round(mountSpaceMs),
      accessWorldMs: Math.round(accessWorldMs),
      opTimingsMs: opTimingsMs.map(Math.round),
      opTotalMs: Math.round(opTotalMs),
      opAvgMs: Math.round(opAvgMs),
      opMinMs: Math.round(opMinMs),
      opMaxMs: Math.round(opMaxMs),
      opsPerSec: Math.round(opsPerSec * 10) / 10,
      opCount: opTimingsMs.length,
    }
  } finally {
    for (const c of cleanups) {
      try {
        c[Symbol.dispose]()
      } catch (err) {
        console.error('cleanup failed', err)
      }
    }
  }
}
