import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AuthUnlockWizard } from './AuthUnlockWizard.js'

interface MockUseEntityKeypairsResult {
  keypairs: Array<{
    keypair?: { peerId?: string; authMethod?: string }
    unlocked?: boolean
  }>
  unlockedCount: number
  loading: boolean
}

interface MockSSOPopupFlow {
  waitForResult: Promise<string>
  cancel: () => void
}

const mockUseEntityKeypairs = vi.hoisted(() =>
  vi.fn<() => MockUseEntityKeypairsResult>(),
)
const mockRecoverPasskeyEntityPem = vi.hoisted(() =>
  vi.fn<
    (
      ...args: unknown[]
    ) => Promise<
      | { case: 'pem'; pemPrivateKey: Uint8Array }
      | { case: 'pin'; encryptedBlobBase64: string }
    >
  >(),
)
const mockRecoverSSOEntityPem = vi.hoisted(() =>
  vi.fn<
    (
      ...args: unknown[]
    ) => Promise<
      | { case: 'pem'; pemPrivateKey: Uint8Array }
      | { case: 'pin'; encryptedBlobBase64: string }
    >
  >(),
)
const mockResolveRecoveredEntityPem = vi.hoisted(() =>
  vi.fn<(...args: unknown[]) => Promise<Uint8Array>>(),
)
const mockStartSSOPopupFlow = vi.hoisted(() => vi.fn<() => MockSSOPopupFlow>())

class MockFileReader {
  result: ArrayBuffer | null = null
  onload: null | (() => void) = null

  readAsArrayBuffer(file: Blob) {
    void file.arrayBuffer().then((buf) => {
      this.result = buf
      this.onload?.()
    })
  }
}

vi.mock('./useEntityKeypairs.js', () => ({
  useEntityKeypairs: () => mockUseEntityKeypairs(),
}))

vi.mock('./accountEscalationUnlock.js', () => ({
  recoverPasskeyEntityPem: (...args: unknown[]) =>
    mockRecoverPasskeyEntityPem(...args),
  recoverSSOEntityPem: (...args: unknown[]) => mockRecoverSSOEntityPem(...args),
  resolveRecoveredEntityPem: (...args: unknown[]) =>
    mockResolveRecoveredEntityPem(...args),
}))

vi.mock('./sso-popup.js', () => ({
  startSSOPopupFlow: () => mockStartSSOPopupFlow(),
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => ({ lookupProvider: vi.fn() }),
}))

vi.mock('@s4wave/app/provider/spacewave/useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => ({
    ssoBaseUrl: 'https://account.test/auth/sso',
    accountBaseUrl: 'https://account.test',
  }),
}))

