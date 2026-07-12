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
  Dialog: ({ children, open }: { children?: React.ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
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
