import React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { OrgSpaceList } from './OrgSpaceList.js'

const mockNavigateSession = vi.fn()

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionNavigate: () => mockNavigateSession,
}))

describe('OrgSpaceList', () => {
  beforeEach(() => {
    cleanup()
    mockNavigateSession.mockReset()
  })

  it('opens organization-owned spaces through the session root', () => {
    render(
      <OrgSpaceList
        orgId="org-1"
        spaces={[
          {
            id: 'space-1',
            displayName: 'Roadmap',
            objectType: 'space',
          },
        ]}
      />,
    )

    fireEvent.click(screen.getByText('Roadmap'))

    expect(mockNavigateSession).toHaveBeenCalledWith({
      path: 'org/org-1/so/space-1',
    })
  })

  it('shows the empty-state message when the organization has no spaces', () => {
    render(<OrgSpaceList orgId="org-1" spaces={[]} />)

    expect(screen.getByText('No spaces yet')).toBeDefined()
  })
})
