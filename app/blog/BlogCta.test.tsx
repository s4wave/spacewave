import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Ref } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BlogCta } from './BlogCta.js'

interface MockTurnstileInstance {
  getResponse(): string | undefined
  getResponsePromise(): Promise<string>
  reset(): void
}

interface MockTurnstileProps {
  ref?: Ref<MockTurnstileInstance>
  siteKey: string
}

const mockNavigate = vi.hoisted(() => vi.fn())
const turnstileHarness = vi.hoisted(() => ({
  getResponse: vi.fn<() => string | undefined>(),
  getResponsePromise: vi.fn<() => Promise<string>>(),
  reset: vi.fn<() => void>(),
}))

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return false
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/ui/turnstile.js', () => ({
  TURNSTILE_PROD_SITE_KEY: 'production-site-key',
  Turnstile: ({ ref, siteKey }: MockTurnstileProps) => {
    const instance = {
      getResponse: turnstileHarness.getResponse,
      getResponsePromise: turnstileHarness.getResponsePromise,
      reset: turnstileHarness.reset,
    }

    if (typeof ref === 'function') {
      ref(instance)
    } else if (ref) {
      ref.current = instance
    }

    return <div data-testid="turnstile" data-site-key={siteKey} />
  },
}))

const originalFetch = globalThis.fetch

function parseJsonBody(init: RequestInit | undefined): unknown {
  const body = init?.body
  if (typeof body !== 'string') {
    throw new Error('expected a JSON string request body')
  }
  const value: unknown = JSON.parse(body)
  return value
}

describe('BlogCta', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    turnstileHarness.getResponse.mockReset()
    turnstileHarness.getResponsePromise.mockReset()
    turnstileHarness.reset.mockReset()
  })

  afterEach(() => {
    cleanup()
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('posts email before resolving Turnstile and upgrades the capture with the token', async () => {
    const user = userEvent.setup()
    const email = 'ada@example.com'
    let resolveCaptureResponse: (response: Response) => void = () => {}
    const captureResponse = new Promise<Response>((resolve) => {
      resolveCaptureResponse = resolve
    })
    let resolveTurnstileToken: (token: string) => void = () => {}
    const turnstileToken = new Promise<string>((resolve) => {
      resolveTurnstileToken = resolve
    })
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input) => {
      if (input === '/api/email/capture') {
        return captureResponse
      }
      if (input === '/api/email/capture/capture-123/upgrade') {
        return Promise.resolve(Response.json({ success: true }))
      }
      return Promise.resolve(new Response(null, { status: 404 }))
    })
    globalThis.fetch = fetchMock
    turnstileHarness.getResponsePromise.mockReturnValue(turnstileToken)

    render(<BlogCta />)

    expect(screen.queryByTestId('turnstile')).toBeNull()

    await user.type(screen.getByPlaceholderText('your@email.com'), email)
    await user.click(screen.getByRole('button', { name: 'Subscribe' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    expect(screen.getByTestId('turnstile')).toBeDefined()
    expect(turnstileHarness.getResponsePromise).not.toHaveBeenCalled()

    const [captureUrl, captureInit] = fetchMock.mock.calls[0]
    expect(captureUrl).toBe('/api/email/capture')
    expect(captureInit?.method).toBe('POST')
    expect(parseJsonBody(captureInit)).toEqual({
      email,
      source: 'blog',
    })

    resolveCaptureResponse(
      Response.json({ success: true, capture_id: 'capture-123' }),
    )

    await waitFor(() =>
      expect(turnstileHarness.getResponsePromise).toHaveBeenCalledTimes(1),
    )
    expect(fetchMock).toHaveBeenCalledTimes(1)

    resolveTurnstileToken('turnstile-token-123')

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))

    const [upgradeUrl, upgradeInit] = fetchMock.mock.calls[1]
    expect(upgradeUrl).toBe('/api/email/capture/capture-123/upgrade')
    expect(upgradeInit?.method).toBe('POST')
    expect(parseJsonBody(upgradeInit)).toEqual({
      turnstile_token: 'turnstile-token-123',
    })

    expect(fetchMock.mock.invocationCallOrder[0]).toBeLessThan(
      turnstileHarness.getResponsePromise.mock.invocationCallOrder[0],
    )
    expect(
      turnstileHarness.getResponsePromise.mock.invocationCallOrder[0],
    ).toBeLessThan(fetchMock.mock.invocationCallOrder[1])
  })
})
