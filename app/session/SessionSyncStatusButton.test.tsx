import { useState } from 'react'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  SyncActivityDirection,
  SyncP2PState,
  SyncStatusState,
  SyncAccountAccessState,
} from '@s4wave/sdk/session/session.pb.js'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'

import { SessionSyncStatusButton } from './SessionSyncStatusButton.js'
import {
  buildSessionSyncStatusView,
  type SessionSyncStatusView,
} from './SessionSyncStatusContext.js'

const mockUseSessionSyncStatus = vi.hoisted(() => vi.fn())

vi.mock('./SessionSyncStatusContext.js', async () => {
  const actual = await vi.importActual<
    typeof import('./SessionSyncStatusContext.js')
  >('./SessionSyncStatusContext.js')
  return {
    ...actual,
    useSessionSyncStatus: mockUseSessionSyncStatus,
    useSessionSyncSummary: mockUseSessionSyncStatus,
  }
})

function Harness() {
  const [openMenu, setOpenMenu] = useState('session-sync-status')
  return (
    <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
      <SessionSyncStatusButton />
      <ViewerFrame />
    </BottomBarRoot>
  )
}

describe('SessionSyncStatusButton', () => {
  afterEach(() => {
    cleanup()
  })

  it.each([
    {
      name: 'connected account with an incomplete local copy',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_LOCAL,
        peer: {
          activePeerCount: 1,
          localCopies: [
            {
              sharedObjectId: 'drive',
              displayName: 'My Drive',
              complete: false,
              bytes: 4096n,
            },
          ],
          transfer: {
            state: SyncStatusState.SyncStatusState_ACTIVE,
            pendingDownloadCount: 1,
          },
        },
      }),
      label: 'Copying Spaces',
      detail:
        '0 of 1 Spaces available offline. You can use your account while copying continues.',
    },
    {
      name: 'durable copy with no peer online',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_LOCAL,
        peer: {
          activePeerCount: 0,
          localCopies: [
            {
              sharedObjectId: 'drive',
              displayName: 'My Drive',
              complete: true,
              bytes: 4096n,
            },
          ],
          transfer: {},
        },
      }),
      label: 'Available offline',
      detail:
        'The latest received Spaces and their contents are stored on this device.',
      spinning: false,
    },
    {
      name: 'local copy out of browser storage',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_LOCAL,
        peer: {
          activePeerCount: 1,
          localCopies: [
            {
              sharedObjectId: 'drive',
              displayName: 'My Drive',
              complete: false,
              error:
                'seg-1: getFileHandle: QuotaExceededError: The operation failed because it would cause the application to exceed its storage quota.',
            },
          ],
          transfer: {},
        },
      }),
      label: 'Storage full',
      detail:
        'This browser is out of storage, so 1 Space cannot be stored on this device. Free up space to continue.',
    },
    {
      name: 'idle cloud',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: { transfer: { state: SyncStatusState.SyncStatusState_SYNCED } },
        peer: {},
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
    },
    {
      name: 'active without visible work',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: { transfer: { state: SyncStatusState.SyncStatusState_ACTIVE } },
        peer: {},
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
      spinning: false,
    },
    {
      name: 'active upload',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: {
          transfer: {
            state: SyncStatusState.SyncStatusState_ACTIVE,
            direction: SyncActivityDirection.SyncActivityDirection_UPLOAD,
            pendingUploadBytes: 4096n,
            uploadBytesPerSecond: 2048n,
            activeUploadBytes: 8192n,
            activeUploadTransferredBytes: 2048n,
            inFlightUploadCount: 1,
          },
        },
        peer: {},
      }),
      label: 'Uploading changes',
      detail: 'Uploading to cloud.',
    },
    {
      name: 'active download',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: {
          transfer: {
            state: SyncStatusState.SyncStatusState_ACTIVE,
            direction: SyncActivityDirection.SyncActivityDirection_DOWNLOAD,
            downloadBytesPerSecond: 4096n,
          },
        },
        peer: {},
      }),
      label: 'Downloading updates',
      detail: 'Downloading from cloud.',
    },
    {
      name: 'local download from peers',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_LOCAL,
        peer: {
          transfer: {
            state: SyncStatusState.SyncStatusState_ACTIVE,
            direction: SyncActivityDirection.SyncActivityDirection_DOWNLOAD,
            downloadBytesPerSecond: 4096n,
          },
        },
      }),
      label: 'Downloading updates',
      detail: 'Downloading from peers.',
    },
    {
      name: 'mixed activity',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: {
          transfer: {
            state: SyncStatusState.SyncStatusState_ACTIVE,
            direction:
              SyncActivityDirection.SyncActivityDirection_UPLOAD_DOWNLOAD,
            uploadBytesPerSecond: 1024n,
            downloadBytesPerSecond: 2048n,
          },
        },
        peer: {},
      }),
      label: 'Syncing changes',
      detail: 'Uploading and downloading.',
    },
    {
      name: 'error',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: {
          transfer: {
            state: SyncStatusState.SyncStatusState_ERROR,
            lastError: 'upload failed',
          },
        },
        peer: {},
      }),
      label: 'Sync needs attention',
      detail: 'upload failed',
    },
    {
      name: 'local ready',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_LOCAL,
        peer: { transfer: { state: SyncStatusState.SyncStatusState_SYNCED } },
      }),
      label: 'Local ready',
      detail: 'Local session ready.',
    },
    {
      name: 'p2p active',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: { transfer: { state: SyncStatusState.SyncStatusState_SYNCED } },
        peer: { state: SyncP2PState.SyncP2PState_ACTIVE },
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
      p2pLabel: 'P2P active',
    },
    {
      name: 'cloud success with a p2p error',
      view: view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: { transfer: { state: SyncStatusState.SyncStatusState_SYNCED } },
        peer: {
          state: SyncP2PState.SyncP2PState_ERROR,
          lastError: 'pairing failed',
        },
      }),
      label: 'Synced',
      detail: 'All sync work complete.',
      peerError: 'pairing failed',
      p2pLabel: 'P2P error',
    },
  ])('renders $name status in the collapsed button and popover', (test) => {
    mockUseSessionSyncStatus.mockReturnValue(test.view)

    render(<Harness />)

    expect(screen.getByTestId('session-sync-status-button')).toBeTruthy()
    expect(
      screen
        .getByTestId('session-sync-status-button')
        .getAttribute('aria-label'),
    ).toBe(`Session sync status: ${test.label}`)
    expect(screen.getAllByText(test.label).length).toBeGreaterThan(0)
    expect(screen.getAllByText(test.detail).length).toBeGreaterThan(0)
    if (test.peerError) {
      expect(screen.getByText(test.peerError)).toBeTruthy()
    }
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
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: { transfer: { state: SyncStatusState.SyncStatusState_SYNCED } },
        peer: {},
      }),
    )

    render(<Harness />)

    fireEvent.keyDown(screen.getByTestId('session-sync-status-popover'), {
      key: 'Escape',
    })

    expect(screen.queryByTestId('session-sync-status-popover')).toBeNull()
  })

  it('renders compact pack-read diagnostics from the sync snapshot', () => {
    mockUseSessionSyncStatus.mockReturnValue(
      view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: {
          transfer: { state: SyncStatusState.SyncStatusState_SYNCED },
          packRangeRequestCount: 3n,
          packRangeResponseBytes: 4096n,
          packIndexTailFetchCount: 1n,
          packIndexTailResponseBytes: 2048n,
          packLastOpenedPacks: 2,
          packLastCandidatePacks: 5,
          packIndexCacheHits: 7n,
          packIndexCacheMisses: 1n,
        },
        peer: {},
      }),
    )

    render(<Harness />)

    fireEvent.click(screen.getByText('Storage diagnostics'))
    expect(screen.getByText('3 / 4.0 KiB')).toBeTruthy()
    expect(screen.getByText('1 / 2.0 KiB')).toBeTruthy()
    expect(screen.getByText('2 opened / 5 candidates')).toBeTruthy()
    expect(screen.getByText('7 hits / 1 misses')).toBeTruthy()
  })

  it('renders in-flight upload progress separately from queued upload bytes', () => {
    mockUseSessionSyncStatus.mockReturnValue(
      view({
        access: SyncAccountAccessState.SyncAccountAccessState_READY,
        cloud: {
          transfer: {
            state: SyncStatusState.SyncStatusState_ACTIVE,
            direction: SyncActivityDirection.SyncActivityDirection_UPLOAD,
            pendingUploadBytes: 65536n,
            activeUploadBytes: 32768n,
            activeUploadTransferredBytes: 8192n,
            inFlightUploadCount: 1,
          },
        },
        peer: {},
      }),
    )

    render(<Harness />)

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
