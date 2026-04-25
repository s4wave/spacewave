import type { ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SSOLinkDialog } from './SSOLinkDialog.js'

const mockStartDesktopSSOLink = vi.hoisted(() => vi.fn())
const mockToastSuccess = vi.hoisted(() => vi.fn())
const openSpy = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr', () => ({
  isDesktop: true,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: <T,>(res: { value: T | null }) => res.value,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: { startDesktopSSOLink: mockStartDesktopSSOLink },
      },
      loading: false,
      error: null,
      retry: () => {},
    }),
  },
}))

vi.mock('@s4wave/app/provider/spacewave/useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => ({
    ssoBaseUrl: 'https://account.test/auth/sso',
    accountBaseUrl: 'https://account.test',
  }),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: (...args: unknown[]) => {
      mockToastSuccess(...args)
    },
  },
}))

const confirmCredential = { type: 'password', password: 'secret-password' }

vi.mock('./AuthConfirmDialog.js', () => ({
  buildEntityCredential: () => ({
    credential: { case: 'password', value: 'secret-password' },
  }),
  AuthConfirmDialog: ({
    open,
    onOpenChange,
    onConfirm,
    title,
  }: {
    open: boolean
    onOpenChange: (open: boolean) => void
    onConfirm: (credential: typeof confirmCredential) => Promise<void>
    title: string
  }) =>
    open ?
      <div>
        <div>{title}</div>
        <button
          type="button"
          onClick={() =>
            void onConfirm(confirmCredential)
              .then(() => onOpenChange(false))
              .catch(() => {})
          }
        >
          Confirm link
        </button>
      </div>
    : null,
}))

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({
    open,
    children,
  }: {
    open: boolean
    onOpenChange: (open: boolean) => void
    children: ReactNode
  }) => (open ? <div>{children}</div> : null),
  DialogContent: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogDescription: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogFooter: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

describe('SSOLinkDialog desktop branch', () => {
  const onOpenChange = vi.fn()
  const linkSSO = vi.fn().mockResolvedValue({})
  const account = {
    value: { linkSSO },
    loading: false,
    error: null,
    retry: vi.fn(),
  }

  beforeEach(() => {
    cleanup()
    mockStartDesktopSSOLink.mockReset()
    mockToastSuccess.mockReset()
    openSpy.mockReset()
    onOpenChange.mockReset()
    linkSSO.mockClear()
    vi.stubGlobal('open', openSpy)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('routes through the native RPC without opening a popup or sending origin', async () => {
    mockStartDesktopSSOLink.mockResolvedValue({
      ssoProvider: 'google',
      code: 'desktop-code-999',
    })

    render(
      <SSOLinkDialog
        open={true}
        provider="google"
        account={account as never}
        onOpenChange={onOpenChange}
      />,
    )

    fireEvent.click(screen.getByText('Continue in browser'))

    await waitFor(() => {
      expect(mockStartDesktopSSOLink).toHaveBeenCalledTimes(1)
    })

    const call = mockStartDesktopSSOLink.mock.calls[0] ?? []
    const req = call[0] as Record<string, unknown>
    expect(req).toEqual({ ssoProvider: 'google' })
    expect(req).not.toHaveProperty('origin')
    expect(req).not.toHaveProperty('redirectPath')
    expect(req).not.toHaveProperty('redirectUri')
    expect(call[1]).toBeInstanceOf(AbortSignal)

    expect(openSpy).not.toHaveBeenCalled()

    fireEvent.click(await screen.findByText('Confirm link'))

    await waitFor(() => {
      expect(linkSSO).toHaveBeenCalledWith(
        expect.objectContaining({
          provider: 'google',
          code: 'desktop-code-999',
          redirectUri: 'https://account.test/auth/sso/callback',
        }),
      )
    })
    expect(mockToastSuccess).toHaveBeenCalledWith('Linked Google')
  })

  it('surfaces desktop RPC errors without opening a popup', async () => {
    mockStartDesktopSSOLink.mockRejectedValue(new Error('relay timeout'))

    render(
      <SSOLinkDialog
        open={true}
        provider="github"
        account={account as never}
        onOpenChange={onOpenChange}
      />,
    )

    fireEvent.click(screen.getByText('Continue in browser'))

    await waitFor(() => {
      expect(screen.getByText('relay timeout')).toBeTruthy()
    })
    expect(openSpy).not.toHaveBeenCalled()
    expect(linkSSO).not.toHaveBeenCalled()
  })
})
