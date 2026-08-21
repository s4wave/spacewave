import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'

import { AppSession } from './AppSession.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import {
  releaseQuickstartSessionHandoffsForTests,
  stageQuickstartSessionHandoff,
} from './quickstart/session-handoff.js'

const mockUseParams = vi.hoisted(() => vi.fn())
const mockUseNavigate = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn())
const mockUseSessionMetadata = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  resolvePath: vi.fn(),
  useNavigate: () => mockUseNavigate,
  useParams: mockUseParams,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: mockUseResource,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('@s4wave/web/state/interaction.js', () => ({
  markInteracted: vi.fn(),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionIndexContext: {
    Provider: ({ children }: { children?: ReactNode }) => <>{children}</>,
  },
  SessionRouteContext: {
    Provider: ({ children }: { children?: ReactNode }) => <>{children}</>,
  },
}))

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: mockUseSessionMetadata,
}))

vi.mock('./session/SessionContainer.js', () => ({
  SessionContainer: () => <div data-testid="session-container" />,
}))

vi.mock('./session/PinUnlockOverlay.js', () => ({
  PinUnlockOverlay: () => <div data-testid="pin-unlock-overlay" />,
}))

describe('AppSession', () => {
  afterEach(() => {
    cleanup()
    releaseQuickstartSessionHandoffsForTests()
    vi.clearAllMocks()
  })

  it('describes a pending session attachment as connecting', () => {
    mockUseParams.mockReturnValue({ sessionIndex: '1' })
    mockUseRootResource.mockReturnValue({ value: null })
    mockUseSessionMetadata.mockReturnValue(null)
    mockUseResource.mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })

    render(<AppSession />)

    expect(screen.getByText('Connecting to the session.')).toBeTruthy()
  })

  it('renders the pre-mount PIN unlock overlay for locked sessions', () => {
    mockUseParams.mockReturnValue({ sessionIndex: '1' })
    mockUseRootResource.mockReturnValue({ value: null })
    mockUseSessionMetadata.mockReturnValue({
      lockMode: SessionLockMode.PIN_ENCRYPTED,
      displayName: 'Cloud Session',
    })
    mockUseResource.mockReturnValue({
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    render(<AppSession />)

    expect(screen.getByTestId('pin-unlock-overlay')).toBeTruthy()
    expect(screen.queryByTestId('session-container')).toBeNull()
  })

  it('renders the session container once the session is mounted', () => {
    mockUseParams.mockReturnValue({ sessionIndex: '1' })
    mockUseRootResource.mockReturnValue({ value: null })
    mockUseSessionMetadata.mockReturnValue({
      lockMode: SessionLockMode.PIN_ENCRYPTED,
      displayName: 'Cloud Session',
    })
    mockUseResource.mockReturnValue({
      value: {} as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    render(<AppSession />)

    expect(screen.getByTestId('session-container')).toBeTruthy()
    expect(screen.queryByTestId('pin-unlock-overlay')).toBeNull()
  })

  it('adopts a staged quickstart session before mounting by index', async () => {
    type TestRoot = {
      mountSessionByIdx: ReturnType<typeof vi.fn>
    }
    const session = {
      released: false,
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const root: TestRoot = {
      mountSessionByIdx: vi.fn(),
    }
    type SessionFactory = (
      root: TestRoot,
      signal: AbortSignal,
      cleanup: RegisterCleanup,
    ) => Promise<unknown>
    const factoryRef: { current: SessionFactory | null } = { current: null }

    stageQuickstartSessionHandoff({
      sessionIndex: 1,
      session: session as never,
    })
    mockUseParams.mockReturnValue({ sessionIndex: '1' })
    mockUseRootResource.mockReturnValue({ value: root })
    mockUseSessionMetadata.mockReturnValue(null)
    mockUseResource.mockImplementation(
      (_rootResource: unknown, resourceFactory: SessionFactory) => {
        factoryRef.current = resourceFactory
        return {
          value: null,
          loading: true,
          error: null,
          retry: vi.fn(),
        }
      },
    )

    render(<AppSession />)

    const cleanupCalls: unknown[] = []
    const cleanupResource: RegisterCleanup = (resource) => {
      cleanupCalls.push(resource)
      return resource
    }
    const factory = factoryRef.current
    if (!factory) {
      throw new Error('session factory was not registered')
    }
    const loaded = await factory(
      root,
      new AbortController().signal,
      cleanupResource,
    )
    expect(loaded).toBe(session)

    expect(root.mountSessionByIdx).not.toHaveBeenCalled()
    expect(cleanupCalls).toEqual([session])
  })
})
