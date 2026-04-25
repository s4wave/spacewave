import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SessionRoutes,
  consumePendingJoin,
  storePendingJoin,
} from './SessionRoutes.js'

const mockUseParams = vi.hoisted(() => vi.fn())
const mockUseSessionList = vi.hoisted(() => vi.fn())
const mockActiveRoutePath = vi.hoisted(() => ({ value: '/join/:code' }))

vi.mock('@s4wave/web/router/router.js', () => ({
  Route: ({ children, path }: { children?: React.ReactNode; path: string }) =>
    path === mockActiveRoutePath.value ? <>{children}</> : null,
  useParams: () => mockUseParams(),
}))

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: () => mockUseSessionList(),
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: ({ to }: { to: string }) => (
    <div data-testid="navigate">{to}</div>
  ),
}))

vi.mock('../AppQuickstart.js', () => ({
  AppQuickstart: () => null,
}))

vi.mock('../AppSession.js', () => ({
  AppSession: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/CheckoutResultPage.js', () => ({
  CheckoutResultPage: () => null,
}))

vi.mock('@s4wave/app/pair/PairCodePage.js', () => ({
  PairCodePage: () => null,
}))

describe('SessionRoutes join redirect', () => {
  beforeEach(() => {
    cleanup()
    sessionStorage.clear()
    mockUseParams.mockReset()
    mockUseSessionList.mockReset()
    mockActiveRoutePath.value = '/join/:code'
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
    expect(consumePendingJoin()).toBe('abc123')
    expect(consumePendingJoin()).toBeNull()
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

    expect(screen.getByTestId('navigate').textContent).toBe('/u/3/join/abc123')
  })

  it('stores and consumes pending join codes directly', () => {
    storePendingJoin('xyz789')
    expect(consumePendingJoin()).toBe('xyz789')
    expect(consumePendingJoin()).toBeNull()
  })
})