describe('AuthUnlockWizard', () => {
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
    vi.stubGlobal('FileReader', MockFileReader)
    unlockEntityKeypair.mockReset()
    lockEntityKeypair.mockReset()
    lockAllEntityKeypairs.mockReset()
    mockUseEntityKeypairs.mockReset()
    mockRecoverPasskeyEntityPem.mockReset()
    mockRecoverSSOEntityPem.mockReset()
    mockResolveRecoveredEntityPem.mockReset()
    mockStartSSOPopupFlow.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('disables and marks the password method input read-only while unlock is in flight', async () => {
    let resolveUnlock: (() => void) | undefined
    mockUseEntityKeypairs.mockReturnValue({
      keypairs: [
        {
          keypair: { peerId: 'peer-password', authMethod: 'password' },
          unlocked: false,
        },
      ],
      unlockedCount: 0,
      loading: false,
    })
    unlockEntityKeypair.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveUnlock = resolve
      }),
    )

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

    const input = screen.getByPlaceholderText('Enter your password')
    fireEvent.change(input, { target: { value: 'secret-pass' } })
    fireEvent.click(screen.getByRole('button', { name: 'Unlock' }))

    await waitFor(() => {
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-password', {
        credential: {
          case: 'password',
          value: 'secret-pass',
        },
      })
    })

    expect(input.hasAttribute('disabled')).toBe(true)
    expect(input.hasAttribute('readonly')).toBe(true)
    const loadingButton = screen.getByRole('button', { name: '...' })
    expect(loadingButton.hasAttribute('disabled')).toBe(true)

    resolveUnlock?.()
  })

  it('maps backup-key unlock errors through the shared auth error mapper', async () => {
    mockUseEntityKeypairs.mockReturnValue({
      keypairs: [
        {
          keypair: { peerId: 'peer-pem', authMethod: 'pem' },
          unlocked: false,
        },
      ],
      unlockedCount: 0,
      loading: false,
    })
    unlockEntityKeypair.mockRejectedValue(new Error('unknown_keypair'))

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

    const file = new File(['pem-data'], 'backup.pem', {
      type: 'application/x-pem-file',
    })
    const input = document.querySelector('input[type="file"]')
    if (!input) {
      throw new Error('missing file input')
    }
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => {
      const unlockButton = screen.getByRole('button', { name: 'Unlock' })
      expect(unlockButton.hasAttribute('disabled')).toBe(false)
    })

    fireEvent.click(screen.getByRole('button', { name: 'Unlock' }))

    await waitFor(() => {
      expect(
        screen.getByText('The selected key is not registered on this account.'),
      ).toBeDefined()
    })
  })

  it('unlocks a passkey row through the shared passkey recovery helper', async () => {
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
      pemPrivateKey: new Uint8Array([1, 2, 3]),
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

    fireEvent.click(screen.getByText('Use passkey'))

    await waitFor(() => {
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-passkey', {
        credential: {
          case: 'pemPrivateKey',
          value: new Uint8Array([1, 2, 3]),
        },
      })
    })
  })

  it('unlocks an SSO row through the shared popup and recovery helpers', async () => {
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
    mockStartSSOPopupFlow.mockReturnValue({
      waitForResult: Promise.resolve('oauth-123'),
      cancel: vi.fn(),
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
      expect(mockStartSSOPopupFlow).toHaveBeenCalled()
      expect(mockRecoverSSOEntityPem).toHaveBeenCalled()
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-google', {
        credential: {
          case: 'pemPrivateKey',
          value: new Uint8Array([4, 5, 6]),
        },
      })
    })
  })

  it('resumes a passkey unlock with a PIN inside the shared browser method card', async () => {
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
      case: 'pin',
      encryptedBlobBase64: 'blob-123',
    })
    mockResolveRecoveredEntityPem.mockResolvedValue(new Uint8Array([7, 8, 9]))
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

    const pinInput = await screen.findByPlaceholderText('Enter PIN')
    fireEvent.change(pinInput, { target: { value: '2468' } })
    fireEvent.click(screen.getByRole('button', { name: 'Unlock' }))

    await waitFor(() => {
      expect(mockResolveRecoveredEntityPem).toHaveBeenCalledWith(
        expect.objectContaining({ lookupProvider: expect.any(Function) }),
        {
          case: 'pin',
          encryptedBlobBase64: 'blob-123',
        },
        '2468',
      )
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-passkey', {
        credential: {
          case: 'pemPrivateKey',
          value: new Uint8Array([7, 8, 9]),
        },
      })
    })
  })

  it('shows SSO waiting and cancel states inside the shared browser method card', async () => {
    let rejectFlow: ((err: Error) => void) | undefined
    const cancel = vi.fn()
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
    mockStartSSOPopupFlow.mockReturnValue({
      waitForResult: new Promise<string>((_, reject) => {
        rejectFlow = reject
      }),
      cancel,
    })

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

    await screen.findByText('Complete Google in the browser, then return here.')
    fireEvent.click(screen.getAllByRole('button', { name: 'Cancel' })[0])

    expect(cancel).toHaveBeenCalled()

    rejectFlow?.(new Error('canceled'))
  })

  it('allows retry after an SSO failure inside the shared browser method card', async () => {
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
    mockStartSSOPopupFlow.mockReturnValue({
      waitForResult: Promise.resolve('oauth-123'),
      cancel: vi.fn(),
    })
    mockRecoverSSOEntityPem
      .mockRejectedValueOnce(new Error('invalid_signature'))
      .mockResolvedValueOnce({
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

    await screen.findByText(
      'Signature verification failed. The password or key file may be incorrect.',
    )

    fireEvent.click(screen.getByRole('button', { name: 'Use Google' }))

    await waitFor(() => {
      expect(mockRecoverSSOEntityPem).toHaveBeenCalledTimes(2)
      expect(unlockEntityKeypair).toHaveBeenCalledWith('peer-google', {
        credential: {
          case: 'pemPrivateKey',
          value: new Uint8Array([4, 5, 6]),
        },
      })
    })
  })

  it('locks all keypairs on close when retention is disabled', async () => {
    mockUseEntityKeypairs.mockReturnValue({
      keypairs: [
        {
          keypair: { peerId: 'peer-password', authMethod: 'password' },
          unlocked: true,
        },
      ],
      unlockedCount: 1,
      loading: false,
    })

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

    fireEvent.click(screen.getAllByRole('button', { name: 'Cancel' })[0])

    await waitFor(() => {
      expect(lockAllEntityKeypairs).toHaveBeenCalledTimes(1)
    })
  })

  it('keeps keypairs unlocked on close when retention is enabled', async () => {
    mockUseEntityKeypairs.mockReturnValue({
      keypairs: [
        {
          keypair: { peerId: 'peer-password', authMethod: 'password' },
          unlocked: true,
        },
      ],
      unlockedCount: 1,
      loading: false,
    })

    render(
      <AuthUnlockWizard
        open={true}
        onClose={() => {}}
        onConfirm={async () => {}}
        title="Unlock"
        threshold={0}
        account={account as never}
        retainAfterClose={true}
      />,
    )

    fireEvent.click(screen.getAllByRole('button', { name: 'Cancel' })[0])

    await waitFor(() => {
      expect(lockAllEntityKeypairs).not.toHaveBeenCalled()
    })
  })
})
