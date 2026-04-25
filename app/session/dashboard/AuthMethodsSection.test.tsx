import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { AuthMethodsSection } from './AuthMethodsSection.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import { AccountAuthMethodKind } from '@s4wave/core/provider/spacewave/api/api.pb.js'

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: vi.fn(),
}))
vi.mock('@s4wave/web/ui/CollapsibleSection.js', () => ({
  CollapsibleSection: ({
    title,
    badge,
    headerActions,
    children,
  }: {
    title: string
    badge?: React.ReactNode
    headerActions?: React.ReactNode
    children: React.ReactNode
  }) => (
    <section>
      <h2>{title}</h2>
      {badge}
      {headerActions}
      {children}
    </section>
  ),
}))
vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['session-settings'],
  useStateAtom: (_ns: unknown, _key: string, init: boolean) =>
    [init, vi.fn()] as const,
}))

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

const mockUseStreamingResource = useStreamingResource as ReturnType<
  typeof vi.fn
>

function makeAccountResource(value: Account | null = null): Resource<Account> {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

// authMethodsResult returns a mock for the watchAuthMethods streaming resource.
function authMethodsResult(value: unknown, loading = false) {
  return { value, loading, error: null, retry: vi.fn() }
}

// accountInfoResult returns a mock for the watchAccountInfo streaming resource.
function accountInfoResult(
  value: { authThreshold?: number; keypairCount?: number } | null = {
    authThreshold: 0,
    keypairCount: 0,
  },
) {
  return { value, loading: false, error: null, retry: vi.fn() }
}

// mockBothCalls sets mockReturnValueOnce for the two useStreamingResource calls
// the component makes: first watchAuthMethods, then watchAccountInfo.
function mockBothCalls(
  authMethods: ReturnType<typeof authMethodsResult>,
  accountInfo: ReturnType<typeof accountInfoResult> = accountInfoResult(),
) {
  mockUseStreamingResource
    .mockReturnValueOnce(authMethods)
    .mockReturnValueOnce(accountInfo)
}

describe('AuthMethodsSection', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  beforeEach(() => {
    // Default: return loading state for both useStreamingResource calls.
    mockUseStreamingResource.mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
  })

  it('renders loading message when loading', () => {
    mockBothCalls(authMethodsResult(null, true))
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('Loading auth methods...')).toBeDefined()
  })

  it('renders empty state when not loading and no keypairs', () => {
    mockBothCalls(authMethodsResult({ authMethods: [] }))
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('No auth methods found')).toBeDefined()
  })

  it('renders empty state when value is null and not loading', () => {
    mockBothCalls(authMethodsResult(null))
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('No auth methods found')).toBeDefined()
  })

  it('renders "Auth Methods" heading', () => {
    mockBothCalls(authMethodsResult(null))
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('Auth Methods')).toBeDefined()
  })

  it('renders a badge with the auth method count', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'key-1',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'key-1', authMethod: 'password' },
          },
          {
            peerId: 'key-2',
            label: 'Backup key (.pem)',
            kind: AccountAuthMethodKind.BACKUP_KEY,
            keypair: { peerId: 'key-2', authMethod: 'pem' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('2')).toBeDefined()
  })

  it('renders add auth method header action', () => {
    mockBothCalls(authMethodsResult(null))
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByLabelText('Add auth method')).toBeDefined()
  })

  it('renders password method label', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'short-id',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'short-id', authMethod: 'password' },
          },
          {
            peerId: 'another-id',
            label: 'Backup key (.pem)',
            kind: AccountAuthMethodKind.BACKUP_KEY,
            keypair: { peerId: 'another-id', authMethod: 'pem' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('Password')).toBeDefined()
  })

  it('renders backup key label for pem', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'id-1',
            label: 'Backup key (.pem)',
            kind: AccountAuthMethodKind.BACKUP_KEY,
            keypair: { peerId: 'id-1', authMethod: 'pem' },
          },
          {
            peerId: 'id-2',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'id-2', authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('Backup key (.pem)')).toBeDefined()
  })

  it('renders passkey label and secondary metadata', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'id-a',
            label: 'Passkey',
            kind: AccountAuthMethodKind.PASSKEY,
            secondaryLabel: 'Synced passkey',
            keypair: { peerId: 'id-a', authMethod: 'passkey' },
          },
          {
            peerId: 'id-b',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'id-b', authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('Passkey')).toBeDefined()
    expect(screen.getByText('Synced passkey')).toBeDefined()
  })

  it('renders Google metadata from the richer auth-method row', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'id-x',
            provider: 'google',
            label: 'Google',
            kind: AccountAuthMethodKind.GOOGLE_SSO,
            secondaryLabel: 'user@example.com',
            keypair: { peerId: 'id-x', authMethod: 'google_sso' },
          },
          {
            peerId: 'id-y',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'id-y', authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('Google')).toBeDefined()
    expect(screen.getByText('user@example.com')).toBeDefined()
  })

  it('renders GitHub rows as removable auth methods', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'id-gh',
            provider: 'github',
            label: 'GitHub',
            kind: AccountAuthMethodKind.GITHUB_SSO,
            secondaryLabel: 'octo@example.com',
            keypair: { peerId: 'id-gh', authMethod: 'github_sso' },
          },
          {
            peerId: 'id-pw',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'id-pw', authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText('GitHub')).toBeDefined()
    expect(screen.getByText('octo@example.com')).toBeDefined()
    expect(screen.getAllByText('Remove')).toHaveLength(2)
  })

  it('truncates peer IDs longer than 16 characters', () => {
    const longId = 'abcdefghijklmnopqrstuvwxyz'
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: longId,
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: longId, authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    const truncated = longId.slice(0, 8) + '...' + longId.slice(-8)
    expect(screen.getByText(truncated)).toBeDefined()
  })

  it('does not truncate peer IDs 16 characters or shorter', () => {
    const shortId = '1234567890123456'
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: shortId,
            label: 'Backup key (.pem)',
            kind: AccountAuthMethodKind.BACKUP_KEY,
            keypair: { peerId: shortId, authMethod: 'pem' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getByText(shortId)).toBeDefined()
  })

  it('disables remove button when only one keypair exists', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'solo-key',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'solo-key', authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    const removeButton = screen.getByText('Remove').closest('button')
    expect(removeButton?.hasAttribute('disabled')).toBe(true)
  })

  it('enables remove button when multiple keypairs exist', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'key-1',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'key-1', authMethod: 'password' },
          },
          {
            peerId: 'key-2',
            label: 'Backup key (.pem)',
            kind: AccountAuthMethodKind.BACKUP_KEY,
            keypair: { peerId: 'key-2', authMethod: 'pem' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    const removeButtons = screen.getAllByText('Remove')
    removeButtons.forEach((btn) => {
      const button = btn.closest('button')
      expect(button?.hasAttribute('disabled')).toBe(false)
    })
  })

  it('shows change only for password rows', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'key-1',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'key-1', authMethod: 'password' },
          },
          {
            peerId: 'key-2',
            label: 'Passkey',
            kind: AccountAuthMethodKind.PASSKEY,
            keypair: { peerId: 'key-2', authMethod: 'passkey' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getAllByText('Change')).toHaveLength(1)
    expect(screen.getAllByText('Remove')).toHaveLength(2)
  })

  it('hides remove for unknown auth-method rows', () => {
    mockBothCalls(
      authMethodsResult({
        authMethods: [
          {
            peerId: 'key-1',
            label: 'Mystery method',
            kind: AccountAuthMethodKind.UNKNOWN,
            keypair: { peerId: 'key-1', authMethod: 'mystery' },
          },
          {
            peerId: 'key-2',
            label: 'Password',
            kind: AccountAuthMethodKind.PASSWORD,
            keypair: { peerId: 'key-2', authMethod: 'password' },
          },
        ],
      }),
    )
    render(<AuthMethodsSection account={makeAccountResource({} as Account)} />)
    expect(screen.getAllByText('Remove')).toHaveLength(1)
    expect(screen.getAllByText('Change')).toHaveLength(1)
  })

  it('refreshes the viewer when watchAuthMethods emits a newly linked Google row', () => {
    const passwordRow = {
      peerId: 'id-pw',
      label: 'Password',
      kind: AccountAuthMethodKind.PASSWORD,
      keypair: { peerId: 'id-pw', authMethod: 'password' },
    }
    const googleRow = {
      peerId: 'id-google',
      provider: 'google',
      label: 'Google',
      kind: AccountAuthMethodKind.GOOGLE_SSO,
      secondaryLabel: 'linked@example.com',
      keypair: { peerId: 'id-google', authMethod: 'google_sso' },
    }
    let authMethods = { authMethods: [passwordRow] }
    let callCount = 0
    mockUseStreamingResource.mockImplementation(() => {
      callCount++
      if (callCount % 2 === 1) {
        return authMethodsResult(authMethods)
      }
      return accountInfoResult()
    })

    const account = makeAccountResource({} as Account)
    const { rerender } = render(<AuthMethodsSection account={account} />)
    expect(screen.queryByText('Google')).toBeNull()

    authMethods = { authMethods: [passwordRow, googleRow] }
    rerender(<AuthMethodsSection account={account} />)

    expect(screen.getByText('Google')).toBeDefined()
    expect(screen.getByText('linked@example.com')).toBeDefined()
  })
})
