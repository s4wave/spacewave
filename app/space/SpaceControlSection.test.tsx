import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { SOConfigChangeType } from '@s4wave/core/sobject/sobject.pb.js'
import {
  SpaceControl,
  type SpaceSharingState,
} from '@s4wave/sdk/space/space.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SpaceControlSection } from './SpaceControlSection.js'

const h = vi.hoisted(() => ({
  setSpaceControl: vi.fn(),
  approveSpaceChange: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContext: {
    useContext: () => ({
      value: {
        setSpaceControl: h.setSpaceControl,
        approveSpaceChange: h.approveSpaceChange,
      },
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

describe('SpaceControlSection', () => {
  function renderSection(state: SpaceSharingState) {
    return render(
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
        <SpaceControlSection />
      </SpaceContainerContext.Provider>,
    )
  }

  // group is three voters waiting on a change to remove bob.
  const group: SpaceSharingState = {
    control: SpaceControl.SpaceControl_GROUP,
    canVote: true,
    canSetControl: true,
    totalWeight: 3n,
    quorumWeight: 3n,
    participantInfo: [
      { entityId: 'alice', peerIds: ['peer-alice'], isSelf: true },
      { entityId: 'bob', peerIds: ['peer-bob'] },
      { entityId: 'carol', peerIds: ['peer-carol'] },
    ],
    groupChanges: [
      {
        hash: new Uint8Array([1, 2]),
        changeType: SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
        removedPeerIds: ['peer-bob'],
        control: SpaceControl.SpaceControl_GROUP,
        weight: 2n,
      },
    ],
  }

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('hands control to the group after the owner confirms', async () => {
    h.setSpaceControl.mockResolvedValue({})
    renderSection({
      control: SpaceControl.SpaceControl_OWNER,
      canManage: true,
      canSetControl: true,
    })

    fireEvent.click(screen.getByRole('radio', { name: /The group/ }))
    expect(h.setSpaceControl).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Hand control to the group' }),
    )
    expect(h.setSpaceControl).toHaveBeenCalledWith(
      SpaceControl.SpaceControl_GROUP,
    )
    await vi.waitFor(() =>
      expect(screen.queryByText('Hand control to the group?')).toBeNull(),
    )
    expect(toast.info).not.toHaveBeenCalled()
  })

  it('shows a waiting change and lets a voter agree', () => {
    h.approveSpaceChange.mockResolvedValue(undefined)
    renderSection(group)

    expect(
      screen.getByText(
        'Every change needs every member to agree. You have a vote.',
      ),
    ).toBeDefined()
    expect(screen.getByText('Remove bob')).toBeDefined()
    expect(screen.getByText('2 of 3 votes')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Agree' }))
    expect(h.approveSpaceChange).toHaveBeenCalledWith(new Uint8Array([1, 2]))
  })

  it('asks the group to give control back to the owner', async () => {
    h.setSpaceControl.mockResolvedValue({ awaitingGroup: true })
    renderSection({
      ...group,
      groupChanges: [{ ...group.groupChanges![0], viewerAgreed: true }],
    })

    expect(screen.getByText('You agreed')).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Agree' })).toBeNull()
    fireEvent.click(screen.getByRole('radio', { name: /The owner/ }))
    expect(h.setSpaceControl).toHaveBeenCalledWith(
      SpaceControl.SpaceControl_OWNER,
    )
    await vi.waitFor(() =>
      expect(toast.info).toHaveBeenCalledWith('Asked the group', {
        description: 'The change happens once enough members agree.',
      }),
    )
  })

  it('shows control to a member who cannot change it', () => {
    renderSection({ ...group, canVote: false, canSetControl: false })

    const owner = screen.getByRole('radio', { name: /The owner/ })
    expect((owner as HTMLButtonElement).disabled).toBe(true)
    expect(
      screen.getByText('Every change needs every member to agree.'),
    ).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Agree' })).toBeNull()
  })
})
