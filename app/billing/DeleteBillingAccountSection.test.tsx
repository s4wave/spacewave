import type { ButtonHTMLAttributes, ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DeleteBillingAccountSection } from './DeleteBillingAccountSection.js'

const mockDeleteBillingAccount = vi.hoisted(() => vi.fn())
const mockOnDeleted = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          deleteBillingAccount: mockDeleteBillingAccount,
        },
      },
    }),
  },
}))

vi.mock('@s4wave/web/ui/DashboardButton.js', () => ({
  DashboardButton: (props: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props} />
  ),
}))

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({ open, children }: { open: boolean; children?: ReactNode }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogDescription: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogFooter: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...parts: Array<string | false | null | undefined>) =>
    parts.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  BillingStatus: {
    BillingStatus_NONE: 1,
    BillingStatus_ACTIVE: 2,
    BillingStatus_PAST_DUE: 3,
    BillingStatus_PAST_DUE_READONLY: 4,
    BillingStatus_CANCELED: 5,
  },
}))

describe('DeleteBillingAccountSection', () => {
  beforeEach(() => {
    mockDeleteBillingAccount.mockReset()
    mockOnDeleted.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('disables delete until the billing account is canceled', () => {
    render(
      <DeleteBillingAccountSection
        billingAccountId="ba_test"
        displayName="Personal"
        status={2}
        assigneeCount={0}
        onDeleted={mockOnDeleted}
      />,
    )

    expect(
      screen.getByRole('button', { name: 'Delete billing account' }),
    ).toHaveProperty('disabled', true)
    expect(
      screen.getByText(
        'Only canceled billing accounts or billing accounts with no subscription can be deleted.',
      ),
    ).toBeTruthy()
  })

  it('allows delete when the billing account has no subscription', () => {
    render(
      <DeleteBillingAccountSection
        billingAccountId="ba_test"
        displayName="Personal"
        status={1}
        assigneeCount={0}
        onDeleted={mockOnDeleted}
      />,
    )

    expect(
      screen.getByRole('button', { name: 'Delete billing account' }),
    ).toHaveProperty('disabled', false)
  })

  it('shows the past-due tooltip reason when balance remains', () => {
    render(
      <DeleteBillingAccountSection
        billingAccountId="ba_test"
        displayName="Personal"
        status={3}
        assigneeCount={0}
        onDeleted={mockOnDeleted}
      />,
    )

    expect(
      screen.getByText(
        'This billing account still has a past-due balance. Resolve the balance before deleting it.',
      ),
    ).toBeTruthy()
  })

  it('shows the detach tooltip reason while principals still point at the BA', () => {
    render(
      <DeleteBillingAccountSection
        billingAccountId="ba_test"
        displayName="Personal"
        status={5}
        assigneeCount={2}
        onDeleted={mockOnDeleted}
      />,
    )

    expect(
      screen.getByText(
        'Detach this billing account from every personal account and organization before deleting it.',
      ),
    ).toBeTruthy()
  })

  it('deletes the billing account after confirmation', async () => {
    mockDeleteBillingAccount.mockResolvedValue(undefined)

    render(
      <DeleteBillingAccountSection
        billingAccountId="ba_test"
        displayName="Personal"
        status={5}
        assigneeCount={0}
        onDeleted={mockOnDeleted}
      />,
    )

    fireEvent.click(
      screen.getAllByRole('button', { name: 'Delete billing account' })[0]!,
    )
    fireEvent.change(screen.getByPlaceholderText('DELETE'), {
      target: { value: 'DELETE' },
    })
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Delete billing account' })[1]!,
    )

    await waitFor(() =>
      expect(mockDeleteBillingAccount).toHaveBeenCalledWith('ba_test'),
    )
    expect(mockOnDeleted).toHaveBeenCalled()
  })
})
