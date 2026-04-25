import { createContext, useContext, useMemo, type ReactNode } from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import {
  SyncActivityDirection,
  SyncP2PState,
  SyncStatusState,
  SyncTransportState,
  type WatchSyncStatusResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

export type SessionSyncVisualState = 'loading' | 'synced' | 'active' | 'error'

export interface SessionSyncStatusView {
  snapshot: WatchSyncStatusResponse | null
  visualState: SessionSyncVisualState
  loading: boolean
  active: boolean
  error: boolean
  local: boolean
  summaryLabel: string
  detailLabel: string
  ariaLabel: string
  transportLabel: string
  p2pLabel: string
  uploadRateLabel: string
  downloadRateLabel: string
  pendingUploadLabel: string
  activeUploadLabel: string
  pendingDownloadLabel: string
  packRangeLabel: string
  packIndexTailLabel: string
  packLookupLabel: string
  packIndexCacheLabel: string
  lastActivityLabel: string
  lastError: string
}

const loadingView: SessionSyncStatusView = {
  snapshot: null,
  visualState: 'loading',
  loading: true,
  active: false,
  error: false,
  local: false,
  summaryLabel: 'Checking sync status',
  detailLabel: 'Waiting for the session sync watcher.',
  ariaLabel: 'Session sync status: checking',
  transportLabel: 'Transport unknown',
  p2pLabel: 'P2P unknown',
  uploadRateLabel: '0 B/s',
  downloadRateLabel: '0 B/s',
  pendingUploadLabel: '0 B',
  activeUploadLabel: '0 B',
  pendingDownloadLabel: '0 B',
  packRangeLabel: '0 / 0 B',
  packIndexTailLabel: '0 / 0 B',
  packLookupLabel: '0 opened / 0 candidates',
  packIndexCacheLabel: '0 hits / 0 misses',
  lastActivityLabel: 'No recent activity',
  lastError: '',
}

const SessionSyncStatusContext =
  createContext<SessionSyncStatusView>(loadingView)

// SessionSyncStatusProvider owns the session sync-status watch for a session UI tree.
export function SessionSyncStatusProvider({
  children,
}: {
  children: ReactNode
}) {
  const sessionResource = SessionContext.useContext()
  const resource = useStreamingResource(
    sessionResource,
    (session, signal) => session.watchSyncStatus({}, signal),
    [],
  )
  const value = useMemo(
    () =>
      buildSessionSyncStatusView(
        resource?.value ?? null,
        resource?.loading ?? true,
        resource?.error ?? null,
      ),
    [resource?.error, resource?.loading, resource?.value],
  )

  return (
    <SessionSyncStatusContext.Provider value={value}>
      {children}
    </SessionSyncStatusContext.Provider>
  )
}

// useSessionSyncStatus returns the current provider-owned session sync status view.
export function useSessionSyncStatus(): SessionSyncStatusView {
  return useContext(SessionSyncStatusContext)
}

export function buildSessionSyncStatusView(
  snapshot: WatchSyncStatusResponse | null,
  loading: boolean,
  watchError: Error | null,
): SessionSyncStatusView {
  if (watchError) {
    return {
      ...loadingView,
      snapshot,
      visualState: 'error',
      loading: false,
      error: true,
      summaryLabel: 'Sync watcher error',
      detailLabel: watchError.message,
      ariaLabel: `Session sync status: error, ${watchError.message}`,
      lastError: watchError.message,
    }
  }
  if (loading && !snapshot) {
    return loadingView
  }

  const state = snapshot?.state ?? SyncStatusState.SyncStatusState_SYNCED
  const direction =
    snapshot?.direction ?? SyncActivityDirection.SyncActivityDirection_NONE
  const transport =
    snapshot?.transportState ?? SyncTransportState.SyncTransportState_UNKNOWN
  const p2p = snapshot?.p2pState ?? SyncP2PState.SyncP2PState_UNKNOWN
  const lastError = snapshot?.lastError ?? ''
  const visualState = syncVisualState(snapshot, state, direction, lastError)
  const local = transport === SyncTransportState.SyncTransportState_UNAVAILABLE
  const summaryLabel = syncSummaryLabel(visualState, direction, local)
  const detailLabel = syncDetailLabel(visualState, direction, local, lastError)
  const uploadRateLabel = formatRate(snapshot?.uploadBytesPerSecond)
  const downloadRateLabel = formatRate(snapshot?.downloadBytesPerSecond)

  return {
    snapshot,
    visualState,
    loading: false,
    active: visualState === 'active',
    error: visualState === 'error',
    local,
    summaryLabel,
    detailLabel,
    ariaLabel: `Session sync status: ${summaryLabel}`,
    transportLabel: syncTransportLabel(transport),
    p2pLabel: syncP2PLabel(p2p),
    uploadRateLabel,
    downloadRateLabel,
    pendingUploadLabel: formatBytes(snapshot?.pendingUploadBytes),
    activeUploadLabel: formatActiveUpload(snapshot),
    pendingDownloadLabel: formatBytes(snapshot?.pendingDownloadBytes),
    packRangeLabel: formatCountBytes(
      snapshot?.packRangeRequestCount,
      snapshot?.packRangeResponseBytes,
    ),
    packIndexTailLabel: formatCountBytes(
      snapshot?.packIndexTailFetchCount,
      snapshot?.packIndexTailResponseBytes,
    ),
    packLookupLabel: formatPackLookup(snapshot),
    packIndexCacheLabel: formatPackIndexCache(snapshot),
    lastActivityLabel: formatLastActivity(snapshot?.lastActivityAt),
    lastError,
  }
}

function syncVisualState(
  snapshot: WatchSyncStatusResponse | null,
  state: SyncStatusState,
  direction: SyncActivityDirection,
  lastError: string,
): SessionSyncVisualState {
  if (lastError || state === SyncStatusState.SyncStatusState_ERROR) {
    return 'error'
  }
  if (
    state === SyncStatusState.SyncStatusState_ACTIVE &&
    hasVisibleSyncWork(snapshot, direction)
  ) {
    return 'active'
  }
  return 'synced'
}

function hasVisibleSyncWork(
  snapshot: WatchSyncStatusResponse | null,
  direction: SyncActivityDirection,
): boolean {
  return (
    direction !== SyncActivityDirection.SyncActivityDirection_NONE ||
    hasBytes(snapshot?.pendingUploadBytes) ||
    hasBytes(snapshot?.pendingDownloadBytes) ||
    hasCount(snapshot?.pendingUploadCount) ||
    hasCount(snapshot?.pendingDownloadCount)
  )
}

function hasBytes(value?: bigint): boolean {
  return (value ?? 0n) > 0n
}

function hasCount(value?: number): boolean {
  return (value ?? 0) > 0
}

function syncSummaryLabel(
  state: SessionSyncVisualState,
  direction: SyncActivityDirection,
  local: boolean,
): string {
  if (state === 'loading') {
    return 'Checking sync status'
  }
  if (state === 'error') {
    return 'Sync needs attention'
  }
  if (state === 'active') {
    return activeDirectionLabel(direction)
  }
  if (local) {
    return 'Local ready'
  }
  return 'Synced'
}

function syncDetailLabel(
  state: SessionSyncVisualState,
  direction: SyncActivityDirection,
  local: boolean,
  lastError: string,
): string {
  if (state === 'error') {
    return lastError || 'The sync watcher reported an error.'
  }
  if (state === 'active') {
    return activeDirectionDetail(direction)
  }
  if (local) {
    return 'Local session ready.'
  }
  return 'All sync work complete.'
}

function activeDirectionLabel(direction: SyncActivityDirection): string {
  if (direction === SyncActivityDirection.SyncActivityDirection_UPLOAD) {
    return 'Uploading changes'
  }
  if (direction === SyncActivityDirection.SyncActivityDirection_DOWNLOAD) {
    return 'Downloading updates'
  }
  if (
    direction === SyncActivityDirection.SyncActivityDirection_UPLOAD_DOWNLOAD
  ) {
    return 'Syncing changes'
  }
  return 'Sync active'
}

function activeDirectionDetail(direction: SyncActivityDirection): string {
  if (direction === SyncActivityDirection.SyncActivityDirection_UPLOAD) {
    return 'Uploading to cloud.'
  }
  if (direction === SyncActivityDirection.SyncActivityDirection_DOWNLOAD) {
    return 'Downloading from cloud.'
  }
  if (
    direction === SyncActivityDirection.SyncActivityDirection_UPLOAD_DOWNLOAD
  ) {
    return 'Uploading and downloading.'
  }
  return 'Sync active.'
}

function syncTransportLabel(state: SyncTransportState): string {
  if (state === SyncTransportState.SyncTransportState_ONLINE) {
    return 'Cloud online'
  }
  if (state === SyncTransportState.SyncTransportState_CONNECTING) {
    return 'Cloud connecting'
  }
  if (state === SyncTransportState.SyncTransportState_UNAVAILABLE) {
    return 'Cloud unavailable'
  }
  if (state === SyncTransportState.SyncTransportState_ERROR) {
    return 'Cloud transport error'
  }
  return 'Cloud transport unknown'
}

function syncP2PLabel(state: SyncP2PState): string {
  if (state === SyncP2PState.SyncP2PState_NO_PEERS) {
    return 'No P2P peers'
  }
  if (state === SyncP2PState.SyncP2PState_IDLE) {
    return 'P2P idle'
  }
  if (state === SyncP2PState.SyncP2PState_ACTIVE) {
    return 'P2P active'
  }
  if (state === SyncP2PState.SyncP2PState_ERROR) {
    return 'P2P error'
  }
  return 'P2P unknown'
}

function formatActiveUpload(snapshot: WatchSyncStatusResponse | null): string {
  const active = snapshot?.activeUploadBytes ?? 0n
  const sent = snapshot?.activeUploadTransferredBytes ?? 0n
  const count = snapshot?.inFlightUploadCount ?? 0
  if (active === 0n && count === 0) {
    return '0 B'
  }
  return `${formatBytes(sent)} / ${formatBytes(active)}`
}

function formatRate(bytes?: bigint): string {
  return `${formatBytes(bytes)}/s`
}

function formatCountBytes(count?: bigint, bytes?: bigint): string {
  return `${formatCount(count)} / ${formatBytes(bytes)}`
}

function formatCount(count?: bigint): string {
  return Number(count ?? 0n).toLocaleString()
}

function formatPackLookup(snapshot: WatchSyncStatusResponse | null): string {
  const opened = snapshot?.packLastOpenedPacks ?? 0
  const candidates = snapshot?.packLastCandidatePacks ?? 0
  return `${opened} opened / ${candidates} candidates`
}

function formatPackIndexCache(
  snapshot: WatchSyncStatusResponse | null,
): string {
  const hits = formatCount(snapshot?.packIndexCacheHits)
  const misses = formatCount(snapshot?.packIndexCacheMisses)
  const errors =
    (snapshot?.packIndexCacheReadErrors ?? 0n) +
    (snapshot?.packIndexCacheWriteErrors ?? 0n)
  if (errors > 0n) {
    return `${hits} hits / ${misses} misses / ${formatCount(errors)} errors`
  }
  return `${hits} hits / ${misses} misses`
}

function formatBytes(bytes?: bigint): string {
  const value = Number(bytes ?? 0n)
  if (value < 1024) {
    return `${value} B`
  }
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  const exp = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length,
  )
  const amount = value / 1024 ** exp
  return `${formatAmount(amount)} ${units[exp - 1]}`
}

function formatAmount(value: number): string {
  if (value >= 10) {
    return value.toFixed(0)
  }
  return value.toFixed(1)
}

function formatLastActivity(date?: Date): string {
  if (!date) {
    return 'No recent activity'
  }
  return `Last activity ${date.toLocaleTimeString([], {
    hour: 'numeric',
    minute: '2-digit',
  })}`
}
