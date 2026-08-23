import { describe, expect, it, vi } from 'vitest'

import { BootReport, BootReportState } from './report.pb.js'
import { initBootReportCollector } from './collector.js'
import { openBootReportStore } from './store.js'

// These specs run in the real browser project against Chromium's native
// IndexedDB and Web Locks APIs. They prove the two behaviors unit doubles
// cannot fully exercise: cross-store boot-lock contention and persistence of
// a boot that seals before the durable store finishes opening.

type BootReportStoreHandle = Awaited<ReturnType<typeof openBootReportStore>>

function uniqueReportId(): string {
  return `boot-report-e2e-${crypto.randomUUID().replaceAll('-', '')}`
}

function recordingReport(reportId: string): BootReport {
  return BootReport.create({
    schemaVersion: 1,
    reportId,
    startedUnixMicros: BigInt(Date.now()) * 1000n,
    entrypointId: 'drive',
    usableMark: 'boot-status.app',
    state: BootReportState.RECORDING,
  })
}

// releaseAll seals every tracked holder so each acquired Web Lock resolves
// its hold callback even when an assertion failed mid-test; IndexedDB record
// deletion alone cannot release a lock.
async function releaseAll(
  holders: BootReportStoreHandle[],
  reportId: string,
): Promise<void> {
  for (const store of holders) {
    await store
      .sealReport(recordingReport(reportId), BootReportState.ABORTED)
      .catch(() => undefined)
    await store.delete(reportId).catch(() => undefined)
  }
}

describe('BootReport durability in a real browser', () => {
  it('resolves two-store boot-lock contention through native Web Locks', async () => {
    const reportId = uniqueReportId()
    const holders: BootReportStoreHandle[] = []
    try {
      const first = await openBootReportStore()
      await expect(first.holdBootLock(reportId)).resolves.toBe(true)
      holders.push(first)

      // A second store must learn denial from the manager itself, never
      // from an optimistic synchronous read.
      const second = await openBootReportStore()
      await expect(second.holdBootLock(reportId)).resolves.toBe(false)

      // sealReport releases the hold after the terminal commit so a later
      // store can acquire the same report's lock.
      await first.sealReport(recordingReport(reportId), BootReportState.READY)
      holders.pop()
      const third = await openBootReportStore()
      await expect(third.holdBootLock(reportId)).resolves.toBe(true)
      holders.push(third)
    } finally {
      await releaseAll(holders, reportId)
    }
  })

  it('persists a fast-sealed READY boot through the lazy durable attach', async () => {
    const globals = globalThis as {
      __swStartupMarks?: unknown[]
      __swBootReport?: BootReport
    }
    // The inline buffer already holds the usable mark, so the collector
    // seals during construction - before the lazy store open can finish.
    globals.__swStartupMarks = [
      {
        name: 'spacewave.startup.boot.started',
        label: 'boot.started',
        sequence: 1,
        detail: { source: 'browser' },
      },
      {
        name: 'spacewave.startup.boot-status.app',
        label: 'boot-status.app',
        sequence: 2,
        detail: {},
      },
    ]
    let collector: { stop(): void } | undefined
    try {
      collector = initBootReportCollector({
        entrypointId: 'drive',
        usableMark: 'boot-status.app',
      })
      expect(collector?.isSealed()).toBe(true)
      const reportId = globals.__swBootReport?.reportId ?? ''
      expect(reportId).toMatch(/^boot-report-/)

      const store = await openBootReportStore()
      await vi.waitFor(async () => {
        const stored = await store.get(reportId)
        expect(stored?.state).toBe(BootReportState.READY)
      })
      await store.delete(reportId)
    } finally {
      collector?.stop()
      delete globals.__swStartupMarks
      delete globals.__swBootReport
    }
  })
})
