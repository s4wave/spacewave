import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'

import {
  SOParticipantRole,
  type SOSyncPeer,
} from '@s4wave/core/sobject/sobject.pb.js'
import {
  SpaceSequencer,
  type SpaceSharingState,
} from '@s4wave/sdk/space/space.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SpaceSyncSection } from './SpaceSyncSection.js'

const h = vi.hoisted(() => ({
  setSpaceSequencer: vi.fn().mockResolvedValue({ changed: true }),
  unorderedCount: 0,
  syncPeers: [] as SOSyncPeer[],
}))

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: () => ({
    health: { unorderedCount: h.unorderedCount, syncPeers: h.syncPeers },
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: () => ({ value: {} }) },
  SpaceContext: {
    useContext: () => ({
      value: { setSpaceSequencer: h.setSpaceSequencer },
    }),
  },
}))

vi.mock('@s4wave/web/state/index.js', () => ({
  useStateAtom: <T,>(_atom: unknown, _key: string, initial: T) =>
    useState<T>(initial),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: vi.fn(), info: vi.fn() },
}))

describe('SpaceSyncSection', () => {
  const otherPeer = '12D3KooWOther'

  function renderSection(state: SpaceSharingState) {
    return render(section(state))
  }

  function section(state: SpaceSharingState) {
    return (
      <SpaceContainerContext.Provider
        spaceId="test-space"
        spaceState={{ ready: true }}
        spaceSharingState={state}
        spaceWorldResource={{
          value: null,
          loading: true,
          error: null,
          retry: vi.fn(),
        }}
        spaceWorld={{} as EngineWorldState}
        navigateToRoot={vi.fn()}
        navigateToObjects={vi.fn()}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <SpaceSyncSection />
      </SpaceContainerContext.Provider>
    )
  }

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.unorderedCount = 0
    h.syncPeers = []
  })

  it('lets an owner of a cloud Space choose Spacewave Cloud', () => {
    renderSection({
      canManage: true,
      sequencer: SpaceSequencer.SpaceSequencer_MERGE,
      sequencerChoices: [
        SpaceSequencer.SpaceSequencer_MERGE,
        SpaceSequencer.SpaceSequencer_PROVIDER,
      ],
    })

    expect(
      screen.getByText('Edits take turns through Spacewave Cloud.'),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('radio', { name: /One order/ }))
    expect(h.setSpaceSequencer).toHaveBeenCalledWith(
      SpaceSequencer.SpaceSequencer_PROVIDER,
    )
  })

  it('keeps a choice selected until the watched state shows it', async () => {
    let resolve = (_: { changed: boolean }) => {}
    h.setSpaceSequencer.mockReturnValueOnce(
      new Promise((r) => {
        resolve = r
      }),
    )
    const merge: SpaceSharingState = {
      canManage: true,
      sequencer: SpaceSequencer.SpaceSequencer_MERGE,
      sequencerChoices: [
        SpaceSequencer.SpaceSequencer_MERGE,
        SpaceSequencer.SpaceSequencer_PROVIDER,
      ],
    }
    const { rerender } = renderSection(merge)

    const oneOrder = screen.getByRole('radio', { name: /One order/ })
    fireEvent.click(oneOrder)
    expect(oneOrder.getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('status').textContent).toBe(
      'Switching to One order…',
    )

    await act(async () => resolve({ changed: true }))
    expect(screen.getByRole('status').textContent).toBe(
      'Switching to One order…',
    )

    rerender(
      section({ ...merge, sequencer: SpaceSequencer.SpaceSequencer_PROVIDER }),
    )
    expect(screen.queryByRole('status')).toBeNull()
    expect(
      screen.getByText('Spacewave Cloud puts every edit in order.'),
    ).toBeDefined()
  })

  it('returns to the watched choice when the change fails', async () => {
    h.setSpaceSequencer.mockRejectedValueOnce(new Error('offline'))
    renderSection({
      canManage: true,
      sequencer: SpaceSequencer.SpaceSequencer_MERGE,
      sequencerChoices: [
        SpaceSequencer.SpaceSequencer_MERGE,
        SpaceSequencer.SpaceSequencer_PROVIDER,
      ],
    })

    await act(async () =>
      fireEvent.click(screen.getByRole('radio', { name: /One order/ })),
    )
    expect(
      screen.getByRole('radio', { name: /Merge/ }).getAttribute('aria-checked'),
    ).toBe('true')
    expect(screen.queryByRole('status')).toBeNull()
    expect(toast.error).toHaveBeenCalled()
  })

  it('shows the choice without letting a writer change it', () => {
    renderSection({
      canManage: false,
      sequencer: SpaceSequencer.SpaceSequencer_PROVIDER,
      sequencerChoices: [],
    })

    const merge = screen.getByRole('radio', { name: /Merge/ })
    expect((merge as HTMLButtonElement).disabled).toBe(true)
    expect(
      screen.getByText('Spacewave Cloud puts every edit in order.'),
    ).toBeDefined()
  })

  it('names the main device and replaces it when lost', () => {
    h.unorderedCount = 3
    renderSection({
      canManage: true,
      sequencer: SpaceSequencer.SpaceSequencer_OTHER_DEVICE,
      sequencerPeerId: otherPeer,
      sequencerChoices: [
        SpaceSequencer.SpaceSequencer_MERGE,
        SpaceSequencer.SpaceSequencer_THIS_DEVICE,
      ],
      participantInfo: [
        {
          entityId: 'alice',
          peerIds: [otherPeer],
          role: SOParticipantRole.SOParticipantRole_OWNER,
        },
      ],
    })

    expect(
      screen.getByText("alice's device puts every edit in order."),
    ).toBeDefined()
    expect(
      screen.getByText('3 edits are waiting to be put in order.'),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: /main device is lost/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace main device' }))
    expect(h.setSpaceSequencer).toHaveBeenCalledWith(
      SpaceSequencer.SpaceSequencer_THIS_DEVICE,
    )
  })

  it('shows the sequencer peer ID in technical details', () => {
    renderSection({
      canManage: true,
      sequencer: SpaceSequencer.SpaceSequencer_OTHER_DEVICE,
      sequencerPeerId: otherPeer,
    })

    expect(screen.queryByTitle(otherPeer)).toBeNull()
    fireEvent.click(screen.getByText('Show technical details'))
    expect(screen.getByTitle(otherPeer)).toBeDefined()
  })

  it('lists each synced device with its connection state', () => {
    const lostPeer = '12D3KooWLost'
    h.syncPeers = [
      { peerId: otherPeer, connected: true, since: new Date() },
      {
        peerId: lostPeer,
        connected: false,
        since: new Date(),
        error: 'stream reset',
      },
    ]
    renderSection({
      sequencer: SpaceSequencer.SpaceSequencer_MERGE,
      participantInfo: [{ isSelf: true, peerIds: [otherPeer] }],
    })

    const list = screen.getByRole('list', { name: 'Other devices' })
    const rows = within(list).getAllByRole('listitem')
    expect(within(rows[0]).getByText('Your other device')).toBeDefined()
    expect(within(rows[0]).getByText(/^Connected since /)).toBeDefined()
    expect(within(rows[1]).getByText('Unknown device')).toBeDefined()
    expect(within(rows[1]).getByText(/^Disconnected since /)).toBeDefined()
    expect(within(rows[1]).getByText('stream reset')).toBeDefined()
    expect(screen.queryByTitle(lostPeer)).toBeNull()
    fireEvent.click(screen.getByText('Show technical details'))
    expect(screen.getByTitle(lostPeer)).toBeDefined()
  })

  it('says when no other device has synced', () => {
    renderSection({ sequencer: SpaceSequencer.SpaceSequencer_MERGE })

    expect(
      screen.getByText('No other device has synced since this device started.'),
    ).toBeDefined()
  })
})
