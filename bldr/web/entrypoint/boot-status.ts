import { markStartupBoundary } from '../bldr/startup-marks.js'

export interface BrowserBootStatus {
  phase: string
  detail: string
  state: 'loading' | 'error'
  progress?: number
}

export const bootStatusEvent = 'spacewave:boot-status'

declare global {
  var __swBootStatus: BrowserBootStatus | undefined
}

const phaseProgress: Record<string, number> = {
  loading: 0.04,
  manifest: 0.12,
  'manifest-ready': 0.22,
  wasm: 0.38,
  entrypoint: 0.54,
  runtime: 0.76,
  ready: 0.9,
  app: 0.96,
}

const startupPhaseInfo = {
  prepare: {
    label: 'Prepare',
    detail: 'Preparing browser files.',
    progress: 0.08,
  },
  connect: {
    label: 'Connect',
    detail: 'Connecting the app shell.',
    progress: 0.3,
  },
  runtime: {
    label: 'Runtime',
    detail: 'Starting the Spacewave runtime.',
    progress: 0.58,
  },
  frame: {
    label: 'App',
    detail: 'Downloading the app bundle. This can take a while the first time.',
    progress: 0.84,
  },
  done: {
    label: 'Done',
    detail: 'Spacewave is ready.',
    progress: 1,
  },
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

function canMutateBootStatusTarget(
  target: Element | null,
): target is Element {
  if (!target) return false
  const root = target.closest('#bldr-root[data-prerendered]')
  if (!root) return true
  return !!target.closest('#sw-loading')
}

function startupDisplayForBootPhase(phase: string, state: string) {
  const id = bootPhaseStartupPhase[phase] ?? 'prepare'
  const info = startupPhaseInfo[id]
  return {
    id,
    detail: `${info.label}: ${info.detail}`,
    progress: info.progress,
    indeterminate: id === 'frame',
    error: state === 'error',
  }
}

function updateProgressTarget(
  target: Element | null,
  progress: number | undefined,
  indeterminate?: boolean,
) {
  if (!canMutateBootStatusTarget(target)) return
  if (target instanceof HTMLElement) {
    target.style.width =
      indeterminate || progress === undefined
        ? '33%'
        : `${Math.round(progress * 100)}%`
    target.style.transition = indeterminate ? 'none' : 'width 200ms'
    target.classList.toggle('animate-progress-indeterminate', !!indeterminate)
  }
  if (indeterminate) {
    target.removeAttribute('aria-valuenow')
    target.setAttribute('aria-valuetext', 'Loading')
    return
  }
  target.removeAttribute('aria-valuetext')
  target.setAttribute(
    'aria-valuenow',
    String(Math.round((progress ?? 0) * 100)),
  )
}

function updateProgressLabel(
  target: Element | null,
  progress: number | undefined,
  indeterminate?: boolean,
) {
  if (!canMutateBootStatusTarget(target)) return
  target.replaceChildren(
    indeterminate ? '' : `${Math.round((progress ?? 0) * 100)}%`,
  )
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
              ? 'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)'
              : 'color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)'
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

export function writeBrowserBootStatus(status: BrowserBootStatus): void {
  const progress = status.progress ?? phaseProgress[status.phase]
  const next = progress === undefined ? status : { ...status, progress }
  const display = startupDisplayForBootPhase(next.phase, next.state)
  globalThis.__swBootStatus = next

  const detailTarget = document.querySelector('[data-sw-boot-status]')
  if (canMutateBootStatusTarget(detailTarget)) {
    detailTarget.replaceChildren(display.detail)
  }

  const stateTarget = document.querySelector('[data-sw-boot-state]')
  if (canMutateBootStatusTarget(stateTarget)) {
    stateTarget.setAttribute('data-sw-boot-state', next.state)
  }

  updateProgressTarget(
    document.querySelector('[data-sw-boot-progress]'),
    display.progress,
    display.indeterminate,
  )
  updateProgressLabel(
    document.querySelector('[data-sw-boot-progress-label]'),
    display.progress,
    display.indeterminate,
  )
  updateStaticPhaseRail(display.id, next.state)
  updateStaticErrorState(next)

  markStartupBoundary(`boot-status.${next.phase}`, {
    source: 'browser',
    phase: next.phase,
    state: next.state,
    progress: next.progress,
  })
  window.dispatchEvent(new CustomEvent(bootStatusEvent, { detail: next }))
}
