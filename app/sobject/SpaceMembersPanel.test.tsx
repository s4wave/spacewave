import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { SpaceSharingState } from '@s4wave/sdk/space/space.pb.js'

import { SpaceMembersPanel } from './SpaceMembersPanel.js'

const mockSession = {
  spacewave: {
    removeSpaceMember: vi.fn(),
    processMailboxEntry: vi.fn(),
  },
  removeSpaceParticipant: vi.fn(),
  revokeSpaceInvite: vi.fn(),
  resourceRef: { resourceId: 1, released: false },
  id: 1,
  client: {},
  service: {},
} as unknown as Session

function renderPanel(spaceSharingState: SpaceSharingState) {
  return render(
    <SessionContext.Provider
      resource={
        {
          value: mockSession,
          loading: false,
          error: null,
          retry: vi.fn(),
        } as Resource<Session>
      }
    >
      <SpaceContainerContext.Provider
        spaceId="space-1"
        spaceState={{ ready: true }}
        spaceSharingState={spaceSharingState}
        spaceWorldResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        spaceWorld={{} as never}
        navigateToRoot={vi.fn()}
        navigateToObjects={vi.fn()}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <SpaceMembersPanel />
      </SpaceContainerContext.Provider>
    </SessionContext.Provider>,
  )
}

describe('SpaceMembersPanel', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders attested labels for active members and pending requests', () => {
    renderPanel({
      participantInfo: [
        {
          entityId: 'alice',
          accountId: 'acct-alice',
          peerIds: ['peer-alice'],
          role: 2,
        },
      ],
      invites: [
        {
          inviteId: 'invite-1',
          uses: 0,
          maxUses: 1,
          role: 2,
        },
      ],
      mailboxEntries: [
        {
          id: BigInt(7),
          status: 'pending',
          entityId: 'casey',
          accountId: 'acct-casey',
          peerId: 'peer-casey',
          inviteId: 'invite-1',
        },
      ],
    } as SpaceSharingState)

    expect(screen.getByText('alice')).toBeDefined()
    expect(screen.getByText('acct-alice')).toBeDefined()
    expect(screen.getByTestId('pending-request-label').textContent).toBe(
      'casey',
    )
    expect(screen.getByText('acct-casey')).toBeDefined()
    expect(screen.getByText('via invite-1')).toBeDefined()
  })

  it('shows an explicit empty pending state when invites exist without requests', () => {
    renderPanel({
      invites: [
        {
          inviteId: 'invite-1',
          uses: 0,
          maxUses: 1,
          role: 2,
        },
      ],
      mailboxEntries: [],
    } as SpaceSharingState)

    expect(screen.getByText('Pending Requests')).toBeDefined()
    expect(screen.getByTestId('pending-request-empty').textContent).toBe(
      'No pending requests yet',
    )
  })
})
