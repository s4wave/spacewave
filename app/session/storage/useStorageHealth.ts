import { useCallback, useMemo } from 'react'

import { useSessionStorageStats } from '@s4wave/app/session/SessionStorageStatsContext.js'
import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from '@s4wave/app/session/SessionSyncStatusContext.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'

export type StorageProtectionState =
  | 'checking'
  | 'protected'
  | 'not-protected'
  | 'unavailable'

export interface BrowserStorageManager {
  estimate?: () => Promise<{ usage?: number; quota?: number }>
  persisted?: () => Promise<boolean>
}

export interface BrowserStorageSnapshot {
  supported: boolean
  usageBytes: number | null
  quotaBytes: number | null
  persisted: boolean | null
}

export interface StorageHealthView {
  providerLoading: boolean
  providerSupported: boolean
  providerBytes: bigint
  blockCount: bigint
  browserReadFailed: boolean
  originUsageBytes: number | null
  originQuotaBytes: number | null
  protectionState: StorageProtectionState
  sync: SessionSyncStatusView
  replicaLabel: 'Not yet verified'
  safariCleanupRisk: boolean
}

// useStorageHealth composes provider, browser, and sync facts without inferring
// replica safety from any of them.
export function useStorageHealth(): StorageHealthView {
  const provider = useSessionStorageStats()
  const sync = useSessionSyncStatus()
  const browser = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        readBrowserStorageHealth(
          typeof navigator === 'undefined' ? null : navigator.storage,
          signal,
        ),
      [],
    ),
  )

  return useMemo(
    () =>
      buildStorageHealthView(
        provider,
        sync,
        browser.data,
        browser.loading,
        browser.error,
        typeof navigator === 'undefined' ? '' : navigator.userAgent,
      ),
    [browser.data, browser.error, browser.loading, provider, sync],
  )
}

// readBrowserStorageHealth reads both StorageManager facts independently so one
// unavailable browser API does not hide the other.
export async function readBrowserStorageHealth(
  storage: BrowserStorageManager | null,
  signal: AbortSignal,
): Promise<BrowserStorageSnapshot> {
  if (!storage) {
    return {
      supported: false,
      usageBytes: null,
      quotaBytes: null,
      persisted: null,
    }
  }

  const estimate = storage.estimate
    ? storage.estimate()
    : Promise.reject(new Error('Storage estimate is unavailable'))
  const persisted = storage.persisted
    ? storage.persisted()
    : Promise.reject(new Error('Storage persistence status is unavailable'))
  const [estimateResult, persistedResult] = await Promise.allSettled([
    estimate,
    persisted,
  ])

  if (signal.aborted) {
    throw signal.reason
  }

  return {
    supported: !!storage.estimate || !!storage.persisted,
    usageBytes:
      estimateResult.status === 'fulfilled'
        ? normalizeStorageBytes(estimateResult.value.usage)
        : null,
    quotaBytes:
      estimateResult.status === 'fulfilled'
        ? normalizeStorageBytes(estimateResult.value.quota)
        : null,
    persisted:
      persistedResult.status === 'fulfilled' ? persistedResult.value : null,
  }
}

export function buildStorageHealthView(
  provider: {
    loading: boolean
    supported: boolean
    totalBytes: bigint
    blockCount: bigint
  },
  sync: SessionSyncStatusView,
  browser: BrowserStorageSnapshot | undefined,
  browserLoading: boolean,
  browserError: Error | null,
  userAgent: string,
): StorageHealthView {
  const protectionState: StorageProtectionState = browserLoading
    ? 'checking'
    : browser?.persisted === true
      ? 'protected'
      : browser?.persisted === false
        ? 'not-protected'
        : 'unavailable'

  return {
    providerLoading: provider.loading,
    providerSupported: provider.supported,
    providerBytes: provider.totalBytes,
    blockCount: provider.blockCount,
    browserReadFailed: !!browserError,
    originUsageBytes: browser?.usageBytes ?? null,
    originQuotaBytes: browser?.quotaBytes ?? null,
    protectionState,
    sync,
    replicaLabel: 'Not yet verified',
    safariCleanupRisk: isSafariUserAgent(userAgent),
  }
}

export function isSafariUserAgent(userAgent: string): boolean {
  return (
    /Safari/i.test(userAgent) &&
    !/(Chrome|Chromium|CriOS|Edg|EdgiOS|OPR|FxiOS)/i.test(userAgent)
  )
}

function normalizeStorageBytes(value: number | undefined): number | null {
  return value != null && Number.isFinite(value) && value >= 0 ? value : null
}
