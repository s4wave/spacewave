import React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { OrganizationDashboard } from './OrganizationDashboard.js'

const mockNavigateSession = vi.fn()
const mockUseOrgContainerState = vi.hoisted(() => vi.fn())
const mockSetOpenMenu = vi.hoisted(() => vi.fn())
const mockSetOpenSection = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionNavigate: () => mockNavigateSession,
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mockSetOpenMenu,
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['org-details'],
  useStateAtom: () => ['members', mockSetOpenSection] as const,
}))

vi.mock('./OrgContainer.js', () => ({
  useOrgContainerState: mockUseOrgContainerState,
}))

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...values: Array<string | false | null | undefined>) =>
    values.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
}))

vi.mock('@s4wave/web/ui/command.js', () => ({
  Command: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CommandEmpty: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CommandGroup: ({
    heading,
    children,
  }: {
    heading?: React.ReactNode
    children: React.ReactNode
  }) => (
    <section>
      {heading}
      {children}
    </section>
  ),
  CommandInput: ({ placeholder }: { placeholder?: string }) => (
    <input placeholder={placeholder} />
  ),
  CommandItem: ({
    children,
    onSelect,
  }: {
    children: React.ReactNode
    onSelect?: () => void
  }) => <button onClick={() => onSelect?.()}>{children}</button>,
  CommandList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

describe('OrganizationDashboard', () => {
  beforeEach(() => {
    cleanup()
    mockNavigateSession.mockReset()
    mockSetOpenMenu.mockReset()
    mockSetOpenSection.mockReset()
    mockUseOrgContainerState.mockReset()
    mockUseOrgContainerState.mockReturnValue({
      orgId: 'org-1',
      orgName: 'Studio',
      degraded: false,
      spaces: [{ id: 'space-1', displayName: 'Roadmap', objectType: 'space' }],
      orgState: {
        organization: {
          displayName: 'Studio',
        },
        spaces: [{ id: 'space-1', displayName: 'Roadmap' }],
      },
    })
  })

  it('opens org quickstarts through an explicit org route', () => {
    render(<OrganizationDashboard />)

    fireEvent.click(screen.getByText('Create a Drive'))

    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: 'org/org-1/new/drive',
    })
  })

  it('opens the new-space action through the org route', () => {
    render(<OrganizationDashboard />)

    fireEvent.click(screen.getByText('+ New Space'))

    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: 'org/org-1/new/drive',
    })
  })

  it('opens existing spaces within the session route root', () => {
    render(<OrganizationDashboard />)

    fireEvent.click(screen.getByText('Roadmap'))

    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: 'org/org-1/so/space-1',
    })
  })

  it('renders an in-place degraded org shell when the org root is unavailable', () => {
    mockUseOrgContainerState.mockReturnValue({
      orgId: 'org-1',
      orgName: 'Studio',
      degraded: true,
      spaces: [{ id: 'space-1', displayName: 'Roadmap', objectType: 'space' }],
      orgState: null,
    })

    render(<OrganizationDashboard />)

    expect(screen.getByText('Organization root unavailable.')).toBeDefined()
    expect(
      screen.getByText(
        'Spaces stay available from the session inventory while you review remediation options.',
      ),
    ).toBeDefined()
    expect(screen.getByText('Roadmap')).toBeDefined()
  })

  it('keeps degraded org spaces usable through the canonical org route', () => {
    mockUseOrgContainerState.mockReturnValue({
      orgId: 'org-1',
      orgName: 'Studio',
      degraded: true,
      spaces: [{ id: 'space-1', displayName: 'Roadmap', objectType: 'space' }],
      orgState: null,
    })

    render(<OrganizationDashboard />)

    fireEvent.click(screen.getByText('Roadmap'))

    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: 'org/org-1/so/space-1',
    })
  })

  it('routes the degraded issue banner into the organization recovery overlay section', () => {
    mockUseOrgContainerState.mockReturnValue({
      orgId: 'org-1',
      orgName: 'Studio',
      degraded: true,
      spaces: [{ id: 'space-1', displayName: 'Roadmap', objectType: 'space' }],
      orgState: null,
    })

    render(<OrganizationDashboard />)

    fireEvent.click(screen.getByText('Fix issue'))

    expect(mockSetOpenSection).toHaveBeenCalledWith('recovery')
    expect(mockSetOpenMenu).toHaveBeenCalledWith('organization')
  })
})
