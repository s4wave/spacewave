import React from 'react'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'

import { SessionsSection } from './SessionsSection.js'

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: vi.fn(),
}))
vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value: unknown }) => resource.value,
}))
vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => mockSessionResource,
  },
  useSessionNavigate: () => mockNavigate,
}))
vi.mock('@s4wave/web/ui/CollapsibleSection.js', () => ({
  CollapsibleSection: ({
    title,
    badge,
    children,
  }: {
    title: string
    badge?: React.ReactNode
    children: React.ReactNode
  }) => (
    <section>
      <h2>{title}</h2>
      {badge}
      {children}
    </section>
  ),
}))
vi.mock('./AuthConfirmDialog.js', () => ({
  buildEntityCredential: (credential: {
    type: 'password'
    password: string
  }) => ({
    credential: { case: 'password', value: credential.password },
  }),
  AuthConfirmDialog: ({
    open,
    onConfirm,
  }: {
    open: boolean
    onOpenChange: (open: boolean) => void
    title: string
    description: string
    confirmLabel?: React.ReactNode
    intent: unknown
    onConfirm: (credential: {
      type: 'password'
      password: string
    }) => Promise<void>
    account?: unknown
  }) =>
    open ?
      <button
        onClick={() => void onConfirm({ type: 'password', password: 'secret' })}
      >
        Confirm Session Revoke
      </button>
    : null,
}))

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { AccountSessionKind } from '@s4wave/sdk/account/account.pb.js'

const mockNavigate = vi.fn()
const mockSession = {
  unlinkDevice: vi.fn().mockResolvedValue(undefined),
}
const mockAccountValue = {
  revokeSession: vi.fn().mockResolvedValue(undefined),
}
const mockSessionResource = {
  value: mockSession,
  loading: false,
  error: null,
  retry: vi.fn(),
}
const mockAccountResource = {
  value: mockAccountValue as unknown as Account,
  loading: false,
  error: null,
  retry: vi.fn(),
} satisfies Resource<Account>

const mockUseStreamingResource = useStreamingResource as ReturnType<
  typeof vi.fn
>

function sessionsResult(value: unknown, loading = false) {
  return {
    value,
    loading,
    error: null,
    retry: vi.fn(),
  }
}

describe('SessionsSection', () => {
  beforeEach(() => {
    mockNavigate.mockClear()
    mockSession.unlinkDevice.mockClear()
    mockAccountValue.revokeSession.mockClear()
    Object.defineProperty(window, 'confirm', {
      value: vi.fn(() => true),
      writable: true,
      configurable: true,
    })
    mockUseStreamingResource.mockReturnValue(sessionsResult(null, true))
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders the section heading', () => {
    mockUseStreamingResource.mockReturnValue(sessionsResult({ sessions: [] }))
    render(<SessionsSection account={mockAccountResource} isLocal={false} />)
    expect(screen.getByText('Sessions')).toBeDefined()
  })

  it('renders the empty local state with link action', () => {
    mockUseStreamingResource.mockReturnValue(sessionsResult({ sessions: [] }))
    render(<SessionsSection account={mockAccountResource} isLocal />)

    expect(screen.getByText('No linked sessions yet.')).toBeDefined()

    fireEvent.click(screen.getByText('Link My Device'))
    expect(mockNavigate).toHaveBeenCalledWith({
      path: 'setup/link-device',
    })
  })

  it('renders the empty cloud state once the sessions watch resolves', () => {
    mockUseStreamingResource.mockReturnValue(sessionsResult({ sessions: [] }))
    render(<SessionsSection account={mockAccountResource} isLocal={false} />)

    expect(screen.getByText('No other sessions found.')).toBeDefined()
    expect(screen.queryByText('Loading sessions...')).toBeNull()
  })

  it('renders current and other session rows with metadata', () => {
    mockUseStreamingResource.mockReturnValue(
      sessionsResult({
        sessions: [
          {
            peerId: 'peer-current',
            currentSession: true,
            kind: AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_CLOUD_AUTH_SESSION,
            label: 'Chrome on macOS (Portland, OR)',
            clientName: 'Chrome',
            os: 'macOS',
            location: 'Portland, OR',
            lastSeenAt: new Date('2026-04-16T00:00:00Z'),
          },
          {
            peerId: 'peer-other',
            currentSession: false,
            kind: AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION,
            label: 'Laptop',
            createdAt: new Date('2026-04-15T00:00:00Z'),
          },
        ],
      }),
    )
    render(<SessionsSection account={mockAccountResource} isLocal />)

    expect(screen.getByText('Chrome on macOS (Portland, OR)')).toBeDefined()
    expect(screen.getByText('This device')).toBeDefined()
    expect(screen.getByText('Laptop')).toBeDefined()
    expect(screen.getByText('Linked')).toBeDefined()
  })

  it('unlinks a local non-current session with confirmation', async () => {
    mockUseStreamingResource.mockReturnValue(
      sessionsResult({
        sessions: [
          {
            peerId: 'peer-1',
            label: 'Laptop',
            kind: AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION,
          },
        ],
      }),
    )
    render(<SessionsSection account={mockAccountResource} isLocal />)

    fireEvent.click(screen.getByTitle('Unlink session'))
    await waitFor(() => {
      expect(mockSession.unlinkDevice).toHaveBeenCalledWith('peer-1')
    })
  })

  it('revokes a cloud non-current session with confirmation', async () => {
    mockUseStreamingResource.mockReturnValue(
      sessionsResult({
        sessions: [
          {
            peerId: 'peer-2',
            label: 'Safari',
            kind: AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_CLOUD_AUTH_SESSION,
          },
        ],
      }),
    )
    render(<SessionsSection account={mockAccountResource} isLocal={false} />)

    fireEvent.click(screen.getByTitle('Log out session'))
    fireEvent.click(screen.getByText('Confirm Session Revoke'))
    await waitFor(() => {
      expect(mockAccountValue.revokeSession).toHaveBeenCalledWith({
        sessionPeerId: 'peer-2',
        credential: {
          credential: { case: 'password', value: 'secret' },
        },
      })
    })
  })

  it('does not render a destructive action for the current session row', () => {
    mockUseStreamingResource.mockReturnValue(
      sessionsResult({
        sessions: [
          {
            peerId: 'peer-current',
            currentSession: true,
            kind: AccountSessionKind.AccountSessionKind_ACCOUNT_SESSION_KIND_CLOUD_AUTH_SESSION,
          },
        ],
      }),
    )
    render(<SessionsSection account={mockAccountResource} isLocal={false} />)

    expect(screen.queryByTitle('Log out session')).toBeNull()
    expect(screen.queryByTitle('Unlink session')).toBeNull()
  })
})
