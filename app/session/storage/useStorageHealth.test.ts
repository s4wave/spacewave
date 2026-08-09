import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { buildSessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'

import {
  buildStorageHealthView,
  isSafariUserAgent,
  readBrowserStorageHealth,
  useStorageHealth,
} from './useStorageHealth.js'

const mockWebDocument = vi.hoisted(() => ({
  readStoragePersistenceStatus: vi.fn(),
  requestStoragePersistence: vi.fn(),
}))

vi.mock('@aptre/bldr-react', () => ({
  useBldrContext: () => ({ webDocument: mockWebDocument }),
}))

vi.mock('@s4wave/app/session/SessionStorageStatsContext.js', () => ({
  useSessionStorageStats: () => ({
    loading: false,
    supported: true,
    totalBytes: 0n,
    blockCount: 0n,
  }),
}))

vi.mock('@s4wave/app/session/SessionSyncStatusContext.js', async (orig) => {
  const mod =
    await orig<
      typeof import('@s4wave/app/session/SessionSyncStatusContext.js')
    >()
  return {
    ...mod,
    useSessionSyncStatus: () =>
      mod.buildSessionSyncStatusView(null, false, null),
  }
})

describe('useStorageHealth', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    mockWebDocument.readStoragePersistenceStatus.mockReset()
    mockWebDocument.requestStoragePersistence.mockReset()
  })

  it('re-reads protection state after an explicit request', async () => {
    vi.stubGlobal('navigator', {
      storage: {
        estimate: () => Promise.resolve({ usage: 1_024, quota: 4_096 }),
      },
      userAgent: 'Mozilla/5.0 Chrome/136.0.0.0 Safari/537.36',
    })
    mockWebDocument.readStoragePersistenceStatus.mockResolvedValue(
      'not-persisted',
    )
    mockWebDocument.requestStoragePersistence.mockImplementation(() => {
      mockWebDocument.readStoragePersistenceStatus.mockResolvedValue(
        'persisted',
      )
      return Promise.resolve()
    })

    const { result } = renderHook(() => useStorageHealth())
    await waitFor(() =>
      expect(result.current.protectionState).toBe('not-protected'),
    )

    await act(() => result.current.requestProtection())

    await waitFor(() =>
      expect(result.current.protectionState).toBe('protected'),
    )
    expect(mockWebDocument.requestStoragePersistence).toHaveBeenCalledTimes(1)
  })
})

describe('storage health state', () => {
  it('keeps an available estimate when the persistence query fails', async () => {
    const snapshot = await readBrowserStorageHealth(
      {
        estimate: vi.fn(() => Promise.resolve({ usage: 4_096, quota: 16_384 })),
        persisted: vi.fn(() => Promise.reject(new Error('denied'))),
      },
      new AbortController().signal,
    )

    expect(snapshot).toEqual({
      supported: true,
      usageBytes: 4_096,
      quotaBytes: 16_384,
      persisted: null,
    })
  })

  it('projects protected local storage without treating sync as a replica', () => {
    const view = buildStorageHealthView(
      {
        loading: false,
        supported: true,
        totalBytes: 8_192n,
        blockCount: 12n,
      },
      buildSessionSyncStatusView(null, false, null),
      {
        supported: true,
        usageBytes: 12_288,
        quotaBytes: 65_536,
        persisted: true,
      },
      false,
      null,
      'Mozilla/5.0 Version/18.5 Safari/605.1.15',
    )

    expect(view.protectionState).toBe('protected')
    expect(view.providerBytes).toBe(8_192n)
    expect(view.safariCleanupRisk).toBe(true)
  })

  it('does not apply Safari cleanup copy to Chromium-family user agents', () => {
    expect(
      isSafariUserAgent(
        'Mozilla/5.0 AppleWebKit/537.36 Chrome/136.0.0.0 Safari/537.36',
      ),
    ).toBe(false)
    expect(
      isSafariUserAgent(
        'Mozilla/5.0 iPhone AppleWebKit/605.1.15 CriOS/136.0 Mobile Safari/604.1',
      ),
    ).toBe(false)
  })
})
