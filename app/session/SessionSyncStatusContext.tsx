import { createContext, use, useMemo, type ReactNode } from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import {
  SyncActivityDirection,
  SyncP2PState,
  SyncStatusState,
  SyncTransportState,
  SyncAccountAccessState,
  type SyncTransferProgress,
  type SyncCloudProgress,
  type WatchSyncStatusResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { isStorageQuotaError } from '@s4wave/app/session/storage/storage-error.js'

export type SessionSyncVisualState = 'loading' | 'synced' | 'active' | 'error'

// SessionSyncSummary is the part of the sync status that changes with the
// sync state, not with each transfer rate sample.
export interface SessionSyncSummary {
  visualState: SessionSyncVisualState
  loading: boolean
  active: boolean
  error: boolean
  local: boolean
  summaryLabel: string
  detailLabel: string
  ariaLabel: string
  lastError: string
}

// SessionSyncStatusView is the complete sync status, including the transfer
// rates and counters that change on every sample.
export interface SessionSyncStatusView extends SessionSyncSummary {
  snapshot: WatchSyncStatusResponse | null
  transportLabel: string
  p2pLabel: string
  peerError: string
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
  localCopies: {
    id: string
    name: string
    status: string
    detail: string
    error: string
  }[]
  peerUploadLabel: string
  peerDownloadLabel: string
  peers: { id: string; name: string; state: string; traffic: string }[]
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
  peerError: '',
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
  localCopies: [],
  peerUploadLabel: '0 B',
  peerDownloadLabel: '0 B',
  peers: [],
}

const SessionSyncStatusContext =
  createContext<SessionSyncStatusView>(loadingView)

const SessionSyncSummaryContext = createContext<SessionSyncSummary>(loadingView)

// SessionSyncStatusProvider owns the session sync-status watch for a session
// UI tree. It provides the complete status, and a summary whose value changes
// only when a summary field does, so summary consumers skip the rate samples.
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
  const {
    visualState,
    loading,
    active,
    error,
    local,
    summaryLabel,
    detailLabel,
    ariaLabel,
    lastError,
  } = value
  const summary = useMemo(
    (): SessionSyncSummary => ({
      visualState,
      loading,
      active,
      error,
      local,
      summaryLabel,
      detailLabel,
      ariaLabel,
      lastError,
    }),
    [
      visualState,
      loading,
      active,
      error,
      local,
      summaryLabel,
      detailLabel,
      ariaLabel,
      lastError,
    ],
  )

  return (
    <SessionSyncSummaryContext.Provider value={summary}>
      <SessionSyncStatusContext.Provider value={value}>
        {children}
      </SessionSyncStatusContext.Provider>
    </SessionSyncSummaryContext.Provider>
  )
}

// useSessionSyncStatus returns the complete session sync status, which changes
// on every transfer rate sample. Prefer useSessionSyncSummary where the
// summary fields suffice.
export function useSessionSyncStatus(): SessionSyncStatusView {
  return use(SessionSyncStatusContext)
}

