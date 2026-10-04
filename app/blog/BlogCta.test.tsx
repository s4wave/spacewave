import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Ref } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { EmailCaptureRequest } from '@s4wave/core/provider/spacewave/api/api.pb.js'

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

  it('posts the email with the Turnstile token once Turnstile resolves', async () => {
    const user = userEvent.setup()
    const email = 'ada@example.com'
    let resolveTurnstileToken: (token: string) => void = () => {}
    const turnstileToken = new Promise<string>((resolve) => {
      resolveTurnstileToken = resolve
    })
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >(() => Promise.resolve(new Response(new Uint8Array())))
    globalThis.fetch = fetchMock
    turnstileHarness.getResponsePromise.mockReturnValue(turnstileToken)

    render(<BlogCta />)
    expect(screen.queryByTestId('turnstile')).toBeNull()

    await user.type(screen.getByPlaceholderText('your@email.com'), email)
    await user.click(screen.getByRole('button', { name: 'Subscribe' }))

    await waitFor(() =>
      expect(turnstileHarness.getResponsePromise).toHaveBeenCalledTimes(1),
    )
    expect(screen.getByTestId('turnstile')).toBeDefined()
    expect(fetchMock).not.toHaveBeenCalled()

    resolveTurnstileToken('turnstile-token-123')

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/email/capture')
    expect(init?.method).toBe('POST')
    expect(new Headers(init?.headers).get('X-Turnstile-Token')).toBe(
      'turnstile-token-123',
    )
    if (!(init?.body instanceof Uint8Array)) {
      throw new Error('expected a binary request body')
    }
    expect(EmailCaptureRequest.fromBinary(init.body)).toEqual({
      email,
      source: 'blog',
    })
    await screen.findByText('Subscribed. Thanks for joining.')
  })
})
