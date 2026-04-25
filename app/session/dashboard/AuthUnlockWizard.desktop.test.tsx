import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AuthUnlockWizard } from './AuthUnlockWizard.js'

const mockStartDesktopSSOLink = vi.hoisted(() => vi.fn())
const mockStartDesktopPasskeyReauth = vi.hoisted(() => vi.fn())
const mockRecoverPasskeyEntityPem = vi.hoisted(() => vi.fn())
const mockRecoverSSOEntityPem = vi.hoisted(() => vi.fn())
const mockStartSSOPopupFlow = vi.hoisted(() => vi.fn())
const mockUseEntityKeypairs = vi.hoisted(() => vi.fn())
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
        spacewave: {
          startDesktopSSOLink: mockStartDesktopSSOLink,
          startDesktopPasskeyReauth: mockStartDesktopPasskeyReauth,
        },
      },
      loading: false,
      error: null,
      retry: () => {},
    }),
  },
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => ({
    value: { lookupProvider: vi.fn() },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/app/provider/spacewave/useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => ({
    ssoBaseUrl: 'https://account.test/auth/sso',
    accountBaseUrl: 'https://account.test',
  }),
}))

vi.mock('./useEntityKeypairs.js', () => ({
  useEntityKeypairs: () => mockUseEntityKeypairs(),
}))

vi.mock('./sso-popup.js', () => ({
  startSSOPopupFlow: () => mockStartSSOPopupFlow(),
}))

vi.mock('./accountEscalationUnlock.js', () => ({
  recoverPasskeyEntityPem: (...args: unknown[]) =>
    mockRecoverPasskeyEntityPem(...args),
  recoverSSOEntityPem: (...args: unknown[]) => mockRecoverSSOEntityPem(...args),
  resolveRecoveredEntityPem: vi.fn(),
}))

describe('AuthUnlockWizard desktop SSO unlock', () => {
  const unlockEntityKeypair = vi.fn()
  const lockEntityKeypair = vi.fn()
  const lockAllEntityKeypairs = vi.fn()
  const account = {
    value: {
      unlockEntityKeypair,
      lockEntityKeypair,
      lockAllEntityKeypairs,
    },
    loading: false,
    error: null,
    retry: vi.fn(),
  }

  beforeEach(() => {
    cleanup()
    mockStartDesktopSSOLink.mockReset()
    mockStartDesktopPasskeyReauth.mockReset()
    mockRecoverPasskeyEntityPem.mockReset()
    mockRecoverSSOEntityPem.mockReset()
    mockStartSSOPopupFlow.mockReset()
    mockUseEntityKeypairs.mockReset()
    unlockEntityKeypair.mockReset()
    lockEntityKeypair.mockReset()
    lockAllEntityKeypairs.mockReset()
    vi.stubGlobal('open', openSpy)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('uses the native desktop relay without opening a popup and still unlocks via recovered PEM', async () => {
    mockUseEntityKeypairs.mockReturnValue({
      keypairs: [
        {
          keypair: { peerId: 'peer-google', authMethod: 'google_sso' },
          unlocked: false,
        },
      ],
      unlockedCount: 0,
      loading: false,
    })
    mockStartDesktopSSOLink.mockResolvedValue({
      ssoProvider: 'google',
      code: 'desktop-code-123',
    })
    mockRecoverSSOEntityPem.mockResolvedValue({
      case: 'pem',
      pemPrivateKey: new Uint8Array([4, 5, 6]),
    })
    unlockEntityKeypair.mockResolvedValue({})

    render(
      <AuthUnlockWizard
        open={true}
        onClose={() => {}}
        onConfirm={async () => {}}
        title="Unlock"
        threshold={0}
        account={account as never}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Use Google' }))

    await waitFor(() => {
      expect(mockStartDesktopSSOLink).toHaveBeenCalledTimes(1)
    })

    const call = mockStartDesktopSSOLink.mock.calls[0] ?? []
    const req = call[0] as Record<string, unknown>
    expect(req).toEqual({ ssoProvider: 'google' })
    expect(req).not.toHaveProperty('origin')
    expect(req).not.toHaveProperty('redirectPath')
    expect(call[1]).toBeInstanceOf(AbortSignal)

    await waitFor(() => {
      expect(mockRecoverSSOEntityPem).toHaveBeenCalledWith(
        expect.anything(),
        'google',
        'desktop-code-123',
        'https://account.test/auth/sso/callback',
      )
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-google', {
        credential: {
          case: 'pemPrivateKey',
          value: new Uint8Array([4, 5, 6]),
        },
      })
    })

    expect(mockStartSSOPopupFlow).not.toHaveBeenCalled()
    expect(openSpy).not.toHaveBeenCalled()
  })

  it('routes desktop passkey unlock through the native reauth helper inputs', async () => {
    mockUseEntityKeypairs.mockReturnValue({
      keypairs: [
        {
          keypair: { peerId: 'peer-passkey', authMethod: 'passkey' },
          unlocked: false,
        },
      ],
      unlockedCount: 0,
      loading: false,
    })
    mockRecoverPasskeyEntityPem.mockResolvedValue({
      case: 'pem',
      pemPrivateKey: new Uint8Array([7, 8, 9]),
    })
    unlockEntityKeypair.mockResolvedValue({})

    render(
      <AuthUnlockWizard
        open={true}
        onClose={() => {}}
        onConfirm={async () => {}}
        title="Unlock"
        threshold={0}
        account={account as never}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Use passkey' }))

    await waitFor(() => {
      expect(mockRecoverPasskeyEntityPem).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({
          desktopSession: expect.objectContaining({
            startDesktopPasskeyReauth: mockStartDesktopPasskeyReauth,
          }),
          targetPeerId: 'peer-passkey',
        }),
      )
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-passkey', {
        credential: {
          case: 'pemPrivateKey',
          value: new Uint8Array([7, 8, 9]),
        },
      })
    })
  })
})
