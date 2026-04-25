import type { ReactNode } from 'react'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SyncActivityDirection,
  SyncP2PState,
  SyncStatusState,
  SyncTransportState,
} from '@s4wave/sdk/session/session.pb.js'

import { SessionSyncStatusButton } from './SessionSyncStatusButton.js'
import {
  buildSessionSyncStatusView,
  type SessionSyncStatusView,
} from './SessionSyncStatusContext.js'

const mockUseSessionSyncStatus = vi.hoisted(() => vi.fn())
const mockToggle = vi.hoisted(() => vi.fn())
const mockSelected = vi.hoisted(() => ({ value: true }))

vi.mock('./SessionSyncStatusContext.js', async () => {
  const actual = await vi.importActual<
    typeof import('./SessionSyncStatusContext.js')
  >('./SessionSyncStatusContext.js')
  return {
    ...actual,
    useSessionSyncStatus: mockUseSessionSyncStatus,
  }
})

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: (props: {
    id: string
    position?: 'left' | 'right'
    button: (
      selected: boolean,
      onClick: () => void,
      className?: string,
    ) => ReactNode
    children?: ReactNode
  }) => (
    <div
      data-testid={`bottom-bar-level-${props.id}`}
      data-position={props.position ?? 'left'}
    >
      {props.button(mockSelected.value, mockToggle, '')}
      {props.children}
    </div>
  ),
}))

describe('SessionSyncStatusButton', () => {
  beforeEach(() => {
    mockToggle.mockClear()
    mockSelected.value = true
  })

  afterEach(() => {
    cleanup()
  })

  it.each([
    {
      name: 'idle cloud',
      view: view({
        state: SyncStatusState.SyncStatusState_SYNCED,
        transportState: SyncTransportState.SyncTransportState_ONLINE,
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
    },
    {
      name: 'active without visible work',
      view: view({
        state: SyncStatusState.SyncStatusState_ACTIVE,
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
      spinning: false,
    },
    {
      name: 'active upload',
      view: view({
        state: SyncStatusState.SyncStatusState_ACTIVE,
        direction: SyncActivityDirection.SyncActivityDirection_UPLOAD,
        uploadBytesPerSecond: 2048n,
        pendingUploadBytes: 4096n,
        activeUploadBytes: 8192n,
        activeUploadTransferredBytes: 2048n,
        inFlightUploadCount: 1,
      }),
      label: 'Uploading changes',
      detail: 'Uploading to cloud.',
    },
    {
      name: 'active download',
      view: view({
        state: SyncStatusState.SyncStatusState_ACTIVE,
        direction: SyncActivityDirection.SyncActivityDirection_DOWNLOAD,
        downloadBytesPerSecond: 4096n,
      }),
      label: 'Downloading updates',
      detail: 'Downloading from cloud.',
    },
    {
      name: 'mixed activity',
      view: view({
        state: SyncStatusState.SyncStatusState_ACTIVE,
        direction: SyncActivityDirection.SyncActivityDirection_UPLOAD_DOWNLOAD,
        uploadBytesPerSecond: 1024n,
        downloadBytesPerSecond: 2048n,
      }),
      label: 'Syncing changes',
      detail: 'Uploading and downloading.',
    },
    {
      name: 'error',
      view: view({
        state: SyncStatusState.SyncStatusState_ERROR,
        lastError: 'upload failed',
      }),
      label: 'Sync needs attention',
      detail: 'upload failed',
    },
    {
      name: 'local ready',
      view: view({
        state: SyncStatusState.SyncStatusState_SYNCED,
        transportState: SyncTransportState.SyncTransportState_UNAVAILABLE,
      }),
      label: 'Local ready',
      detail: 'Local session ready.',
    },
    {
      name: 'p2p active',
      view: view({
        state: SyncStatusState.SyncStatusState_SYNCED,
        transportState: SyncTransportState.SyncTransportState_ONLINE,
        p2pState: SyncP2PState.SyncP2PState_ACTIVE,
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
      p2pLabel: 'P2P active',
    },
    {
      name: 'p2p error',
      view: view({
        state: SyncStatusState.SyncStatusState_ERROR,
        p2pState: SyncP2PState.SyncP2PState_ERROR,
        lastError: 'pairing failed',
      }),
      label: 'Sync needs attention',
      detail: 'pairing failed',
      p2pLabel: 'P2P error',
    },
  ])('renders $name status in the collapsed button and popover', (test) => {
    mockUseSessionSyncStatus.mockReturnValue(test.view)

    render(<SessionSyncStatusButton />)

    expect(
      screen.getByTestId('bottom-bar-level-session-sync-status'),
    ).toBeTruthy()
    expect(
      screen
        .getByTestId('session-sync-status-button')
        .getAttribute('aria-label'),
    ).toBe(`Session sync status: ${test.label}`)
    expect(screen.getByText(test.label)).toBeTruthy()
    expect(screen.getAllByText(test.detail).length).toBeGreaterThan(0)
    if (test.p2pLabel) {
      expect(screen.getByText(test.p2pLabel)).toBeTruthy()
    }
    if (test.spinning === false) {
      expect(
        screen
          .getByTestId('session-sync-status-button')
          .querySelector('.animate-spin'),
      ).toBeNull()
    }
  })

  it('closes the popover on Escape', () => {
    mockUseSessionSyncStatus.mockReturnValue(
      view({
        state: SyncStatusState.SyncStatusState_SYNCED,
      }),
    )

    render(<SessionSyncStatusButton />)

    fireEvent.keyDown(screen.getByTestId('session-sync-status-popover'), {
      key: 'Escape',
    })

    expect(mockToggle).toHaveBeenCalledTimes(1)
  })

  it('renders compact pack-read diagnostics from the sync snapshot', () => {
    mockUseSessionSyncStatus.mockReturnValue(
      view({
        state: SyncStatusState.SyncStatusState_SYNCED,
        packRangeRequestCount: 3n,
        packRangeResponseBytes: 4096n,
        packIndexTailFetchCount: 1n,
        packIndexTailResponseBytes: 2048n,
        packLastOpenedPacks: 2,
        packLastCandidatePacks: 5,
        packIndexCacheHits: 7n,
        packIndexCacheMisses: 1n,
      }),
    )

    render(<SessionSyncStatusButton />)

    expect(screen.getByText('Pack reads')).toBeTruthy()
    expect(screen.getByText('3 / 4.0 KiB')).toBeTruthy()
    expect(screen.getByText('1 / 2.0 KiB')).toBeTruthy()
    expect(screen.getByText('2 opened / 5 candidates')).toBeTruthy()
    expect(screen.getByText('7 hits / 1 misses')).toBeTruthy()
  })

  it('renders in-flight upload progress separately from queued upload bytes', () => {
    mockUseSessionSyncStatus.mockReturnValue(
      view({
        state: SyncStatusState.SyncStatusState_ACTIVE,
        direction: SyncActivityDirection.SyncActivityDirection_UPLOAD,
        pendingUploadBytes: 65536n,
        activeUploadBytes: 32768n,
        activeUploadTransferredBytes: 8192n,
        inFlightUploadCount: 1,
      }),
    )

    render(<SessionSyncStatusButton />)

    expect(screen.getByText('Uploading now')).toBeTruthy()
    expect(screen.getByText('8.0 KiB / 32 KiB')).toBeTruthy()
    expect(screen.getByText('64 KiB')).toBeTruthy()
  })
})

function view(
  snapshot: Parameters<typeof buildSessionSyncStatusView>[0],
): SessionSyncStatusView {
  return buildSessionSyncStatusView(snapshot, false, null)
}
