import { createContext, useContext, useMemo, type ReactNode } from 'react'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import type { SharedObjectSelfEnrollment } from '@s4wave/sdk/session/shared-object-self-enrollment.js'
import type {
  SharedObjectSelfEnrollmentFailure,
  WatchSharedObjectSelfEnrollmentStateResponse,
} from '@s4wave/sdk/session/shared-object-self-enrollment.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

export type SessionSelfEnrollmentVisualState =
  | 'loading'
  | 'ready'
  | 'pending'
  | 'running'
  | 'waiting-for-step-up'
  | 'skipped'
  | 'failed'

export interface SessionSelfEnrollmentStatusView {
  resource: SharedObjectSelfEnrollment | null
  snapshot: WatchSharedObjectSelfEnrollmentStateResponse | null
  visualState: SessionSelfEnrollmentVisualState
  loading: boolean
  pending: boolean
  running: boolean
  credentialRequired: boolean
  skipped: boolean
  failed: boolean
  count: number
  completedCount: number
  totalCount: number
  progress: number
  generationKey: string
  failures: SharedObjectSelfEnrollmentFailure[]
  summaryLabel: string
  detailLabel: string
}

const loadingView: SessionSelfEnrollmentStatusView = {
  resource: null,
  snapshot: null,
  visualState: 'loading',
  loading: true,
  pending: false,
  running: false,
  credentialRequired: false,
  skipped: false,
  failed: false,
  count: 0,
  completedCount: 0,
  totalCount: 0,
  progress: 0,
  generationKey: '',
  failures: [],
  summaryLabel: 'Checking connected spaces',
  detailLabel: 'Waiting for connected-space status.',
}

const SessionSelfEnrollmentStatusContext =
  createContext<SessionSelfEnrollmentStatusView>(loadingView)

// SessionSelfEnrollmentStatusProvider owns the session self-enrollment watch.
export function SessionSelfEnrollmentStatusProvider({
  children,
}: {
  children: ReactNode
}) {
  const sessionResource = SessionContext.useContext()
  const enrollmentResource = useResource(
    sessionResource,
    async (session, signal, cleanup) =>
      cleanup(await session.spacewave.mountSharedObjectSelfEnrollment(signal)),
    [],
  )
  const enrollment = useResourceValue(enrollmentResource) ?? null
  const state = useStreamingResource(
    enrollmentResource,
    (value, signal) => value.watchState(signal),
    [],
  )
  const value = useMemo(
    () =>
      buildSessionSelfEnrollmentStatusView(
        enrollment,
        state?.value ?? null,
        state?.loading ?? true,
        state?.error ?? null,
      ),
    [enrollment, state?.error, state?.loading, state?.value],
  )

  return (
    <SessionSelfEnrollmentStatusContext.Provider value={value}>
      {children}
    </SessionSelfEnrollmentStatusContext.Provider>
  )
}

// useSessionSelfEnrollmentStatus returns the session self-enrollment status.
export function useSessionSelfEnrollmentStatus(): SessionSelfEnrollmentStatusView {
  return useContext(SessionSelfEnrollmentStatusContext)
}

export function buildSessionSelfEnrollmentStatusView(
  resource: SharedObjectSelfEnrollment | null,
  snapshot: WatchSharedObjectSelfEnrollmentStateResponse | null,
  loading: boolean,
  error: Error | null,
): SessionSelfEnrollmentStatusView {
  if (error) {
    return {
      ...loadingView,
      resource,
      snapshot,
      visualState: 'failed',
      loading: false,
      failed: true,
      summaryLabel: 'Connection status unavailable',
      detailLabel: error.message,
    }
  }
  if (loading && !snapshot) {
    return { ...loadingView, resource }
  }

  const failures = snapshot?.failures ?? []
  const count = snapshot?.count ?? 0
  const completedCount = snapshot?.completedSharedObjectIds?.length ?? 0
  const totalCount = Math.max(
    count,
    completedCount,
    snapshot?.sharedObjectIds?.length ?? 0,
  )
  const running = !!snapshot?.running
  const credentialRequired = !!snapshot?.credentialRequired
  const skipped = !!snapshot?.skipped
  const failed = failures.length > 0
  const pending = count > 0
  const visualState = selfEnrollmentVisualState(
    pending,
    running,
    credentialRequired,
    skipped,
    failed,
  )

  return {
    resource,
    snapshot,
    visualState,
    loading: false,
    pending,
    running,
    credentialRequired,
    skipped,
    failed,
    count,
    completedCount,
    totalCount,
    progress:
      totalCount > 0 ? Math.min(100, (completedCount / totalCount) * 100) : 0,
    generationKey: snapshot?.generationKey ?? '',
    failures,
    summaryLabel: selfEnrollmentSummaryLabel(visualState, count),
    detailLabel: selfEnrollmentDetailLabel(visualState, count, failures.length),
  }
}

function selfEnrollmentVisualState(
  pending: boolean,
  running: boolean,
  credentialRequired: boolean,
  skipped: boolean,
  failed: boolean,
): SessionSelfEnrollmentVisualState {
  if (failed) return 'failed'
  if (running) return 'running'
  if (skipped) return 'skipped'
  if (pending && credentialRequired) return 'waiting-for-step-up'
  if (pending) return 'pending'
  return 'ready'
}

function selfEnrollmentSummaryLabel(
  state: SessionSelfEnrollmentVisualState,
  count: number,
): string {
  if (state === 'running') return 'Connecting spaces'
  if (state === 'waiting-for-step-up') return 'Spaces need this session key'
  if (state === 'pending') return 'Spaces need this session'
  if (state === 'skipped') return 'Space connection skipped'
  if (state === 'failed') return 'Some spaces need attention'
  if (state === 'loading') return 'Checking connected spaces'
  if (count > 0) return 'Spaces need this session'
  return ''
}

function selfEnrollmentDetailLabel(
  state: SessionSelfEnrollmentVisualState,
  count: number,
  failureCount: number,
): string {
  if (state === 'running') return `Connecting ${formatSpaceCount(count)}.`
  if (state === 'waiting-for-step-up') {
    return `${formatSpaceCount(count)} need an account unlock before this session can connect.`
  }
  if (state === 'pending') {
    return `${formatSpaceCount(count)} can connect in the background.`
  }
  if (state === 'skipped') return 'This generation will stay skipped for now.'
  if (state === 'failed')
    return `${formatSpaceCount(failureCount)} failed to connect.`
  if (state === 'loading') return 'Waiting for connected-space status.'
  return ''
}

function formatSpaceCount(count: number): string {
  if (count === 1) return '1 space'
  return `${count} spaces`
}
