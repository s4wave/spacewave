import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SessionRoutes,
  consumePendingJoin,
  storePendingJoin,
} from './SessionRoutes.js'

import { JoinSpaceDialog } from '@s4wave/app/sobject/JoinSpaceDialog.js'
import { SOInviteMessage } from '@s4wave/core/sobject/sobject.pb.js'
import { JoinSpaceViaInviteResult } from '@s4wave/sdk/session/session.pb.js'
import { base58Encode } from '@s4wave/app/provider/spacewave/keypair-utils.js'
import { buildInviteLink } from '@s4wave/app/urls.js'

const mockLookupInviteCode = vi.hoisted(() => vi.fn())
const mockJoinSpaceViaInvite = vi.hoisted(() => vi.fn())
const mockUseSessionInfo = vi.hoisted(() => vi.fn())
const mockUseParams = vi.hoisted(() => vi.fn())
const mockUseSessionList = vi.hoisted(() => vi.fn())
const mockActiveRoutePath = vi.hoisted(() => ({ value: '/join/:code' }))

vi.mock('@s4wave/web/router/router.js', () => ({
  Route: ({ children, path }: { children?: React.ReactNode; path: string }) =>
    path === mockActiveRoutePath.value ? <>{children}</> : null,
  useParams: () => {
    const params: unknown = mockUseParams()
    return params
  },
}))

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: () => {
    const list: unknown = mockUseSessionList()
    return list
  },
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: ({ to }: { to: string }) => (
    <div data-testid="navigate">{to}</div>
  ),
}))

vi.mock('../AppQuickstart.js', () => ({
  AppQuickstart: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/CheckoutResultPage.js', () => ({
  CheckoutResultPage: () => null,
}))

vi.mock('@s4wave/app/pair/PairCodePage.js', () => ({
  PairCodePage: () => null,
}))

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

vi.mock('@s4wave/web/ui/loading/Spinner.js', () => ({
  Spinner: () => null,
}))

describe('SessionRoutes join redirect', () => {
  beforeEach(() => {
    cleanup()
    sessionStorage.clear()
    mockUseParams.mockReset()
    mockUseSessionList.mockReset()
    mockActiveRoutePath.value = '/join/:code'
    mockLookupInviteCode.mockReset()
    mockJoinSpaceViaInvite.mockReset()
    mockUseSessionInfo.mockReset()
  })

  afterEach(() => {
    cleanup()
    sessionStorage.clear()
  })

  it('stores the invite code and redirects to root when no session exists yet', () => {
    mockUseParams.mockReturnValue({ code: 'abc123' })
    mockUseSessionList.mockReturnValue({
      loading: false,
      value: { sessions: [] },
    })

    render(SessionRoutes)

    expect(screen.getByTestId('navigate').textContent).toBe('/')
    expect(consumePendingJoin()).toBe('bearer:abc123')
    expect(consumePendingJoin()).toBeNull()
  })

  it('replays the generated hash invite after the pending session handoff', async () => {
    const invite = {
      inviteId: 'invite-1',
      sharedObjectId: 'space-1',
      providerId: 'local',
    }
    const link = buildInviteLink(
      'https://spacewave.app',
      base58Encode(SOInviteMessage.toBinary(invite)),
    )
    const payload = link.split('/').at(-1)

    mockUseParams.mockReturnValue({ code: payload })
    mockUseSessionList.mockReturnValue({
      loading: false,
      value: { sessions: [] },
    })
    mockUseSessionInfo.mockReturnValue({ isCloud: true })
    mockJoinSpaceViaInvite.mockResolvedValue({
      result: JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_ACCEPTED,
      sharedObjectId: invite.sharedObjectId,
    })

    render(SessionRoutes)
    const pendingCode = consumePendingJoin()
    expect(pendingCode).toBe(`bearer:${payload}`)

    render(
      <JoinSpaceDialog
        open={true}
        onAccepted={() => {}}
        onOpenChange={() => {}}
        initialCode={pendingCode ?? undefined}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Join Space' }))

    await waitFor(() => {
      expect(mockLookupInviteCode).not.toHaveBeenCalled()
      expect(mockJoinSpaceViaInvite).toHaveBeenCalledWith(invite)
    })
  })

  it('redirects to the first mounted session join route when a session exists', () => {
    mockUseParams.mockReturnValue({ code: 'abc123' })
    mockUseSessionList.mockReturnValue({
      loading: false,
      value: {
        sessions: [{ sessionIndex: 3 }],
      },
    })

    render(SessionRoutes)

    expect(screen.getByTestId('navigate').textContent).toBe(
      '/u/3/join/bearer:abc123',
    )
  })

  it('stores and consumes pending join codes directly', () => {
    storePendingJoin('xyz789')
    expect(consumePendingJoin()).toBe('bearer:xyz789')
    expect(consumePendingJoin()).toBeNull()
  })
})
