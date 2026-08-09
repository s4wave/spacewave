import type { ButtonHTMLAttributes } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PlanControls } from './PlanControls.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockReactivateSubscription = vi.hoisted(() => vi.fn())
const mockSession = vi.hoisted(() => ({
  value: undefined as
    | {
        spacewave: { reactivateSubscription: typeof mockReactivateSubscription }
      }
    | undefined,
}))
const mockStartCheckout = vi.hoisted(() => vi.fn())
const mockManagedStore = vi.hoisted(() => ({
  reactivate: mockReactivateSubscription,
  refresh: vi.fn(),
}))
const mockBillingState = vi.hoisted(() => ({
  billingAccountId: 'ba_test',
}))
const mockPath = vi.hoisted(() => ({ value: '/u/1/billing/ba_test' }))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: mockSession.value,
    }),
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  usePath: () => mockPath.value,
}))

vi.mock('../provider/spacewave/useBillingAccountCheckout.js', () => ({
  useBillingAccountCheckout: () => ({
    continueCheckout: vi.fn(),
    error: null,
    polling: false,
    showRetry: false,
    startCheckout: mockStartCheckout,
  }),
}))

vi.mock('./useManagedBillingAccounts.js', () => ({
  useManagedBillingAccounts: () => ({ store: mockManagedStore }),
}))

vi.mock('./BillingStateProvider.js', () => ({
  useBillingStateContext: () => mockBillingState,
}))

vi.mock('@s4wave/web/ui/DashboardButton.js', () => ({
  DashboardButton: (props: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props} />
  ),
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  BillingStatus: {
    BillingStatus_ACTIVE: 2,
    BillingStatus_TRIALING: 3,
    BillingStatus_CANCELED: 5,
  },
}))

