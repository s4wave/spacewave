import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { StorageHealth } from './StorageHealth.js'

const mockUseStorageHealth = vi.hoisted(() => vi.fn())

vi.mock('./useStorageHealth.js', () => ({
  useStorageHealth: mockUseStorageHealth,
}))

const healthView = {
  providerLoading: false,
  providerSupported: true,
  providerBytes: 16n * 1024n * 1024n,
  blockCount: 42n,
  browserReadFailed: false,
  originUsageBytes: 32 * 1024 * 1024,
  originQuotaBytes: 256 * 1024 * 1024,
  protectionState: 'protected',
  sync: {
    summaryLabel: 'Sync idle',
    detailLabel: 'No active transfers.',
    error: false,
    loading: false,
    active: false,
    lastError: '',
    pendingUploadLabel: '0 B',
    pendingDownloadLabel: '0 B',
  },
  requestProtection: vi.fn(),
  safariCleanupRisk: true,
}

describe('StorageHealth', () => {
  afterEach(() => cleanup())

  it('groups readings by scope and keeps backup claims subordinate', () => {
    mockUseStorageHealth.mockReturnValue(healthView)
    const onOpenRecovery = vi.fn()
    const onLinkDevice = vi.fn()
    const onUseCloud = vi.fn()

    render(
      <StorageHealth
        mode="settings"
        onOpenRecovery={onOpenRecovery}
        onLinkDevice={onLinkDevice}
        onUseCloud={onUseCloud}
      />,
    )

    expect(screen.getByRole('heading', { name: 'This browser' })).toBeTruthy()
    expect(
      screen.getByRole('heading', { name: "This device's store" }),
    ).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Sync activity' })).toBeTruthy()
    expect(screen.getByText('16 MB physical')).toBeTruthy()
    expect(screen.getByText('Automatic cleanup protection')).toBeTruthy()
    fireEvent.click(screen.getByText('Storage recovery'))
    fireEvent.click(screen.getByText('Request protection'))
    fireEvent.click(screen.getByText('Link device'))
    fireEvent.click(screen.getByText('Cloud options'))
    expect(healthView.requestProtection).toHaveBeenCalledOnce()
    expect(onOpenRecovery).toHaveBeenCalledOnce()
    expect(onLinkDevice).toHaveBeenCalledOnce()
    expect(onUseCloud).toHaveBeenCalledOnce()
    expect(screen.queryByText(/install/i)).toBeNull()
  })

  it('presents a calm recovery view when no write failure is reported', () => {
    mockUseStorageHealth.mockReturnValue(healthView)

    render(
      <StorageHealth
        mode="recovery"
        onLinkDevice={() => {}}
        onUseCloud={() => {}}
      />,
    )

    expect(screen.getByText('Saving works')).toBeTruthy()
    expect(screen.queryByText('Spacewave cannot save reliably')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByText('Why these readings matter')).toBeNull()
    expect(
      screen.getByText('Request automatic cleanup protection'),
    ).toBeTruthy()
    expect(screen.getByText('Export a backup')).toBeTruthy()
    expect(screen.queryByTestId('safari-storage-risk')).toBeNull()
  })
})
