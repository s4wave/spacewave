import { useSyncExternalStore } from 'react'

import {
  projectBrowserStartup,
  withBrowserBootProgress,
  type BrowserBootStatus,
  type BrowserStartupMark,
} from '@s4wave/app/loading/status/browser-startup-model.js'

export const bootStatusEvent = 'spacewave:boot-status'
export const browserStartupMarkPrefix = 'spacewave.startup.'
export const browserStartupMarkEvent = 'spacewave-startup-mark'

export type { BrowserBootStatus, BrowserStartupMark }

const defaultStatus: BrowserBootStatus = {
  phase: 'loading',
  detail: 'Loading application...',
  state: 'loading',
  progress: 0.04,
}

declare global {
  var __swBootStatus: BrowserBootStatus | undefined
  var __swStartupMarks: BrowserStartupMark[] | undefined
  var __swStartupMarkSequence: number | undefined
}

let nextBrowserStartupMarkSequence = 1
let browserStartupRevision = 0

export function readBrowserBootStatus(): BrowserBootStatus {
  return globalThis.__swBootStatus ?? defaultStatus
}

export function readBrowserStartupRevision(): number {
  return browserStartupRevision
}

export function readBrowserStartupMarks(): BrowserStartupMark[] {
  return globalThis.__swStartupMarks ?? []
}

// canMutateBrowserBootStatusTarget returns true when boot scripts may update a
// status node without racing React hydration.
export function canMutateBrowserBootStatusTarget(
  target: Element | null,
): target is Element {
  if (!target) return false
  const root = target.closest('#bldr-root[data-prerendered]')
  if (!root) return true
  return !!target.closest('#sw-loading')
}

function updateProgressTarget(target: Element | null, progress: number) {
  if (!canMutateBrowserBootStatusTarget(target)) return
  const pct = Math.round(progress * 100)
  if (target instanceof HTMLElement) {
    target.style.width = `${pct}%`
  }
  target.setAttribute('aria-valuenow', String(pct))
}

function updateProgressLabel(target: Element | null, progress: number) {
  if (!canMutateBrowserBootStatusTarget(target)) return
  target.replaceChildren(`${Math.round(progress * 100)}%`)
}

function updateStaticPhaseRail(
  phases: ReturnType<typeof projectBrowserStartup>['phases'],
) {
  for (const phase of phases) {
    const target = document.querySelector(`[data-sw-boot-phase="${phase.id}"]`)
    if (!canMutateBrowserBootStatusTarget(target)) continue
    target.setAttribute('data-sw-boot-phase-state', phase.state)

    const dot = target.querySelector<HTMLElement>('[data-sw-boot-phase-dot]')
    if (dot) {
      dot.style.background =
        phase.state === 'error'
          ? 'var(--color-destructive,#ef4444)'
          : phase.state === 'pending'
            ? 'color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)'
            : 'var(--color-brand,var(--color-logo-blue,#4f8cff))'
    }

    const label = target.querySelector<HTMLElement>(
      '[data-sw-boot-phase-label]',
    )
    if (label) {
      label.style.color =
        phase.state === 'error'
          ? 'var(--color-destructive,#ef4444)'
          : phase.state === 'current'
            ? 'var(--color-foreground,#fafafa)'
            : phase.state === 'complete'
              ? 'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)'
              : 'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)'
    }
  }
}

function retryBrowserStartup() {
  markBrowserStartupBoundary('boot-status.retry', {
    source: 'browser',
  })
  window.location.reload()
}

function leaveBrowserStartup() {
  markBrowserStartupBoundary('boot-status.back', {
    source: 'browser',
  })
  if (window.history.length > 1) {
    window.history.back()
    return
  }
  localStorage.removeItem('spacewave-has-session')
  window.location.assign('/')
}