// useSessionSyncSummary returns the session sync summary, which changes only
// when the sync state or its labels change.
export function useSessionSyncSummary(): SessionSyncSummary {
  return use(SessionSyncSummaryContext)
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

  const cloud = snapshot?.cloud
  const peer = snapshot?.peer
  const local =
    snapshot?.access === SyncAccountAccessState.SyncAccountAccessState_LOCAL
  const transfer = local ? peer?.transfer : cloud?.transfer
  const state = transfer?.state ?? SyncStatusState.SyncStatusState_UNKNOWN
  const direction =
    transfer?.direction ?? SyncActivityDirection.SyncActivityDirection_NONE
  const p2p = peer?.state ?? SyncP2PState.SyncP2PState_UNKNOWN
  const peerError = peer?.lastError ?? ''
  const lastError = local ? peerError : (transfer?.lastError ?? '')
  const copies = peer?.localCopies ?? []
  const incomplete = copies.filter((copy) => !copy.complete).length
  const storageFull = copies.filter(
    (copy) => !copy.complete && isStorageQuotaError(copy.error ?? ''),
  ).length
  const paused = incomplete > 0 && !peer?.activePeerCount
  const visualState =
    storageFull > 0
      ? 'error'
      : syncVisualState(transfer, state, direction, lastError)
  let summaryLabel = syncSummaryLabel(visualState, direction, local)
  let detailLabel = syncDetailLabel(visualState, direction, local, lastError)
  if (storageFull > 0) {
    summaryLabel = 'Storage full'
    detailLabel = `This browser is out of storage, so ${storageFull} ${storageFull === 1 ? 'Space' : 'Spaces'} cannot be stored on this device. Free up space to continue.`
  } else if (local && copies.length && !lastError) {
    summaryLabel = incomplete
      ? paused
        ? 'Copy paused'
        : 'Copying Spaces'
      : 'Available offline'
    detailLabel = incomplete
      ? `${copies.length - incomplete} of ${copies.length} Spaces available offline. ${paused ? 'Waiting for a linked Session.' : 'You can use your account while copying continues.'}`
      : 'The latest received Spaces and their contents are stored on this device.'
  }
  const uploadRateLabel = formatRate(transfer?.uploadBytesPerSecond)
  const downloadRateLabel = formatRate(transfer?.downloadBytesPerSecond)

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
    transportLabel: local
      ? peer?.networkState === SyncTransportState.SyncTransportState_ONLINE
        ? 'Peer network ready'
        : 'Peer network offline'
      : syncAccessLabel(snapshot?.access),
    p2pLabel:
      local && !peerError
        ? peer?.activePeerCount
          ? `${peer.activePeerCount} linked Session${peer.activePeerCount === 1 ? '' : 's'} online`
          : 'No linked Sessions online'
        : syncP2PLabel(p2p),
    peerError,
    uploadRateLabel,
    downloadRateLabel,
    pendingUploadLabel: formatBytes(transfer?.pendingUploadBytes),
    activeUploadLabel: formatActiveUpload(transfer),
    pendingDownloadLabel: formatBytes(transfer?.pendingDownloadBytes),
    packRangeLabel: formatCountBytes(
      cloud?.packRangeRequestCount,
      cloud?.packRangeResponseBytes,
    ),
    packIndexTailLabel: formatCountBytes(
      cloud?.packIndexTailFetchCount,
      cloud?.packIndexTailResponseBytes,
    ),
    packLookupLabel: formatPackLookup(cloud),
    packIndexCacheLabel: formatPackIndexCache(cloud),
    lastActivityLabel: formatLastActivity(transfer?.lastActivityAt),
    lastError,
    localCopies: copies.map((copy) => ({
      id: copy.sharedObjectId ?? '',
      name: copy.displayName || 'Space',
      status: copy.complete
        ? 'Available offline'
        : isStorageQuotaError(copy.error ?? '')
          ? 'Storage full'
          : copy.error || paused
            ? 'Waiting to resume'
            : 'Copying',
      detail: `${formatBytes(copy.bytes)} stored · ${formatCount(copy.blocks)} blocks`,
      error: copy.error ?? '',
    })),
    peerUploadLabel: formatBytes(peer?.peerUploadBytes),
    peerDownloadLabel: formatBytes(peer?.peerDownloadBytes),
    peers: (peer?.peers ?? []).map((peer) => ({
      id: peer.peerId ?? '',
      name: `Session …${(peer.peerId ?? '').slice(-8)}`,
      state: peer.connected ? 'Connected' : 'Offline',
      traffic: `↑ ${formatBytes(peer.uploadedBytes)} · ↓ ${formatBytes(peer.downloadedBytes)}`,
    })),
  }
}

