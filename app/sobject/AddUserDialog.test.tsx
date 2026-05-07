import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { AddUserDialog } from './AddUserDialog.js'

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children: React.ReactNode }) => (
    <h1>{children}</h1>
  ),
  DialogDescription: ({ children }: { children: React.ReactNode }) => (
    <p>{children}</p>
  ),
}))

vi.mock('@s4wave/web/ui/tabs.js', () => ({
  Tabs: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TabsList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  TabsTrigger: ({ children }: { children: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
  TabsContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

const mockSession = {
  spacewave: {
    enrollSpaceMember: vi.fn(),
    createSpaceTargetedInvitationByUsername: vi.fn(),
  },
  createSpaceInvite: vi.fn(),
  resourceRef: { resourceId: 1, released: false },
  id: 1,
  client: {},
  service: {},
} as unknown as Session

function renderDialog({ isCloudProvider = true } = {}) {
  return render(
    <SessionContext.Provider
      resource={{
        value: mockSession,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    >
      <SpaceContainerContext.Provider
        spaceId="space-1"
        spaceState={{ ready: true }}
        spaceSharingState={{}}
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
        <AddUserDialog
          open={true}
          onOpenChange={vi.fn()}
          spaceName="Launch Space"
          spaceId="space-1"
          isCloudProvider={isCloudProvider}
          orgId="org-1"
          orgMembers={[
            {
              id: 'm1',
              entityId: 'alice',
              subjectId: 'acct-alice',
              roleId: 'org:owner',
            },
            {
              id: 'm2',
              entityId: 'casey',
              subjectId: 'acct-casey',
              roleId: 'org:member',
            },
          ]}
        />
      </SpaceContainerContext.Provider>
    </SessionContext.Provider>,
  )
}

describe('AddUserDialog', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders human-readable org member labels and launch-scope guidance', () => {
    renderDialog()

    expect(
      screen.getByText(
        'Add an existing org member, send a username invite, or create a shareable code or link.',
      ),
    ).toBeDefined()
    expect(
      screen.getByText(
        'Choose someone already in this organization. Use Username, Code, or Link to share with anyone else.',
      ),
    ).toBeDefined()
    expect(screen.getByText('alice')).toBeDefined()
    expect(screen.getByText('acct-alice')).toBeDefined()
    expect(screen.getByPlaceholderText('Search org members...')).toBeDefined()
  })

  it('filters org members by attested label', () => {
    renderDialog()

    fireEvent.change(screen.getByPlaceholderText('Search org members...'), {
      target: { value: 'casey' },
    })

    expect(screen.getByText('casey')).toBeDefined()
    expect(screen.queryByText('alice')).toBeNull()
  })

  it('offers exact username invite without search discovery', () => {
    renderDialog()

    expect(screen.getByText('Org Members')).toBeDefined()
    expect(screen.getByText('Code')).toBeDefined()
    expect(screen.getByText('Username')).toBeDefined()
    expect(screen.getByText('Link')).toBeDefined()
    expect(
      screen
        .getByText('Username')
        .compareDocumentPosition(screen.getByText('Code')) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    expect(screen.queryByText('Search all users')).toBeNull()
  })

  it('creates a targeted username invite', () => {
    renderDialog()

    fireEvent.change(screen.getByPlaceholderText('alice'), {
      target: { value: 'casey' },
    })
    fireEvent.click(screen.getByText('Send Invite'))

    expect(
      mockSession.spacewave.createSpaceTargetedInvitationByUsername,
    ).toHaveBeenCalledWith('casey', 'space-1', 'writer')
  })

  it('hides username invites for local provider sessions', () => {
    renderDialog({ isCloudProvider: false })

    expect(screen.getByText('Org Members')).toBeDefined()
    expect(screen.getByText('Code')).toBeDefined()
    expect(screen.getByText('Link')).toBeDefined()
    expect(screen.queryByText('Username')).toBeNull()
    expect(screen.queryByText('Spacewave username')).toBeNull()
    expect(screen.queryByText('Send Invite')).toBeNull()
    expect(
      screen.getByText(
        'Add an existing org member, or create a shareable code or link.',
      ),
    ).toBeDefined()
    expect(
      screen.getByText(
        'Choose someone already in this organization. Use Code or Link to share with anyone else.',
      ),
    ).toBeDefined()
  })
})
