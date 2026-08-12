import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { JoinSpaceDialog } from './JoinSpaceDialog.js'
import { JoinSpaceViaInviteResult } from '@s4wave/sdk/session/session.pb.js'

const mockLookupInviteCode = vi.hoisted(() => vi.fn())
const mockJoinSpaceViaInvite = vi.hoisted(() => vi.fn())
const mockAccepted = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: <T,>(res: { value: T | null }) => res.value,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          lookupInviteCode: mockLookupInviteCode,
        },
        joinSpaceViaInvite: mockJoinSpaceViaInvite,
      },
      loading: false,
      error: null,
      retry: vi.fn(),
    }),
  },
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: (...args: unknown[]) => {
    const info: unknown = mockUseSessionInfo(...args)
    return info
  },
}))

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({
    children,
    open,
    onOpenChange,
  }: {
    children?: React.ReactNode
    open: boolean
    onOpenChange: (open: boolean) => void
  }) =>
    open ? (
      <div>
        {children}
        <button type="button" onClick={() => onOpenChange(false)}>
          Dismiss dialog
        </button>
      </div>
    ) : null,
  DialogContent: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogDescription: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

describe('JoinSpaceDialog', () => {
  beforeEach(() => {
    cleanup()
    mockLookupInviteCode.mockReset()
    mockJoinSpaceViaInvite.mockReset()
    mockAccepted.mockReset()
    mockUseSessionInfo.mockReset()
    mockUseSessionInfo.mockReturnValue({ isCloud: true })
    mockLookupInviteCode.mockResolvedValue({
      inviteMessage: {
        inviteId: 'inv-1',
        sharedObjectId: 'so-1',
      },
    })
  })

  afterEach(() => {
    cleanup()
  })

  function renderDialog() {
    render(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={() => {}}
        initialCode="abc123"
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Join Space' }))
  }

  it('disables invalid input and labels the invite field', () => {
    render(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={() => {}}
      />,
    )

    expect(screen.getByLabelText('Invite code or link')).toBeDefined()
    expect(
      (screen.getByRole('button', { name: 'Join Space' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true)
  })

  it('shows progress, reports an error, and retries the same invite', async () => {
    let rejectLookup: ((error: Error) => void) | undefined
    mockLookupInviteCode
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectLookup = reject
          }),
      )
      .mockResolvedValueOnce({ inviteMessage: { inviteId: 'inv-2' } })
    mockJoinSpaceViaInvite.mockResolvedValue({
      result: JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_REJECTED,
    })

    renderDialog()
    expect(screen.getByText('Looking up invite...')).toBeDefined()
    rejectLookup?.(new Error('Invite service unavailable'))

    expect((await screen.findByRole('alert')).textContent).toContain(
      'Invite service unavailable',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

    await waitFor(() => {
      expect(mockLookupInviteCode).toHaveBeenCalledTimes(2)
      expect(screen.getByText('Invite rejected')).toBeDefined()
    })
  })

  it('invalidates an in-flight invite lookup when closed and reopened', async () => {
    let resolveLookup: ((value: { inviteMessage: object }) => void) | undefined
    mockLookupInviteCode.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveLookup = resolve
        }),
    )
    const onOpenChange = vi.fn()
    const { rerender } = render(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={onOpenChange}
        initialCode="abc123"
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Join Space' }))
    rerender(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={false}
        onOpenChange={onOpenChange}
      />,
    )
    rerender(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={onOpenChange}
      />,
    )
    resolveLookup?.({ inviteMessage: { inviteId: 'stale' } })
    await Promise.resolve()
    await Promise.resolve()

    await waitFor(() => {
      expect(mockJoinSpaceViaInvite).not.toHaveBeenCalled()
      expect(
        (screen.getByLabelText('Invite code or link') as HTMLInputElement)
          .value,
      ).toBe('')
    })
  })

  it('invalidates an in-flight lookup when unmounted', async () => {
    let resolveLookup: ((value: { inviteMessage: object }) => void) | undefined
    mockLookupInviteCode.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveLookup = resolve
        }),
    )
    const { unmount } = render(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={() => {}}
        initialCode="abc123"
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Join Space' }))
    unmount()
    resolveLookup?.({ inviteMessage: { inviteId: 'stale' } })
    await Promise.resolve()
    await Promise.resolve()

    expect(mockJoinSpaceViaInvite).not.toHaveBeenCalled()
  })

  it('ignores an in-flight join result after the dialog closes', async () => {
    let resolveJoin: ((value: object) => void) | undefined
    mockJoinSpaceViaInvite.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveJoin = resolve
        }),
    )
    const onOpenChange = vi.fn()
    const { rerender } = render(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={onOpenChange}
        initialCode="abc123"
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Join Space' }))
    await waitFor(() => expect(mockJoinSpaceViaInvite).toHaveBeenCalledTimes(1))
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss dialog' }))
    rerender(
      <JoinSpaceDialog
        onAccepted={mockAccepted}
        open={true}
        onOpenChange={onOpenChange}
      />,
    )
    resolveJoin?.({
      result: JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_ACCEPTED,
      sharedObjectId: 'stale-space',
    })
    await Promise.resolve()
    await Promise.resolve()

    await waitFor(() => {
      expect(screen.queryByText('Joined successfully!')).toBeNull()
      expect(
        (screen.getByLabelText('Invite code or link') as HTMLInputElement)
          .value,
      ).toBe('')
    })
  })

  it('returns to input state when the invite is edited after a failure', async () => {
    mockLookupInviteCode.mockRejectedValueOnce(new Error('Invite expired'))
    renderDialog()

    expect(await screen.findByRole('alert')).toBeDefined()
    const input = screen.getByLabelText('Invite code or link')
    fireEvent.change(input, { target: { value: 'replacement' } })

    expect(screen.queryByRole('alert')).toBeNull()
    expect(input.getAttribute('aria-invalid')).toBe('false')
    fireEvent.click(screen.getByRole('button', { name: 'Join Space' }))

    await waitFor(() => {
      expect(mockLookupInviteCode).toHaveBeenLastCalledWith('replacement')
    })
  })

  it('renders pending owner approval state for cloud mailbox submit', async () => {
    mockJoinSpaceViaInvite.mockResolvedValue({
      result:
        JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL,
      sharedObjectId: 'so-1',
    })

    renderDialog()

    await waitFor(() => {
      expect(screen.getByText('Awaiting owner approval')).toBeDefined()
      expect(mockLookupInviteCode).toHaveBeenCalledWith('abc123')
      expect(
        screen.getByText(
          'The owner must approve this invite before you can open the shared Space. Return here to retry after approval.',
        ),
      ).toBeDefined()
    })
  })

  it('renders joined state for accepted invite results', async () => {
    mockJoinSpaceViaInvite.mockResolvedValue({
      result: JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_ACCEPTED,
      sharedObjectId: 'so-1',
    })

    renderDialog()

    await waitFor(() => {
      expect(screen.getByText('Joined successfully!')).toBeDefined()
      expect(screen.getByText('Your shared Space is ready.')).toBeDefined()
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Open the shared Space' }),
    )
    expect(mockAccepted).toHaveBeenCalledWith('so-1')
  })

  it('renders rejected state for rejected invite results', async () => {
    mockJoinSpaceViaInvite.mockResolvedValue({
      result: JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_REJECTED,
      sharedObjectId: 'so-1',
    })

    renderDialog()

    await waitFor(() => {
      expect(screen.getByText('Invite rejected')).toBeDefined()
      expect(
        screen.getByText('This invite was denied or is no longer valid.'),
      ).toBeDefined()
    })
  })
})