function syncVisualState(
  transfer: SyncTransferProgress | null | undefined,
  state: SyncStatusState,
  direction: SyncActivityDirection,
  lastError: string,
): SessionSyncVisualState {
  if (lastError || state === SyncStatusState.SyncStatusState_ERROR) {
    return 'error'
  }
  if (
    state === SyncStatusState.SyncStatusState_ACTIVE &&
    hasVisibleSyncWork(transfer, direction)
  ) {
    return 'active'
  }
  return 'synced'
}

function hasVisibleSyncWork(
  transfer: SyncTransferProgress | null | undefined,
  direction: SyncActivityDirection,
): boolean {
  return (
    direction !== SyncActivityDirection.SyncActivityDirection_NONE ||
    hasBytes(transfer?.pendingUploadBytes) ||
    hasBytes(transfer?.pendingDownloadBytes) ||
    hasCount(transfer?.pendingUploadCount) ||
    hasCount(transfer?.pendingDownloadCount)
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
    return activeDirectionDetail(direction, local)
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

// activeDirectionDetail describes where the transfer goes: a local session has
// no cloud, so it moves blocks only with linked peers.
function activeDirectionDetail(
  direction: SyncActivityDirection,
  local: boolean,
): string {
  const remote = local ? 'peers' : 'cloud'
  if (direction === SyncActivityDirection.SyncActivityDirection_UPLOAD) {
    return `Uploading to ${remote}.`
  }
  if (direction === SyncActivityDirection.SyncActivityDirection_DOWNLOAD) {
    return `Downloading from ${remote}.`
  }
  if (
    direction === SyncActivityDirection.SyncActivityDirection_UPLOAD_DOWNLOAD
  ) {
    return 'Uploading and downloading.'
  }
  return 'Sync active.'
}

// syncAccessLabel describes account access without claiming the cloud socket is healthy.
function syncAccessLabel(state?: SyncAccountAccessState): string {
  switch (state) {
    case SyncAccountAccessState.SyncAccountAccessState_READY:
    case SyncAccountAccessState.SyncAccountAccessState_DORMANT:
      return 'Account ready'
    case SyncAccountAccessState.SyncAccountAccessState_PENDING:
      return 'Account opening'
    case SyncAccountAccessState.SyncAccountAccessState_FAILED:
      return 'Account unavailable'
    case SyncAccountAccessState.SyncAccountAccessState_DELETED:
      return 'Account deleted'
    case SyncAccountAccessState.SyncAccountAccessState_UNAUTHENTICATED:
      return 'Sign in required'
    case SyncAccountAccessState.SyncAccountAccessState_LOCAL:
      return 'Local account'
    default:
      return 'Account access unknown'
  }
}

function syncP2PLabel(state: SyncP2PState): string {
  switch (state) {
    case SyncP2PState.SyncP2PState_NO_PEERS:
      return 'No P2P peers'
    case SyncP2PState.SyncP2PState_IDLE:
      return 'P2P idle'
    case SyncP2PState.SyncP2PState_ACTIVE:
      return 'P2P active'
    case SyncP2PState.SyncP2PState_ERROR:
      return 'P2P error'
    case SyncP2PState.SyncP2PState_DISABLED:
      return 'P2P disabled'
    case SyncP2PState.SyncP2PState_STARTING:
      return 'P2P starting'
    case SyncP2PState.SyncP2PState_FALLBACK_NO_PEER:
      return 'P2P waiting for a peer'
    default:
      return 'P2P unknown'
  }
}

function formatActiveUpload(
  snapshot: SyncTransferProgress | null | undefined,
): string {
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

function formatPackLookup(
  snapshot: SyncCloudProgress | null | undefined,
): string {
  const opened = snapshot?.packLastOpenedPacks ?? 0
  const candidates = snapshot?.packLastCandidatePacks ?? 0
  return `${opened} opened / ${candidates} candidates`
}

function formatPackIndexCache(
  snapshot: SyncCloudProgress | null | undefined,
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
