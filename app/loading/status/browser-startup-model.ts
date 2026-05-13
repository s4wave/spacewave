import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

export interface BrowserBootStatus {
  phase: string
  detail: string
  state: 'loading' | 'error'
  progress?: number
}

export interface BrowserStartupMark {
  name: string
  label: string
  sequence: number
  detail: Record<string, unknown>
}

export type BrowserStartupPhaseID =
  | 'prepare'
  | 'connect'
  | 'runtime'
  | 'frame'
  | 'done'

export interface BrowserStartupPhase {
  id: BrowserStartupPhaseID
  label: string
  detail: string
  progress: number
}

export interface BrowserStartupPhaseView extends BrowserStartupPhase {
  state: 'pending' | 'current' | 'complete' | 'error'
}

export interface BrowserStartupProjection {
  view: LoadingView
  phase: BrowserStartupPhase
  phases: BrowserStartupPhaseView[]
  evidence: {
    status: BrowserBootStatus
    marks: BrowserStartupMark[]
  }
}

export const browserStartupPhaseRail: readonly BrowserStartupPhase[] = [
  {
    id: 'prepare',
    label: 'Prepare',
    detail: 'Preparing browser files.',
    progress: 0.08,
  },
  {
    id: 'connect',
    label: 'Connect',
    detail: 'Connecting the app shell.',
    progress: 0.3,
  },
  {
    id: 'runtime',
    label: 'Runtime',
    detail: 'Starting the Spacewave runtime.',
    progress: 0.58,
  },
  {
    id: 'frame',
    label: 'Frame',
    detail: 'Opening the app frame.',
    progress: 0.84,
  },
  {
    id: 'done',
    label: 'Done',
    detail: 'Spacewave is ready.',
    progress: 1,
  },
]

const browserStartupPhaseIndex: Record<BrowserStartupPhaseID, number> = {
  prepare: 0,
  connect: 1,
  runtime: 2,
  frame: 3,
  done: 4,
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

const bootPhaseToStartupPhase: Record<string, BrowserStartupPhaseID> = {
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

const startupMarkToPhase: Record<string, BrowserStartupPhaseID> = {
  'boot-status.loading': 'prepare',
  'boot-status.manifest': 'prepare',
  'boot-status.manifest-ready': 'prepare',
  'boot-status.manifest-error': 'prepare',
  'boot-status.wasm': 'connect',
  'boot-status.entrypoint': 'connect',
  'boot-status.entrypoint-error': 'connect',
  'shell.entrypoint-loaded': 'connect',
  'shell.container-resolved': 'connect',
  'runtime.wait-start': 'runtime',
  'runtime.wait-ready': 'runtime',
  'shell.deferred-boot-ready': 'runtime',
  'boot-status.runtime': 'runtime',
  'boot-status.ready': 'runtime',
  'boot-status.runtime-error': 'runtime',
  'boot-status.app': 'frame',
  'shell.boot-requested': 'frame',
  'quickstart.static-handoff-requested': 'frame',
  'webview.registered': 'frame',
  'webview.stylesheet-ready': 'frame',
  'webview.component-ready': 'frame',
  'webview.revealed': 'done',
  'webview.loading-surface-mounted': 'frame',
  'webview.loading-surface-revealed': 'done',
}

function clampProgress(progress: number | undefined): number | undefined {
  if (progress === undefined || Number.isNaN(progress)) return undefined
  return Math.max(0, Math.min(1, progress))
}

export function browserBootPhaseProgress(phase: string): number | undefined {
  return phaseProgress[phase]
}

export function withBrowserBootProgress(
  status: BrowserBootStatus,
): BrowserBootStatus {
  const progress = clampProgress(
    status.progress ?? browserBootPhaseProgress(status.phase),
  )
  if (status.progress === progress) return status
  return {
    ...status,
    progress,
  }
}

export function projectBrowserStartup(
  status: BrowserBootStatus,
  marks: readonly BrowserStartupMark[] = [],
): BrowserStartupProjection {
  const boot = withBrowserBootProgress(status)
  const phase = selectStartupPhase(boot, marks)
  const view: LoadingView =
    boot.state === 'error' ?
      {
        state: 'error',
        title: 'Spacewave',
        detail: `${phase.label}: ${phase.detail}`,
        progress: phase.progress,
        error:
          'Startup did not finish. Check the browser console or startup marks for details.',
      }
    : {
        state: phase.id === 'done' ? 'synced' : 'loading',
        title: 'Spacewave',
        detail: `${phase.label}: ${phase.detail}`,
        progress: phase.progress,
      }

  return {
    view,
    phase,
    phases: projectPhaseViews(phase.id, boot.state),
    evidence: {
      status: boot,
      marks: [...marks],
    },
  }
}

export function projectBrowserStartupView(
  status: BrowserBootStatus,
  marks: readonly BrowserStartupMark[] = [],
): LoadingView {
  return projectBrowserStartup(status, marks).view
}

function selectStartupPhase(
  status: BrowserBootStatus,
  marks: readonly BrowserStartupMark[],
): BrowserStartupPhase {
  let selected = phaseIndex(bootPhaseToStartupPhase[status.phase] ?? 'prepare')
  if (status.state === 'error') {
    return browserStartupPhaseRail[selected]
  }
  for (const mark of marks) {
    if (!startupMarkCanAdvance(mark)) {
      continue
    }
    const phase = startupMarkToPhase[mark.label]
    if (phase) {
      selected = Math.max(selected, phaseIndex(phase))
    }
  }
  return browserStartupPhaseRail[selected]
}

function startupMarkCanAdvance(mark: BrowserStartupMark): boolean {
  if (!mark.label.startsWith('webview.')) {
    return true
  }
  if (typeof mark.detail.webViewId !== 'string') {
    return mark.detail.startupRelevant !== false
  }
  return mark.detail.startupRelevant === true
}

function projectPhaseViews(
  currentID: BrowserStartupPhaseID,
  bootState: BrowserBootStatus['state'],
): BrowserStartupPhaseView[] {
  const currentIdx = phaseIndex(currentID)
  return browserStartupPhaseRail.map((phase, idx) => {
    const state =
      idx < currentIdx ? 'complete'
      : idx === currentIdx && bootState === 'error' ? 'error'
      : idx === currentIdx ? 'current'
      : 'pending'
    return { ...phase, state }
  })
}

function phaseIndex(id: BrowserStartupPhaseID): number {
  return browserStartupPhaseIndex[id]
}
