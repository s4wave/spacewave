import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useBillingAssignments } from './useBillingAssignments.js'

const mocks = vi.hoisted(() => ({
  assign: vi.fn(),
  detach: vi.fn(),
  organizations: [
    { id: 'org_1', displayName: 'Example Org', role: 'org:owner' },
    { id: 'org_2', displayName: 'Member Org', role: 'org:member' },
  ],
}))

vi.mock('./useManagedBillingAccounts.js', () => ({
  useManagedBillingAccounts: () => ({
    session: {},
    store: { assign: mocks.assign, detach: mocks.detach },
  }),
}))
vi.mock('@s4wave/web/contexts/SpacewaveOrgListContext.js', () => ({
  SpacewaveOrgListContext: {
    useContext: () => ({ organizations: mocks.organizations }),
  },
}))
vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({ accountId: 'acct_1' }),
}))

function Consumer() {
  const assignments = useBillingAssignments()
  const organization = assignments.assignTargets.find(
    (target) => target.ownerType === 'organization',
  )
  return (
    <>
      <div>
        {assignments.assignTargets.map((target) => target.label).join(',')}
      </div>
      <div>{assignments.assignError}</div>
      <div>{assignments.detachError}</div>
      <button
        onClick={() =>
          organization && void assignments.assign('ba_1', organization)
        }
      >
        Assign
      </button>
      <button onClick={() => assignments.requestDetach(organization ?? null)}>
        Request detach
      </button>
      <button onClick={() => void assignments.confirmDetach()}>
        Confirm detach
      </button>
    </>
  )
}

describe('useBillingAssignments', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('builds personal and owned-organization targets and performs mutations', async () => {
    mocks.assign.mockResolvedValue({})
    mocks.detach.mockResolvedValue({})
    render(<Consumer />)

    expect(screen.getByText('Personal account,Example Org')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Assign' }))
    await waitFor(() => expect(mocks.assign).toHaveBeenCalledOnce())
    expect(mocks.assign).toHaveBeenCalledWith('ba_1', 'organization', 'org_1')

    fireEvent.click(screen.getByRole('button', { name: 'Request detach' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm detach' }))
    await waitFor(() => expect(mocks.detach).toHaveBeenCalledOnce())
    expect(mocks.detach).toHaveBeenCalledWith('organization', 'org_1')
  })

  it('publishes assignment and detach failures for either layout', async () => {
    mocks.assign.mockRejectedValueOnce(new Error('assign failed'))
    mocks.detach.mockRejectedValueOnce(new Error('detach failed'))
    render(<Consumer />)

    fireEvent.click(screen.getByRole('button', { name: 'Assign' }))
    await waitFor(() => expect(screen.getByText('assign failed')).toBeDefined())
    fireEvent.click(screen.getByRole('button', { name: 'Request detach' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm detach' }))
    await waitFor(() => expect(screen.getByText('detach failed')).toBeDefined())
  })
})