function updateStaticErrorState(
  view: ReturnType<typeof projectBrowserStartup>['view'],
) {
  const errorTarget = document.querySelector('[data-sw-boot-error]')
  if (canMutateBrowserBootStatusTarget(errorTarget)) {
    errorTarget.replaceChildren(view.error ?? '')
    if (errorTarget instanceof HTMLElement) {
      errorTarget.style.display = view.error ? '' : 'none'
    }
  }

  const actionsTarget = document.querySelector('[data-sw-boot-error-actions]')
  if (canMutateBrowserBootStatusTarget(actionsTarget)) {
    if (actionsTarget instanceof HTMLElement) {
      actionsTarget.style.display = view.state === 'error' ? 'flex' : 'none'
    }
  }

  const retryTarget = document.querySelector('[data-sw-boot-retry]')
  if (
    retryTarget instanceof HTMLButtonElement &&
    canMutateBrowserBootStatusTarget(retryTarget)
  ) {
    retryTarget.onclick = retryBrowserStartup
  }

  const backTarget = document.querySelector('[data-sw-boot-back]')
  if (
    backTarget instanceof HTMLButtonElement &&
    canMutateBrowserBootStatusTarget(backTarget)
  ) {
    backTarget.onclick = leaveBrowserStartup
  }
}

export function writeBrowserBootStatus(status: BrowserBootStatus): void {
  const next = withBrowserBootProgress(status)
  const startup = projectBrowserStartup(next, readBrowserStartupMarks())
  const view = startup.view
  globalThis.__swBootStatus = next
  browserStartupRevision++

  const detailTarget = document.querySelector('[data-sw-boot-status]')
  if (canMutateBrowserBootStatusTarget(detailTarget)) {
    detailTarget.replaceChildren(view.detail ?? '')
  }

  const stateTarget = document.querySelector('[data-sw-boot-state]')
  if (canMutateBrowserBootStatusTarget(stateTarget)) {
    stateTarget.setAttribute('data-sw-boot-state', next.state)
  }

  if (view.progress !== undefined) {
    updateProgressTarget(
      document.querySelector('[data-sw-boot-progress]'),
      view.progress,
    )
    updateProgressLabel(
      document.querySelector('[data-sw-boot-progress-label]'),
      view.progress,
    )
  }
  updateStaticPhaseRail(startup.phases)
  updateStaticErrorState(view)

  markBrowserStartupBoundary(`boot-status.${next.phase}`, {
    source: 'browser',
    phase: next.phase,
    state: next.state,
    progress: next.progress,
  })
  window.dispatchEvent(new CustomEvent(bootStatusEvent, { detail: next }))
}

export function markBrowserStartupBoundary(
  label: string,
  detail: Record<string, unknown> = {},
): string {
  const name = `${browserStartupMarkPrefix}${label}`
  const sequence =
    globalThis.__swStartupMarkSequence ?? nextBrowserStartupMarkSequence
  globalThis.__swStartupMarkSequence = sequence + 1
  nextBrowserStartupMarkSequence = sequence + 1
  const markDetail = {
    ...detail,
    label,
    sequence,
  }
  const mark: BrowserStartupMark = {
    name,
    label,
    sequence,
    detail: markDetail,
  }
  globalThis.__swStartupMarks = [...readBrowserStartupMarks(), mark]
  browserStartupRevision++
  if (typeof globalThis.performance?.mark === 'function') {
    try {
      globalThis.performance.mark(name, { detail: markDetail })
    } catch {
      globalThis.performance.mark(name)
    }
  }
  if (
    typeof globalThis.dispatchEvent === 'function' &&
    typeof globalThis.CustomEvent === 'function'
  ) {
    globalThis.dispatchEvent(
      new CustomEvent(browserStartupMarkEvent, {
        detail: { name, detail: markDetail },
      }),
    )
  }
  return name
}

export function resetBrowserStartupMarksForTest(): void {
  nextBrowserStartupMarkSequence = 1
  globalThis.__swStartupMarkSequence = undefined
  globalThis.__swStartupMarks = undefined
  browserStartupRevision = 0
}

export function subscribeBrowserBootStatus(callback: () => void): () => void {
  const handleStartupMark = () => {
    browserStartupRevision++
    callback()
  }
  window.addEventListener(bootStatusEvent, callback)
  window.addEventListener(browserStartupMarkEvent, handleStartupMark)
  return () => {
    window.removeEventListener(bootStatusEvent, callback)
    window.removeEventListener(browserStartupMarkEvent, handleStartupMark)
  }
}

export function useBrowserBootStatus(): BrowserBootStatus {
  return useSyncExternalStore(
    subscribeBrowserBootStatus,
    readBrowserBootStatus,
    () => defaultStatus,
  )
}
