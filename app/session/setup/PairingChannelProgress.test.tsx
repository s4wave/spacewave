import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'

import { PairingChannelProgress } from './PairingChannelProgress.js'

describe('PairingChannelProgress', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('binds the title to the active checklist step', () => {
    render(
      <PairingChannelProgress
        status={PairingStatus.PairingStatus_PEER_CONNECTED}
      />,
    )

    expect(
      screen.getByRole('heading', { name: 'Establishing encrypted channel' }),
    ).toBeDefined()
    expect(screen.getAllByText('Establishing encrypted channel')).toHaveLength(
      2,
    )
    expect(
      screen.queryByRole('heading', {
        name: 'Preparing connection verification',
      }),
    ).toBeNull()
  })

  it('shows a quiet stall line when the active step does not advance', () => {
    vi.useFakeTimers()
    render(<PairingChannelProgress stallTimeoutMs={500} />)

    expect(
      screen.queryByText('Still working… this is taking longer than usual'),
    ).toBeNull()

    act(() => {
      vi.advanceTimersByTime(500)
    })

    expect(
      screen.getByText('Still working… this is taking longer than usual'),
    ).toBeDefined()
  })

  it('sets determinate progress from completed checklist steps', () => {
    const { container, rerender } = render(<PairingChannelProgress />)

    expect(container.querySelector('[data-progress="0"]')).not.toBeNull()
    expect(screen.getByText('0%')).toBeDefined()

    rerender(
      <PairingChannelProgress
        status={PairingStatus.PairingStatus_VERIFYING_EMOJI}
      />,
    )

    expect(container.querySelector('[data-progress="67"]')).not.toBeNull()
    expect(screen.getByText('67%')).toBeDefined()
  })
})
