import { useEffect } from 'react'

import { projectBootProgress } from '@aptre/bldr'

import {
  readBrowserBootStatus,
  readBrowserStartupMarks,
  writeBrowserBootStatus,
  type BrowserBootStatus,
} from '@s4wave/app/prerender/boot-status.js'
import { useBrowserStartupProjection } from './browser-startup.js'

// browserBootFrameReadyProgress is the boot ladder position of the
// worker.first-ready rung (bldr/web/bldr/boot-progress.ts): the first mark
// proving the app frame opened and its plugins started. The watchdog treats
// any projection below this rung as not yet past webview.registered.
export const browserBootFrameReadyProgress = 0.86

// defaultBrowserBootWatchdogCeilingMs is the hard ceiling for a boot that
// never crosses the app-frame threshold: past it the outer shell fails loudly
// instead of stalling silently.
export const defaultBrowserBootWatchdogCeilingMs = 5 * 60 * 1000

declare global {
  // __swBootWatchdogCeilingMs overrides the watchdog ceiling from tests and
  // end-to-end harnesses without changing code.
  var __swBootWatchdogCeilingMs: number | undefined
}

export interface BrowserBootWatchdogOptions {
  ceilingMs?: number
  now?: () => number
}

// readBrowserBootWatchdogCeilingMs resolves the watchdog ceiling: the explicit
// option, then the __swBootWatchdogCeilingMs test hook, then the default.
export function readBrowserBootWatchdogCeilingMs(override?: number): number {
  if (override !== undefined && override > 0) return override
  const injected = globalThis.__swBootWatchdogCeilingMs
  if (typeof injected === 'number' && injected > 0) return injected
  return defaultBrowserBootWatchdogCeilingMs
}

// evaluateBrowserBootStall returns the terminal failure status to synthesize
// when boot stalled below the app-frame threshold since startedAtMs, or
// undefined when boot already reached a terminal state or crossed the
// threshold.
export function evaluateBrowserBootStall(
  startedAtMs: number,
  nowMs: number,
): BrowserBootStatus | undefined {
  const status = readBrowserBootStatus()
  if (status.state !== 'loading') return undefined
  const marks = readBrowserStartupMarks()
  const step = projectBootProgress(status, marks)
  if (step.progress >= browserBootFrameReadyProgress) return undefined
  return {
    phase: 'browser-boot',
    detail:
      `Startup stalled before the app frame after ${formatElapsedMs(nowMs - startedAtMs)}; ` +
      `last mark ${marks.at(-1)?.label ?? step.label} (${step.label}).`,
    state: 'error',
    progress: step.progress,
  }
}

// armBrowserBootWatchdog schedules one check after the ceiling and writes the
// synthesized terminal boot failure when boot remains stuck below the
// app-frame threshold. The returned function cancels the timer.
export function armBrowserBootWatchdog(
  options: BrowserBootWatchdogOptions = {},
): () => void {
  const now = options.now ?? Date.now
  const startedAtMs = now()
  let cancelled = false
  const timer = setTimeout(() => {
    if (cancelled) return
    const failure = evaluateBrowserBootStall(startedAtMs, now())
    if (failure) writeBrowserBootStatus(failure)
  }, readBrowserBootWatchdogCeilingMs(options.ceilingMs))
  return () => {
    cancelled = true
    clearTimeout(timer)
  }
}

// useBrowserBootWatchdog arms the outer-shell watchdog while the loading
// screen projects a non-terminal state below the app-frame threshold. It
// cancels as soon as the projection crosses the threshold or turns terminal.
export function useBrowserBootWatchdog(): void {
  const { view } = useBrowserStartupProjection()
  const armed =
    view.state === 'loading' &&
    (view.progress === undefined ||
      view.progress < browserBootFrameReadyProgress)
  useEffect(() => {
    if (!armed) return
    return armBrowserBootWatchdog()
  }, [armed])
}

// formatElapsedMs renders an elapsed duration as a compact m/s string.
function formatElapsedMs(elapsedMs: number): string {
  const seconds = Math.max(0, Math.round(elapsedMs / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return rest === 0 ? `${minutes}m` : `${minutes}m${rest}s`
}
