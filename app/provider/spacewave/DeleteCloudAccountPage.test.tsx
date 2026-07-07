import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'

import { DeleteCloudAccountPage } from './DeleteCloudAccountPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockToastSuccess = vi.hoisted(() => vi.fn())
const mockToastError = vi.hoisted(() => vi.fn())
const mockUseResourceValue = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())
const mockUseMountAccount = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn())
const mockUseSessionIndex = vi.hoisted(() => vi.fn(() => 0))
const mockUseOnboardingContext = vi.hoisted(() => vi.fn())
const mockSessionResource = vi.hoisted(() => ({
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => mockSessionResource,
  },
  useSessionIndex: mockUseSessionIndex,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContext: mockUseOnboardingContext,
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: mockUseSessionInfo,
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: mockUseMountAccount,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
  },
}))

vi.mock('@s4wave/app/session/SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/app/session/LogoutConfirmDialog.js', () => ({
  LogoutConfirmDialog: () => <div data-testid="logout-confirm-dialog" />,
}))

vi.mock('@s4wave/web/ui/loading/Spinner.js', () => ({
  Spinner: () => <span data-testid="spinner" />,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.pb.js', () => ({
  AccountLifecycleState: {
    AccountLifecycleState_PENDING_DELETE_READONLY: 'pending_delete_readonly',
  },
}))

async function flushAsync() {
  await act(async () => {})
  await act(async () => {})
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('DeleteCloudAccountPage', () => {
  const mockRequestDeleteNowEmail = vi.fn()
  const mockConfirmDeleteNowCode = vi.fn()
  const mockSession = {
    spacewave: {
      requestDeleteNowEmail: mockRequestDeleteNowEmail,
      confirmDeleteNowCode: mockConfirmDeleteNowCode,
      undoDeleteNow: vi.fn(),
    },
  }

  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockToastSuccess.mockReset()
    mockToastError.mockReset()
    mockUseResourceValue.mockReset()
    mockUseSessionInfo.mockReset()
    mockUseMountAccount.mockReset()
    mockUseRootResource.mockReset()
    mockUseSessionIndex.mockReset()
    mockUseOnboardingContext.mockReset()
    mockRequestDeleteNowEmail.mockReset()
    mockConfirmDeleteNowCode.mockReset()

    mockUseResourceValue.mockReturnValue(mockSession)
    mockUseOnboardingContext.mockReturnValue({
      onboarding: {
        lifecycleState: 'active',
        deleteAt: null,
      },
    })
    mockUseSessionInfo.mockReturnValue({
      providerId: 'spacewave',
      accountId: 'acct_1',
      peerId: 'peer_1',
    })
    mockUseMountAccount.mockReturnValue({
      value: {
        selfRevokeSession: vi.fn(),
      },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    mockUseRootResource.mockReturnValue({
      value: {
        deleteSession: vi.fn(),
      },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    mockUseSessionIndex.mockReturnValue(0)
  })

  afterEach(() => {
    cleanup()
  })

  it('sends a delete confirmation email and leaves confirmation state after the code succeeds', async () => {
    const requestEmail = deferred<{ email: string; retryAfter: number }>()
    const confirmDelete = deferred<{
      invoiceTotal: bigint
      invoiceAmountDue: bigint
      invoiceCurrency: string
      invoiceStatus: string
      chargeAttempted: boolean
    }>()
    mockRequestDeleteNowEmail.mockReturnValue(requestEmail.promise)
    mockConfirmDeleteNowCode.mockReturnValue(confirmDelete.promise)

    render(<DeleteCloudAccountPage />)

    const sendButton = screen.getByRole('button', {
      name: 'Send confirmation email',
    })
    expect(sendButton.hasAttribute('disabled')).toBe(false)
    expect(
      screen.getByRole('button', { name: 'Confirm delete account' }),
    ).toHaveProperty('disabled', true)

    fireEvent.click(sendButton)

    expect(mockRequestDeleteNowEmail).toHaveBeenCalledTimes(1)
    expect(
      screen.getByRole('button', { name: 'Sending...' }).hasAttribute(
        'disabled',
      ),
    ).toBe(true)

    await act(async () => {
      requestEmail.resolve({
        email: 'casey@example.com',
        retryAfter: 0,
      })
      await requestEmail.promise
    })
    await flushAsync()

    expect(mockToastSuccess).toHaveBeenCalledWith('Confirmation email sent')
    expect(screen.getByText('casey@example.com')).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Resend confirmation email' }),
    ).toBeTruthy()

    const deleteCodeInput = screen.getByLabelText('6-digit delete code')
    fireEvent.change(deleteCodeInput, { target: { value: '123456' } })

    const confirmButton = screen.getByRole('button', {
      name: 'Confirm delete account',
    })
    expect(confirmButton.hasAttribute('disabled')).toBe(false)

    fireEvent.click(confirmButton)

    expect(mockConfirmDeleteNowCode).toHaveBeenCalledTimes(1)
    expect(mockConfirmDeleteNowCode).toHaveBeenCalledWith('123456')
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Confirming...' }).hasAttribute(
        'disabled',
      ),
    ).toBe(true)

    await act(async () => {
      confirmDelete.resolve({
        invoiceTotal: 0n,
        invoiceAmountDue: 0n,
        invoiceCurrency: 'usd',
        invoiceStatus: 'paid',
        chargeAttempted: false,
      })
      await confirmDelete.promise
    })
    await flushAsync()

    expect(mockToastSuccess).toHaveBeenLastCalledWith(
      'Deletion confirmed. The account is now read-only.',
    )
    expect(mockNavigate).toHaveBeenCalledWith({ path: '../', replace: true })
    expect(screen.queryByText('Confirming...')).toBeNull()
    expect(screen.queryByText(/Deleting/i)).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Confirm delete account' }),
    ).toBeTruthy()
  })
})
