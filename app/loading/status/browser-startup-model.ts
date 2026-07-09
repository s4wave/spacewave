import { projectBootProgress } from '@aptre/bldr'
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
  { id: 'prepare', label: 'Prepare' },
  { id: 'connect', label: 'Connect' },
  { id: 'runtime', label: 'Runtime' },
  { id: 'frame', label: 'App' },
  { id: 'done', label: 'Done' },
]

const browserStartupPhaseIndex: Record<BrowserStartupPhaseID, number> = {
  prepare: 0,
  connect: 1,
  runtime: 2,
  frame: 3,
  done: 4,
}

function clampProgress(progress: number | undefined): number | undefined {
  if (progress === undefined || !Number.isFinite(progress)) return undefined
  return Math.max(0, Math.min(1, progress))
}

// withBrowserBootProgress normalizes the raw status: progress carries a 0..1
// within-step download fraction (never a bar position); invalid values drop.
export function withBrowserBootProgress(
  status: BrowserBootStatus,
): BrowserBootStatus {
  const progress = clampProgress(status.progress)
  if (progress === undefined) {
    return {
      phase: status.phase,
      detail: status.detail,
      state: status.state,
    }
  }
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
  const step = projectBootProgress(boot, marks)
  const progress = phase.id === 'done' ? 1 : step.progress
  const baseView = {
    title: 'Spacewave',
    detail: `${phase.label}: ${step.label}`,
    progress,
  }
  const view: LoadingView = failed
    ? {
        state: 'error',
        ...baseView,
        error:
          'Startup did not finish. Check the browser console or startup marks for details.',
      }
    : {
        state: phase.id === 'done' ? 'synced' : 'loading',
        ...baseView,
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
