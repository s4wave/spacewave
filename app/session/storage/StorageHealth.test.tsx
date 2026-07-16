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
  },
  replicaLabel: 'Not yet verified',
  safariCleanupRisk: true,
}

describe('StorageHealth', () => {
  afterEach(() => cleanup())

  it('keeps local, protected, and replica states distinct in settings', () => {
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

    expect(screen.getByText('16 MB in the local provider')).toBeTruthy()
    expect(screen.getByText('Protected')).toBeTruthy()
    expect(screen.getByText('Not yet verified')).toBeTruthy()
    expect(screen.getByTestId('safari-storage-risk')).toBeTruthy()
    expect(screen.queryByText(/install/i)).toBeNull()

    fireEvent.click(screen.getByText('Recovery view'))
    fireEvent.click(screen.getByText('Link device'))
    fireEvent.click(screen.getByText('Cloud options'))
    expect(onOpenRecovery).toHaveBeenCalledOnce()
    expect(onLinkDevice).toHaveBeenCalledOnce()
    expect(onUseCloud).toHaveBeenCalledOnce()
  })

  it('uses the same health facts on recovery without Safari-only settings copy', () => {
    mockUseStorageHealth.mockReturnValue(healthView)

    render(
      <StorageHealth
        mode="recovery"
        onLinkDevice={() => {}}
        onUseCloud={() => {}}
      />,
    )

    expect(screen.getByText('Spacewave cannot save reliably')).toBeTruthy()
    expect(screen.getByRole('alert')).toBeTruthy()
    expect(screen.getByText('Why these readings matter')).toBeTruthy()
    expect(screen.getByText('Not yet verified')).toBeTruthy()
    expect(screen.queryByTestId('safari-storage-risk')).toBeNull()
    expect(
      screen.getByText(
        'Unavailable until this browser verifies that a current remote replica can restore the Space.',
      ),
    ).toBeTruthy()
  })
})
