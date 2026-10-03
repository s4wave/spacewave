import { render, cleanup } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SharedObjectHealthStatus } from '@s4wave/core/sobject/sobject.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SharedObjectSyncNotice } from './SharedObjectSyncNotice.js'

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { warning: vi.fn(), dismiss: vi.fn() },
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('SharedObjectSyncNotice', () => {
  it('keeps recovery visible after admission and clears only on convergence', () => {
    const { rerender } = render(
      <SharedObjectSyncNotice
        health={{
          status: SharedObjectHealthStatus.READY,
          syncRecoveryPeerIds: ['private-participant'],
          syncDeniedPeerIds: ['private-participant'],
        }}
      />,
    )
    expect(toast.warning).toHaveBeenLastCalledWith(
      'Direct sync needs attention',
      expect.objectContaining({
        description: expect.stringContaining('ask the owner for a new invite'),
        duration: Infinity,
      }),
    )
    rerender(
      <SharedObjectSyncNotice
        health={{
          status: SharedObjectHealthStatus.READY,
          syncRecoveryPeerIds: ['private-participant'],
          syncDeniedPeerIds: [],
        }}
      />,
    )
    expect(toast.warning).toHaveBeenLastCalledWith(
      'Direct sync needs attention',
      expect.objectContaining({
        description: expect.stringContaining(
          'You can keep using local content.',
        ),
      }),
    )
    vi.mocked(toast.dismiss).mockClear()
    rerender(<SharedObjectSyncNotice health={{ syncRecoveryPeerIds: [] }} />)
    expect(toast.dismiss).toHaveBeenCalledTimes(1)
  })

  it('distinguishes a source refusal from loss of local access', () => {
    render(
      <SharedObjectSyncNotice health={{ syncDeniedPeerIds: ['source'] }} />,
    )
    expect(toast.warning).toHaveBeenCalledWith(
      'Direct sync needs attention',
      expect.objectContaining({
        description:
          'A connected device declined to sync this Space. Ask the owner to confirm your access. You can keep using local content.',
      }),
    )
  })

  it('shows each rejected edit until it leaves the health', () => {
    const lost = {
      opHash: new Uint8Array([1, 2]),
      reason: 'the operation failed',
      lostToPeerIds: ['other-member'],
    }
    const refused = {
      opHash: new Uint8Array([3]),
      reason: 'its author could not write to the shared object',
    }
    const { rerender } = render(
      <SharedObjectSyncNotice health={{ rejectedEdits: [lost, refused] }} />,
    )
    expect(toast.warning).toHaveBeenCalledWith(
      "An edit didn't apply",
      expect.objectContaining({
        description: expect.stringContaining(
          "Another member's change reached the Space first",
        ),
        duration: Infinity,
      }),
    )
    expect(toast.warning).toHaveBeenCalledWith(
      "An edit didn't apply",
      expect.objectContaining({
        description:
          'This edit no longer applies because its author could not write to the shared object. Your other edits are kept.',
      }),
    )

    vi.mocked(toast.warning).mockClear()
    rerender(<SharedObjectSyncNotice health={{ rejectedEdits: [refused] }} />)
    expect(toast.warning).not.toHaveBeenCalled()
    expect(toast.dismiss).toHaveBeenCalledTimes(1)
    expect(vi.mocked(toast.dismiss).mock.calls[0]?.[0]).toMatch(/:0102$/)
  })
})
