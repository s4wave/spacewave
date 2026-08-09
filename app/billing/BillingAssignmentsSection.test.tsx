import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BillingAssignmentsSection } from './BillingAssignmentsSection.js'

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
vi.mock('@s4wave/web/ui/DropdownMenu.js', () => ({
  DropdownMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: ReactNode }) => (
    <>{children}</>
  ),
  DropdownMenuItem: ({
    children,
    onSelect,
  }: {
    children: ReactNode
    onSelect: () => void
  }) => <button onClick={onSelect}>{children}</button>,
  DropdownMenuLabel: ({ children }: { children: ReactNode }) => <>{children}</>,
  DropdownMenuSeparator: () => null,
  DropdownMenuTrigger: ({ children }: { children: ReactNode }) => (
    <>{children}</>
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

describe('BillingAssignmentsSection', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('routes detail assignment and dialog actions through the shared owner', () => {
    render(
      <BillingAssignmentsSection
        baId="ba_1"
        managedBillingAccount={{
          id: 'ba_1',
          assignees: [
            { ownerType: 'account', ownerId: 'acct_1', displayName: '' },
          ],
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /Example Org/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Detach Personal' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm detach' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel detach' }))

    expect(mocks.assign).toHaveBeenCalledWith('ba_1', target)
    expect(mocks.requestDetach).toHaveBeenCalledWith({
      ownerType: 'account',
      ownerId: 'acct_1',
      label: 'Personal',
    })
    expect(mocks.confirmDetach).toHaveBeenCalledOnce()
    expect(mocks.cancelDetach).toHaveBeenCalledOnce()
  })
})
