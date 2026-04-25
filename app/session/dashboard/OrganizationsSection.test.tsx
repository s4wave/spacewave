import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { OrganizationsSection } from './OrganizationsSection.js'

const mockNavigateSession = vi.fn()
const mockCreateOrganization = vi.fn()
const mockSessionResource = {
  value: {
    spacewave: {
      createOrganization: mockCreateOrganization,
    },
  },
  loading: false,
  error: null,
  retry: vi.fn(),
}

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value: unknown }) => resource.value,
}))
vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => mockSessionResource,
  },
  useSessionNavigate: () => mockNavigateSession,
}))
vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: {
    useContextSafe: () => ({
      loading: false,
      organizations: [{ id: 'org-1', displayName: 'Studio' }],
    }),
  },
}))
vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({
    providerId: 'spacewave',
  }),
}))
vi.mock('@s4wave/web/ui/CollapsibleSection.js', () => ({
  CollapsibleSection: ({
    title,
    children,
  }: {
    title: string
    children: React.ReactNode
  }) => (
    <section>
      <h2>{title}</h2>
      {children}
    </section>
  ),
}))
vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['session-settings'],
  useStateAtom: (_ns: unknown, _key: string, init: boolean) =>
    [init, vi.fn()] as const,
}))

describe('OrganizationsSection', () => {
  beforeEach(() => {
    cleanup()
    mockNavigateSession.mockClear()
    mockCreateOrganization.mockReset()
  })

  it('opens organizations within the active session', () => {
    render(<OrganizationsSection />)

    fireEvent.click(screen.getByText('Studio'))

    expect(mockNavigateSession).toHaveBeenCalledWith({ path: 'org/org-1/' })
  })

  it('locks the create form while org creation is pending', async () => {
    let resolveCreate: (() => void) | undefined
    mockCreateOrganization.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveCreate = resolve
      }),
    )

    render(<OrganizationsSection />)

    fireEvent.click(screen.getByText('Create Organization'))
    fireEvent.change(screen.getByPlaceholderText('Organization name'), {
      target: { value: 'New Org' },
    })
    fireEvent.click(screen.getByText('Create'))

    const input = screen.getByPlaceholderText('Organization name')
    const createButton = screen.getByText('Creating...')
    const cancelButton = screen.getByText('Cancel')

    expect(mockCreateOrganization).toHaveBeenCalledWith('New Org')
    expect(input).toHaveProperty('disabled', true)
    expect(createButton).toHaveProperty('disabled', true)
    expect(cancelButton).toHaveProperty('disabled', true)
    expect(screen.getByText('Creating organization...')).toBeDefined()

    resolveCreate?.()

    await waitFor(() => {
      expect(screen.queryByPlaceholderText('Organization name')).toBeNull()
    })
  })
})
