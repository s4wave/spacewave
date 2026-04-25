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

let confirmCredential:
  | { type: 'tracker' }
  | { type: 'password'; password: string }
  | { type: 'pem'; pemData: Uint8Array } = {
  type: 'password',
  password: 'secret-password',
}

const mockToastSuccess = vi.fn()

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

vi.mock('./AuthConfirmDialog.js', () => ({
  buildEntityCredential: (
    credential:
      | typeof confirmCredential
      | {
          type: 'tracker'
        },
  ) => {
    if (credential.type === 'tracker') {
      return undefined
    }
    if (credential.type === 'password') {
      return { credential: { case: 'password', value: credential.password } }
    }
    return {
      credential: {
        case: 'pemPrivateKey',
        value: credential.pemData,
      },
    }
  },
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

class FakeBroadcastChannel {
  static channels = new Map<string, FakeBroadcastChannel>()

  public onmessage: ((event: MessageEvent) => void) | null = null

  constructor(public name: string) {
    FakeBroadcastChannel.channels.set(name, this)
  }

  close() {
    FakeBroadcastChannel.channels.delete(this.name)
  }

  postMessage() {}
}

describe('SSOLinkDialog', () => {
  const openSpy = vi.fn()
  const onOpenChange = vi.fn()
  const linkSSO = vi.fn().mockResolvedValue({})
  const account = {
    value: {
      linkSSO,
    },
    loading: false,
    error: null,
    retry: vi.fn(),
  }

  beforeEach(() => {
    cleanup()
    confirmCredential = { type: 'password', password: 'secret-password' }
    mockToastSuccess.mockReset()
    openSpy.mockReset()
    onOpenChange.mockReset()
    linkSSO.mockClear()
    vi.stubGlobal(
      'BroadcastChannel',
      FakeBroadcastChannel as unknown as typeof BroadcastChannel,
    )
    vi.stubGlobal('open', openSpy)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    FakeBroadcastChannel.channels.clear()
  })

  it('opens the provider popup and links with a direct credential', async () => {
    openSpy.mockReturnValue({ close: vi.fn() })

    render(
      <SSOLinkDialog
        open={true}
        provider="google"
        account={account as never}
        onOpenChange={onOpenChange}
      />,
    )

    fireEvent.click(screen.getByText('Continue in popup'))
    expect(openSpy).toHaveBeenCalledTimes(1)
    const url = openSpy.mock.calls[0]?.[0] as string
    expect(url).toContain('https://account.test/auth/sso/google?origin=')
    expect(url).toContain('mode=link')
    expect(url).toContain('redirect_path=')

    const [channelName, channel] =
      Array.from(FakeBroadcastChannel.channels.entries())[0] ?? []
    expect(channelName).toContain('spacewave-sso:')
    channel?.onmessage?.({
      data: {
        type: 'spacewave-sso-finish',
        mode: 'link',
        provider: 'google',
        code: 'oauth-code-123',
      },
    } as MessageEvent)

    fireEvent.click(await screen.findByText('Confirm link'))

    await waitFor(() => {
      expect(linkSSO).toHaveBeenCalledWith(
        expect.objectContaining({
          provider: 'google',
          code: 'oauth-code-123',
          redirectUri: 'https://account.test/auth/sso/callback',
          credential: {
            credential: { case: 'password', value: 'secret-password' },
          },
        }),
      )
    })
    expect(mockToastSuccess).toHaveBeenCalledWith('Linked Google')
  })

  it('uses the tracker-signed path after multisig unlock confirmation', async () => {
    confirmCredential = { type: 'tracker' }
    openSpy.mockReturnValue({ close: vi.fn() })

    render(
      <SSOLinkDialog
        open={true}
        provider="github"
        account={account as never}
        onOpenChange={onOpenChange}
      />,
    )

    fireEvent.click(screen.getByText('Continue in popup'))
    const channel = Array.from(FakeBroadcastChannel.channels.values())[0]
    channel?.onmessage?.({
      data: {
        type: 'spacewave-sso-finish',
        mode: 'link',
        provider: 'github',
        code: 'oauth-code-456',
      },
    } as MessageEvent)

    fireEvent.click(await screen.findByText('Confirm link'))

    await waitFor(() => {
      expect(linkSSO).toHaveBeenCalled()
    })
    const req = linkSSO.mock.calls[0]?.[0] as Record<string, unknown>
    expect(req.provider).toBe('github')
    expect(req.code).toBe('oauth-code-456')
    expect(req.redirectUri).toBe('https://account.test/auth/sso/callback')
    expect(req.credential).toBeUndefined()
  })

  it('keeps the dialog open when the provider identity is already linked', async () => {
    openSpy.mockReturnValue({ close: vi.fn() })
    linkSSO.mockRejectedValueOnce(
      new Error('already_linked: This OAuth identity is already linked'),
    )

    render(
      <SSOLinkDialog
        open={true}
        provider="google"
        account={account as never}
        onOpenChange={onOpenChange}
      />,
    )

    fireEvent.click(screen.getByText('Continue in popup'))
    const channel = Array.from(FakeBroadcastChannel.channels.values())[0]
    channel?.onmessage?.({
      data: {
        type: 'spacewave-sso-finish',
        mode: 'link',
        provider: 'google',
        code: 'oauth-code-conflict',
      },
    } as MessageEvent)

    fireEvent.click(await screen.findByText('Confirm link'))

    await waitFor(() => {
      expect(linkSSO).toHaveBeenCalledWith(
        expect.objectContaining({
          provider: 'google',
          code: 'oauth-code-conflict',
        }),
      )
    })
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(mockToastSuccess).not.toHaveBeenCalled()
  })
})
