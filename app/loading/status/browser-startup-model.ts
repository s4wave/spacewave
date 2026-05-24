import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import {
  buildBrowserRuntimeState,
  projectBrowserRuntimeStartupPhase,
  type BrowserRuntimeState,
} from './browser-runtime-state.js'

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
    runtime: BrowserRuntimeState
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
    label: 'App',
    detail: 'Downloading the app bundle. This can take a while the first time.',
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
  const runtime = buildBrowserRuntimeState(boot, marks)
  const phase =
    browserStartupPhaseRail[
      phaseIndex(projectBrowserRuntimeStartupPhase(runtime))
    ]
  const failed = boot.state === 'error' || !!runtime.terminalFailure
  const view: LoadingView = failed
    ? {
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
        ...(phase.id === 'frame' ? { progressIndeterminate: true } : {}),
      }

  return {
    view,
    phase,
    phases: projectPhaseViews(phase.id, failed),
    evidence: {
      status: boot,
      marks: [...marks],
      runtime,
    },
  }
}

export function projectBrowserStartupView(
  status: BrowserBootStatus,
  marks: readonly BrowserStartupMark[] = [],
): LoadingView {
  return projectBrowserStartup(status, marks).view
}

function projectPhaseViews(
  currentID: BrowserStartupPhaseID,
  failed: boolean,
): BrowserStartupPhaseView[] {
  const currentIdx = phaseIndex(currentID)
  return browserStartupPhaseRail.map((phase, idx) => {
    const state =
      idx < currentIdx
        ? 'complete'
        : idx === currentIdx && failed
          ? 'error'
          : idx === currentIdx
            ? 'current'
            : 'pending'
    return { ...phase, state }
  })
}

function phaseIndex(id: BrowserStartupPhaseID): number {
  return browserStartupPhaseIndex[id]
}