describe('PlanControls', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockSession.value = {
      spacewave: { reactivateSubscription: mockReactivateSubscription },
    }
    mockReactivateSubscription.mockReset()
    mockStartCheckout.mockReset()
    mockBillingState.billingAccountId = 'ba_test'
    window.location.hash = '#/u/1/billing/ba_test'
    mockPath.value = '/u/1/billing/ba_test'
  })

  afterEach(() => {
    cleanup()
  })

  it('starts checkout for the same billing account when reactivation needs checkout', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: true })

    render(<PlanControls status={5} showSelfService={true} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith(
        'ba_test',
        expect.any(AbortSignal),
      ),
    )
    expect(mockStartCheckout).toHaveBeenCalledWith('ba_test')
    await waitFor(() =>
      expect(
        screen.getByRole<HTMLButtonElement>('button', {
          name: 'Reactivate subscription',
        }).disabled,
      ).toBe(false),
    )
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('aborts reactivation on unmount without starting stale checkout', async () => {
    let resolveReactivate!: (value: { needsCheckout: boolean }) => void
    mockReactivateSubscription.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveReactivate = resolve
        }),
    )

    const { unmount } = render(
      <PlanControls status={5} showSelfService={true} />,
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() => expect(mockReactivateSubscription).toHaveBeenCalled())
    const signal = mockReactivateSubscription.mock.calls[0]?.[1] as AbortSignal
    expect(signal.aborted).toBe(false)

    unmount()
    expect(signal.aborted).toBe(true)
    resolveReactivate({ needsCheckout: true })
    await new Promise<void>((resolve) => queueMicrotask(resolve))

    expect(mockStartCheckout).not.toHaveBeenCalled()
  })

  it('aborts the old account generation without checkout or stale action state', async () => {
    let resolveReactivate!: (value: { needsCheckout: boolean }) => void
    mockReactivateSubscription.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveReactivate = resolve
        }),
    )

    const { rerender } = render(
      <PlanControls status={5} showSelfService={true} />,
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() => expect(mockReactivateSubscription).toHaveBeenCalled())
    const signal = mockReactivateSubscription.mock.calls[0]?.[1] as AbortSignal
    mockBillingState.billingAccountId = 'ba_next'
    rerender(<PlanControls status={5} showSelfService={true} />)

    expect(signal.aborted).toBe(true)
    expect(
      screen.getByRole<HTMLButtonElement>('button', {
        name: 'Reactivate subscription',
      }).disabled,
    ).toBe(false)
    resolveReactivate({ needsCheckout: true })
    await new Promise<void>((resolve) => queueMicrotask(resolve))

    expect(mockStartCheckout).not.toHaveBeenCalled()
    expect(screen.queryByText('Reactivate failed')).toBeNull()
  })

  it('aborts the old route generation before checkout', async () => {
    let resolveReactivate!: (value: { needsCheckout: boolean }) => void
    mockReactivateSubscription.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveReactivate = resolve
        }),
    )

    const { rerender } = render(
      <PlanControls status={5} showSelfService={true} />,
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Reactivate subscription' }),
    )

    await waitFor(() => expect(mockReactivateSubscription).toHaveBeenCalled())
    const signal = mockReactivateSubscription.mock.calls[0]?.[1] as AbortSignal
    mockPath.value = '/u/1/billing/ba_other'
    rerender(<PlanControls status={5} showSelfService={true} />)

    expect(signal.aborted).toBe(true)
    resolveReactivate({ needsCheckout: true })
    await new Promise<void>((resolve) => queueMicrotask(resolve))
    expect(mockStartCheckout).not.toHaveBeenCalled()
  })

  it('auto-starts reactivation once when the billing page carries a reactivate intent', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: true })
    mockPath.value = '/u/1/billing/ba_test?reactivate=1'

    render(<PlanControls status={5} showSelfService={true} />)

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith(
        'ba_test',
        expect.any(AbortSignal),
      ),
    )
    expect(mockStartCheckout).toHaveBeenCalledWith('ba_test')
    expect(mockNavigate).toHaveBeenCalledWith({
      path: '/u/1/billing/ba_test',
      replace: true,
    })
  })

  it('does not duplicate a route-triggered reactivation when the button is clicked', async () => {
    mockReactivateSubscription.mockImplementation(() => new Promise(() => {}))
    mockPath.value = '/u/1/billing/ba_test?reactivate=1'

    render(<PlanControls status={5} showSelfService={true} />)
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(mockReactivateSubscription).toHaveBeenCalled())
    await new Promise<void>((resolve) => queueMicrotask(resolve))
    expect(mockReactivateSubscription).toHaveBeenCalledOnce()
  })

  it('waits for the session before consuming a reactivation intent', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: false })
    mockPath.value = '/u/1/billing/ba_test?reactivate=1'
    mockSession.value = undefined

    const { rerender } = render(
      <PlanControls status={5} showSelfService={true} />,
    )
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(mockReactivateSubscription).not.toHaveBeenCalled()

    mockSession.value = {
      spacewave: { reactivateSubscription: mockReactivateSubscription },
    }
    rerender(<PlanControls status={5} showSelfService={true} />)

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith(
        'ba_test',
        expect.any(AbortSignal),
      ),
    )
    expect(mockNavigate).toHaveBeenCalledWith({
      path: '/u/1/billing/ba_test',
      replace: true,
    })
  })

  it('uses the panel route query for auto-reactivation in split mode', async () => {
    mockReactivateSubscription.mockResolvedValue({ needsCheckout: true })
    window.location.hash = '#/g/encoded-shell-layout'
    mockPath.value = '/u/1/billing/ba_test?reactivate=1'

    render(<PlanControls status={5} showSelfService={true} />)

    await waitFor(() =>
      expect(mockReactivateSubscription).toHaveBeenCalledWith(
        'ba_test',
        expect.any(AbortSignal),
      ),
    )
    expect(mockStartCheckout).toHaveBeenCalledWith('ba_test')
    expect(mockNavigate).toHaveBeenCalledWith({
      path: '/u/1/billing/ba_test',
      replace: true,
    })
  })
})
