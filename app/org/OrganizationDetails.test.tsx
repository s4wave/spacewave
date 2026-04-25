import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, screen } from '@testing-library/react'
import { OrganizationDetails } from './OrganizationDetails.js'
import type { WatchOrganizationStateResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: () => null,
}))
vi.mock('@s4wave/web/ui/CollapsibleSection.js', () => ({
  CollapsibleSection: ({
    title,
    children,
  }: {
    title: string
    children: React.ReactNode
  }) => (
    <section data-testid={`section-${title.toLowerCase()}`}>
      <h2>{title}</h2>
      {children}
    </section>
  ),
}))
vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['org-details'],
  useStateAtom: (_ns: unknown, _key: string, init: unknown) =>
    [init, vi.fn()] as const,
}))
vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({ value: null }),
  },
  useSessionNavigate: () => vi.fn(),
}))
vi.mock('./OrgBillingSection.js', () => ({
  OrgBillingSection: ({ billingAccountId }: { billingAccountId?: string }) => (
    <section data-testid="section-billing">
      <h2>Billing</h2>
      <span>{billingAccountId ?? 'no-ba'}</span>
    </section>
  ),
}))
vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => vi.fn(),
}))
vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { info: vi.fn(), error: vi.fn() },
}))

function makeOrgState(
  role: string,
  overrides?: Partial<WatchOrganizationStateResponse>,
): WatchOrganizationStateResponse {
  return {
    organization: {
      id: 'org-1',
      displayName: 'Test Org',
      role,
      billingAccountId: 'billing-1',
    },
    members: [
      {
        id: 'm1',
        entityId: 'owner-user',
        subjectId: 'acct-owner',
        roleId: 'org:owner',
        createdAt: 0n,
      },
      {
        id: 'm2',
        entityId: 'member-user',
        subjectId: 'acct-member',
        roleId: 'org:member',
        createdAt: 0n,
      },
    ],
    spaces: [],
    invites: [],
    ...overrides,
  }
}

describe('OrganizationDetails', () => {
  beforeEach(() => {
    cleanup()
  })

  describe('Owner view', () => {
    it('shows Members, Invites, Settings, Billing, Identifiers sections', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner')}
          isOwner={true}
        />,
      )
      expect(screen.getByTestId('section-members')).toBeDefined()
      expect(screen.getByTestId('section-invites')).toBeDefined()
      expect(screen.getByTestId('section-settings')).toBeDefined()
      expect(screen.getByTestId('section-billing')).toBeDefined()
      expect(screen.getByTestId('section-identifiers')).toBeDefined()
    })

    it('shows org name and Owner role', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner')}
          isOwner={true}
        />,
      )
      expect(screen.getAllByText('Test Org').length).toBeGreaterThanOrEqual(1)
      // Role label appears in both the header and member list badges
      expect(screen.getAllByText('Owner').length).toBeGreaterThanOrEqual(1)
    })

    it('renders member labels with attested usernames first and raw ids second', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner')}
          isOwner={true}
        />,
      )

      expect(screen.getByText('owner-user')).toBeDefined()
      expect(screen.getByText('acct-owner')).toBeDefined()
      expect(screen.getByText('member-user')).toBeDefined()
      expect(screen.getByText('acct-member')).toBeDefined()
      expect(
        screen.getByText(
          'Members are shown by username first. Their account ID stays underneath for review or copy.',
        ),
      ).toBeDefined()
    })

    it('keeps the remove affordance clear for non-owner members', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner')}
          isOwner={true}
        />,
      )

      expect(screen.getByText('Remove')).toBeDefined()
    })

    it('does not show Leave button for owners', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner')}
          isOwner={true}
        />,
      )
      expect(screen.queryByText('Leave')).toBeNull()
    })

    it('shows Billing section even when no billing account is assigned', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner', {
            organization: {
              id: 'org-1',
              displayName: 'Test Org',
              role: 'org:owner',
            },
          })}
          isOwner={true}
        />,
      )
      expect(screen.getByTestId('section-billing')).toBeDefined()
      expect(screen.getByText('no-ba')).toBeDefined()
    })

    it('shows the Recovery section when the org dashboard is degraded', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:owner')}
          degraded={true}
          isOwner={true}
        />,
      )

      expect(screen.getByTestId('section-recovery')).toBeDefined()
      expect(
        screen.getByText('Organization root shared object unavailable'),
      ).toBeDefined()
    })

    it('renders degraded recovery controls even when org state is unavailable', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgName="Fallback Org"
          orgState={null}
          degraded={true}
          isOwner={true}
        />,
      )

      expect(screen.queryByText('Loading organization')).toBeNull()
      expect(screen.getByTestId('section-recovery')).toBeDefined()
      expect(screen.getByText('Fallback Org')).toBeDefined()
      expect(screen.getByRole('button', { name: 'Repair' })).toBeDefined()
      expect(screen.getByRole('button', { name: 'Reinitialize' })).toBeDefined()
      expect(screen.getByText('Shared object ID: org-1')).toBeDefined()
    })
  })

  describe('Non-owner view', () => {
    it('shows Members and Identifiers sections', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:member')}
          isOwner={false}
        />,
      )
      expect(screen.getByTestId('section-members')).toBeDefined()
      expect(screen.getByTestId('section-identifiers')).toBeDefined()
    })

    it('hides Invites, Settings, Billing sections for non-owners', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:member')}
          isOwner={false}
        />,
      )
      expect(screen.queryByTestId('section-invites')).toBeNull()
      expect(screen.queryByTestId('section-settings')).toBeNull()
      expect(screen.queryByTestId('section-billing')).toBeNull()
    })

    it('shows Leave button for non-owners', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:member')}
          isOwner={false}
        />,
      )
      expect(screen.getByText('Leave')).toBeDefined()
    })

    it('disables degraded recovery actions for non-owners without org metadata', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgName="Fallback Org"
          orgState={null}
          degraded={true}
          isOwner={false}
        />,
      )

      expect(
        screen.getByRole('button', { name: 'Repair' }).hasAttribute('disabled'),
      ).toBe(true)
      expect(
        screen
          .getByRole('button', { name: 'Reinitialize' })
          .hasAttribute('disabled'),
      ).toBe(true)
    })

    it('shows Member role label', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:member')}
          isOwner={false}
        />,
      )
      // Role label appears in both the header and member list badges
      expect(screen.getAllByText('Member').length).toBeGreaterThanOrEqual(1)
    })

    it('shows an explicit empty member state', () => {
      render(
        <OrganizationDetails
          orgId="org-1"
          orgState={makeOrgState('org:member', { members: [] })}
          isOwner={false}
        />,
      )

      expect(screen.getByTestId('org-members-empty').textContent).toBe(
        'No members yet',
      )
    })
  })

  describe('Loading state', () => {
    it('shows loading message when orgState is null', () => {
      render(
        <OrganizationDetails orgId="org-1" orgState={null} isOwner={false} />,
      )
      expect(screen.getByText('Loading organization')).toBeDefined()
    })
  })
})
