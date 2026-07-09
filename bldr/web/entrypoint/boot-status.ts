import {
  bootProgressStallDelayMs,
  projectBootProgress,
} from '../bldr/boot-progress.js'
import {
  markStartupBoundary,
  readStartupMarks,
  startupMarkEvent,
} from '../bldr/startup-marks.js'

export interface BrowserBootStatus {
  phase: string
  detail: string
  state: 'loading' | 'error'
  progress?: number
}

export const bootStatusEvent = 'spacewave:boot-status'

declare global {
  var __swBootStatus: BrowserBootStatus | undefined
  var __swBootProgressActivity: (() => void) | undefined
}

function clampProgress(progress: number | undefined): number | undefined {
  if (progress === undefined || !Number.isFinite(progress)) return undefined
  return Math.max(0, Math.min(1, progress))
}

const startupPhaseInfo = {
  prepare: { label: 'Prepare' },
  connect: { label: 'Connect' },
  runtime: { label: 'Runtime' },
  frame: { label: 'App' },
  done: { label: 'Done' },
} as const

type StartupPhase = keyof typeof startupPhaseInfo

const startupPhaseOrder: StartupPhase[] = [
  'prepare',
  'connect',
  'runtime',
  'frame',
  'done',
]

const bootPhaseStartupPhase: Record<string, StartupPhase> = {
  loading: 'prepare',
  manifest: 'prepare',
  'manifest-ready': 'prepare',
  'manifest-error': 'prepare',
  wasm: 'connect',
  entrypoint: 'connect',
  'entrypoint-error': 'connect',
  runtime: 'runtime',
  ready: 'runtime',
  'runtime-error': 'runtime',
  app: 'frame',
}

function canMutateBootStatusTarget(target: Element | null): target is Element {
  if (!target) return false
  const root = target.closest('#bldr-root[data-prerendered]')
  if (!root) return true
  return !!target.closest('#sw-loading')
}

function startupDisplayForBootStatus(status: BrowserBootStatus) {
  const id = bootPhaseStartupPhase[status.phase] ?? 'prepare'
  const step = projectBootProgress(status, readStartupMarks())
  return {
    id,
    detail: `${startupPhaseInfo[id].label}: ${step.label}`,
    progress: step.progress,
    error: status.state === 'error',
  }
}

function withBootProgress(status: BrowserBootStatus): BrowserBootStatus {
  const progress = clampProgress(status.progress)
  if (progress === undefined) {
    return {
      phase: status.phase,
      detail: status.detail,
      state: status.state,
    }
  }
  return { ...status, progress }
}

function updateProgressTarget(target: Element | null, progress: number) {
  if (!canMutateBootStatusTarget(target)) return
  const pct = Math.round(progress * 100)
  if (target instanceof HTMLElement) {
    target.style.width = `${pct}%`
    target.style.transition = 'width 200ms'
    target.removeAttribute('data-sw-boot-progress-stalled')
  }
  target.setAttribute('aria-valuenow', String(pct))
}

function updateProgressLabel(target: Element | null, progress: number) {
  if (!canMutateBootStatusTarget(target)) return
  target.replaceChildren(`${Math.round(progress * 100)}%`)
}

function updateStaticPhaseRail(currentID: StartupPhase, bootState: string) {
  const currentIdx = startupPhaseOrder.indexOf(currentID)
  for (const [idx, phaseID] of startupPhaseOrder.entries()) {
    const target = document.querySelector(`[data-sw-boot-phase="${phaseID}"]`)
    if (!canMutateBootStatusTarget(target)) continue
    const phaseState =
      idx < currentIdx
        ? 'complete'
        : idx === currentIdx && bootState === 'error'
          ? 'error'
          : idx === currentIdx
            ? 'current'
            : 'pending'
    target.setAttribute('data-sw-boot-phase-state', phaseState)

    const dot = target.querySelector<HTMLElement>('[data-sw-boot-phase-dot]')
    if (dot) {
      dot.style.background =
        phaseState === 'error'
          ? 'var(--color-destructive,#ef4444)'
          : phaseState === 'pending'
            ? 'color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)'
            : 'var(--color-brand,var(--color-logo-blue,#4f8cff))'
    }

    const label = target.querySelector<HTMLElement>(
      '[data-sw-boot-phase-label]',
    )
    if (label) {
      label.style.color =
        phaseState === 'error'
          ? 'var(--color-destructive,#ef4444)'
          : phaseState === 'current'
            ? 'var(--color-foreground,#fafafa)'
            : phaseState === 'complete'
              ? 'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 85%,transparent)'
              : 'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 55%,transparent)'
    }
  }
}

function retryBrowserStartup() {
  markStartupBoundary('boot-status.retry', { source: 'browser' })
  window.location.reload()
}

