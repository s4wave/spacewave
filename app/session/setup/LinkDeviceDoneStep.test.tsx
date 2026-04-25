import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { LinkDeviceDoneStep } from './LinkDeviceDoneStep.js'
import type { Session } from '@s4wave/sdk/session/session.js'

interface PairedDeviceSnapshot {
  pairedDevices?: Array<{ peerId?: string }>
}

function watchSnapshots(
  signal: AbortSignal | undefined,
  snapshots: PairedDeviceSnapshot[],
) {
  return {
    async *[Symbol.asyncIterator]() {
      for (const snapshot of snapshots) {
        if (signal?.aborted) {
          return
        }
        yield snapshot
      }
      await new Promise<void>((resolve) => {
        if (signal?.aborted) {
          resolve()
          return
        }
        signal?.addEventListener('abort', () => resolve(), { once: true })
      })
    },
  }
}

function buildSession(snapshots: PairedDeviceSnapshot[]) {
  return {
    confirmPairing: vi.fn(() => Promise.resolve()),
    watchPairedDevices: vi.fn((signal?: AbortSignal) =>
      watchSnapshots(signal, snapshots),
    ),
  } as unknown as Session
}

describe('LinkDeviceDoneStep', () => {
  const onDone = vi.fn()
  const onLinkMore = vi.fn()

  beforeEach(() => {
    onDone.mockClear()
    onLinkMore.mockClear()
  })

  afterEach(() => {
    cleanup()
  })

  it('waits for the watched target peer before showing success', async () => {
    const session = buildSession([])

    render(
      <LinkDeviceDoneStep
        session={session}
        remotePeerId="peer-target"
        onDone={onDone}
        onLinkMore={onLinkMore}
      />,
    )

    await waitFor(() => {
      expect(session.confirmPairing).toHaveBeenCalledWith(
        'peer-target',
        '',
        expect.any(AbortSignal),
      )
    })

    await waitFor(() => {
      expect(screen.getByText('Finishing device sync...')).toBeDefined()
    })
    expect(screen.queryByText('All set!')).toBeNull()
    expect(screen.getByText('Skip and continue to dashboard')).toBeDefined()
    expect(screen.queryByText('Dashboard')).toBeNull()
  })

  it('ignores unrelated paired-device rows until the target peer appears', async () => {
    const session = buildSession([
      { pairedDevices: [{ peerId: 'peer-other' }] },
      { pairedDevices: [{ peerId: 'peer-target' }] },
    ])

    render(
      <LinkDeviceDoneStep
        session={session}
        remotePeerId="peer-target"
        onDone={onDone}
        onLinkMore={onLinkMore}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('All set!')).toBeDefined()
    })
    expect(screen.getByText('Dashboard')).toBeDefined()
    expect(screen.getByText('Link more')).toBeDefined()
    expect(screen.queryByText('Skip and continue to dashboard')).toBeNull()
  })

  it('restarts confirmation state when the flow targets a different peer', async () => {
    const firstSession = buildSession([
      { pairedDevices: [{ peerId: 'peer-a' }] },
    ])
    const secondSession = buildSession([])

    const { rerender } = render(
      <LinkDeviceDoneStep
        session={firstSession}
        remotePeerId="peer-a"
        onDone={onDone}
        onLinkMore={onLinkMore}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('All set!')).toBeDefined()
    })

    rerender(
      <LinkDeviceDoneStep
        session={secondSession}
        remotePeerId="peer-b"
        onDone={onDone}
        onLinkMore={onLinkMore}
      />,
    )

    await waitFor(() => {
      expect(secondSession.confirmPairing).toHaveBeenCalledWith(
        'peer-b',
        '',
        expect.any(AbortSignal),
      )
    })
    expect(screen.getByText('Finishing device sync...')).toBeDefined()
    expect(screen.queryByText('All set!')).toBeNull()
  })
})
