import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  markBrowserStartupBoundary,
  readBrowserBootStatus,
  resetBrowserStartupMarksForTest,
} from '@s4wave/app/prerender/boot-status.js'
import {
  armBrowserBootWatchdog,
  browserBootFrameReadyProgress,
  defaultBrowserBootWatchdogCeilingMs,
  evaluateBrowserBootStall,
  readBrowserBootWatchdogCeilingMs,
} from './browser-boot-watchdog.js'

describe('browser boot watchdog', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(0)
    resetBrowserStartupMarksForTest()
    delete globalThis.__swBootStatus
    delete globalThis.__swBootWatchdogCeilingMs
  })

  afterEach(() => {
    vi.useRealTimers()
    resetBrowserStartupMarksForTest()
    delete globalThis.__swBootStatus
    delete globalThis.__swBootWatchdogCeilingMs
  })

  it('writes an error status at the ceiling when stuck below the threshold', () => {
    readBrowserBootStatus()
    markBrowserStartupBoundary('webview.registered', { source: 'test' })
    expect(readBrowserBootStatus().state).toBe('loading')

    const cancel = armBrowserBootWatchdog({ ceilingMs: 5000 })
    vi.advanceTimersByTime(4999)
    expect(readBrowserBootStatus().state).toBe('loading')

    vi.advanceTimersByTime(1)
    const status = readBrowserBootStatus()
    expect(status.state).toBe('error')
    expect(status.phase).toBe('browser-boot')
    expect(status.detail).toContain('webview.registered')
    expect(status.detail).toContain('5s')
    expect(status.progress).toBeLessThan(browserBootFrameReadyProgress)

    cancel()
  })

  it('cancels without writing when progress crosses the threshold', () => {
    markBrowserStartupBoundary('webview.registered', { source: 'test' })
    const cancel = armBrowserBootWatchdog({ ceilingMs: 5000 })

    vi.advanceTimersByTime(1000)
    markBrowserStartupBoundary('worker.first-ready', { source: 'test' })
    expect(evaluateBrowserBootStall(0, Date.now())).toBeUndefined()

    vi.advanceTimersByTime(10000)
    expect(readBrowserBootStatus().state).toBe('loading')
    cancel()
  })

  it('does nothing after the timer is cancelled', () => {
    markBrowserStartupBoundary('webview.registered', { source: 'test' })
    const cancel = armBrowserBootWatchdog({ ceilingMs: 5000 })
    cancel()

    vi.advanceTimersByTime(defaultBrowserBootWatchdogCeilingMs * 2)
    expect(readBrowserBootStatus().state).toBe('loading')
  })

  it('does not double-write when boot already failed', () => {
    globalThis.__swBootStatus = {
      phase: 'runtime-error',
      detail: 'runtime channel failed',
      state: 'error',
    }
    const before = readBrowserBootStatus()
    const cancel = armBrowserBootWatchdog({ ceilingMs: 5000 })

    vi.advanceTimersByTime(defaultBrowserBootWatchdogCeilingMs)
    const after = readBrowserBootStatus()
    expect(after).toEqual(before)
    expect(evaluateBrowserBootStall(0, Date.now())).toBeUndefined()
    cancel()
  })

  it('resolves the ceiling from the option, test hook, then default', () => {
    expect(readBrowserBootWatchdogCeilingMs()).toBe(
      defaultBrowserBootWatchdogCeilingMs,
    )
    globalThis.__swBootWatchdogCeilingMs = 1234
    expect(readBrowserBootWatchdogCeilingMs()).toBe(1234)
    expect(readBrowserBootWatchdogCeilingMs(50)).toBe(50)
    delete globalThis.__swBootWatchdogCeilingMs
    expect(readBrowserBootWatchdogCeilingMs(-1)).toBe(
      defaultBrowserBootWatchdogCeilingMs,
    )
  })
})