function leaveBrowserStartup() {
  markStartupBoundary('boot-status.back', { source: 'browser' })
  if (window.history.length > 1) {
    window.history.back()
    return
  }
  localStorage.removeItem('spacewave-has-session')
  window.location.assign('/')
}

function updateStaticErrorState(status: BrowserBootStatus) {
  const errorTarget = document.querySelector('[data-sw-boot-error]')
  if (canMutateBootStatusTarget(errorTarget)) {
    errorTarget.replaceChildren(status.state === 'error' ? status.detail : '')
    if (errorTarget instanceof HTMLElement) {
      errorTarget.style.display = status.state === 'error' ? '' : 'none'
    }
  }

  const actionsTarget = document.querySelector('[data-sw-boot-error-actions]')
  if (canMutateBootStatusTarget(actionsTarget)) {
    if (actionsTarget instanceof HTMLElement) {
      actionsTarget.style.display = status.state === 'error' ? 'flex' : 'none'
    }
  }

  const retryTarget = document.querySelector('[data-sw-boot-retry]')
  if (
    retryTarget instanceof HTMLButtonElement &&
    canMutateBootStatusTarget(retryTarget)
  ) {
    retryTarget.onclick = retryBrowserStartup
  }

  const backTarget = document.querySelector('[data-sw-boot-back]')
  if (
    backTarget instanceof HTMLButtonElement &&
    canMutateBootStatusTarget(backTarget)
  ) {
    backTarget.onclick = leaveBrowserStartup
  }
}

function renderBrowserBootStatus(status: BrowserBootStatus): void {
  const display = startupDisplayForBootStatus(status)

  const detailTarget = document.querySelector('[data-sw-boot-status]')
  if (canMutateBootStatusTarget(detailTarget)) {
    detailTarget.replaceChildren(display.detail)
  }

  const stateTarget = document.querySelector('[data-sw-boot-state]')
  if (canMutateBootStatusTarget(stateTarget)) {
    stateTarget.setAttribute('data-sw-boot-state', status.state)
  }

  updateProgressTarget(
    document.querySelector('[data-sw-boot-progress]'),
    display.progress,
  )
  updateProgressLabel(
    document.querySelector('[data-sw-boot-progress-label]'),
    display.progress,
  )
  updateStaticPhaseRail(display.id, status.state)
  updateStaticErrorState(status)
}

export function writeBrowserBootStatus(status: BrowserBootStatus): void {
  const next = withBootProgress(status)
  globalThis.__swBootStatus = next
  renderBrowserBootStatus(next)

  markStartupBoundary(`boot-status.${next.phase}`, {
    source: 'browser',
    phase: next.phase,
    state: next.state,
    progress: next.progress,
  })
  window.dispatchEvent(new CustomEvent(bootStatusEvent, { detail: next }))
}

// bindBrowserBootStatusToStartupMarks re-renders the static boot shell on
// every startup mark and starts a one-shot quiet-window timer. Marks advance
// the bar and label immediately; a gap keeps the accumulated percentage but
// adds the indeterminate shimmer until the next mark. Returns the unsubscribe
// function.
export function bindBrowserBootStatusToStartupMarks(): () => void {
  let stallTimer: number | undefined
  const inlineActivity = globalThis.__swBootProgressActivity
  const clearLocalStall = () => {
    if (stallTimer !== undefined) {
      window.clearTimeout(stallTimer)
      stallTimer = undefined
    }
    const target = document.querySelector('[data-sw-boot-progress]')
    if (canMutateBootStatusTarget(target)) {
      target.removeAttribute('data-sw-boot-progress-stalled')
    }
  }
  const scheduleLocalStall = () => {
    clearLocalStall()
    const status = globalThis.__swBootStatus
    if (!status || status.state !== 'loading') return
    if (projectBootProgress(status, readStartupMarks()).progress >= 1) return
    stallTimer = window.setTimeout(() => {
      stallTimer = undefined
      const current = globalThis.__swBootStatus
      if (!current || current.state !== 'loading') return
      if (projectBootProgress(current, readStartupMarks()).progress >= 1) return
      const target = document.querySelector('[data-sw-boot-progress]')
      if (canMutateBootStatusTarget(target)) {
        target.setAttribute('data-sw-boot-progress-stalled', '')
      }
    }, bootProgressStallDelayMs)
  }
  const refreshActivity = inlineActivity ?? scheduleLocalStall
  const handleMark = () => {
    const status = globalThis.__swBootStatus
    if (status) renderBrowserBootStatus(status)
    refreshActivity()
  }
  window.addEventListener(startupMarkEvent, handleMark)
  handleMark()
  return () => {
    window.removeEventListener(startupMarkEvent, handleMark)
    if (!inlineActivity) clearLocalStall()
  }
}
