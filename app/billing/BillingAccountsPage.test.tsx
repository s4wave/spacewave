import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BillingAccountsPage } from './BillingAccountsPage.js'

const mocks = vi.hoisted(() => ({
  assign: vi.fn(),
  cancelDetach: vi.fn(),
  confirmDetach: vi.fn(),
  requestDetach: vi.fn(),
}))

const target = {
  ownerType: 'organization' as const,
  ownerId: 'org_1',
  label: 'Example Org',
}

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => vi.fn(),
}))
vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionNavigate: () => vi.fn(),
}))
vi.mock('../provider/spacewave/useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => null,
}))
vi.mock('../provider/spacewave/checkout-url.js', () => ({
  getCheckoutResultBaseUrl: () => '',
}))
vi.mock('./useManagedBillingAccounts.js', () => ({
  useManagedBillingAccounts: () => ({
    data: { accounts: [{ id: 'ba_1' }] },
    loading: false,
    error: null,
    store: {},
  }),
}))
vi.mock('./useBillingAssignments.js', () => ({
  useBillingAssignments: () => ({
    actionsDisabled: false,
    assign: mocks.assign,
    assignError: null,
    assigningBillingAccountId: null,
    assignTargets: [target],
    callerAccountId: 'acct_1',
    cancelDetach: mocks.cancelDetach,
    confirmDetach: mocks.confirmDetach,
    detachError: null,
    detachTarget: target,
    detaching: false,
    requestDetach: mocks.requestDetach,
  }),
}))
vi.mock('./BillingAccountsList.js', () => ({
  BillingAccountsList: ({
    onAssign,
    onDetach,
  }: {
    onAssign: (
      billingAccountId: string,
      assignmentTarget: typeof target,
    ) => void
    onDetach: (assignmentTarget: typeof target) => void
  }) => (
    <>
      <button onClick={() => onAssign('ba_1', target)}>Assign list</button>
      <button onClick={() => onDetach(target)}>Detach list</button>
    </>
  ),
}))
vi.mock('./DetachAssignmentDialog.js', () => ({
  DetachAssignmentDialog: ({
    onCancel,
    onConfirm,
  }: {
    onCancel: () => void
    onConfirm: () => void
  }) => (
    <>
      <button onClick={onConfirm}>Confirm detach</button>
      <button onClick={onCancel}>Cancel detach</button>
    </>
  ),
}))

describe('BillingAccountsPage assignments', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('routes list assignment and dialog actions through the shared owner', () => {
    render(<BillingAccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Assign list' }))
    fireEvent.click(screen.getByRole('button', { name: 'Detach list' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm detach' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel detach' }))

    expect(mocks.assign).toHaveBeenCalledWith('ba_1', target)
    expect(mocks.requestDetach).toHaveBeenCalledWith(target)
    expect(mocks.confirmDetach).toHaveBeenCalledOnce()
    expect(mocks.cancelDetach).toHaveBeenCalledOnce()
  })
})
