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
