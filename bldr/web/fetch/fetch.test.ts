import { describe, expect, it, vi } from 'vitest'

import type { FetchService } from './fetch_srpc.pb.js'
import { proxyFetch } from './fetch.js'

describe('proxyFetch', () => {
  it('returns a 500 response when response headers never arrive before timeout', async () => {
    vi.useFakeTimers()
    try {
      const svc: FetchService = {
        Fetch() {
          return {
            [Symbol.asyncIterator]() {
              return {
                next: () => new Promise(() => {}),
              }
            },
          }
        },
      }

      const respPromise = proxyFetch(
        svc,
        new Request('https://example.test/p/test'),
        'client-1',
        { headerTimeoutMs: 50 },
      )

      await vi.advanceTimersByTimeAsync(50)
      const resp = await respPromise

      expect(resp.status).toBe(500)
      await expect(resp.text()).resolves.toContain(
        'timed out waiting 50ms for proxied fetch response headers',
      )
    } finally {
      vi.useRealTimers()
    }
  })

  it('aborts the proxied fetch when the caller-owned signal aborts', async () => {
    const outerAbort = new AbortController()
    let observedSignal: AbortSignal | undefined

    const svc: FetchService = {
      Fetch(_request, signal) {
        observedSignal = signal
        return (async function* () {
          await new Promise<never>((_, reject) => {
            signal?.addEventListener(
              'abort',
              () => reject(new Error('aborted by owner')),
              { once: true },
            )
          })
        })()
      },
    }

    const respPromise = proxyFetch(
      svc,
      new Request('https://example.test/p/test'),
      'client-1',
      { abortSignal: outerAbort.signal },
    )
    outerAbort.abort(new Error('client closed'))

    const resp = await respPromise
    expect(observedSignal?.aborted).toBe(true)
    expect(resp.status).toBe(500)
    await expect(resp.text()).resolves.toContain('aborted by owner')
  })

  it('still returns an error response when proxy error logging hits EPIPE', async () => {
    const logErr = new Error('write EPIPE') as Error & { code: string }
    logErr.code = 'EPIPE'
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {
      throw logErr
    })
    try {
      const svc: FetchService = {
        Fetch() {
          throw new Error('socket closed')
        },
      }

      const resp = await proxyFetch(
        svc,
        new Request('https://example.test/p/test'),
        'client-1',
      )

      expect(resp.status).toBe(500)
      await expect(resp.text()).resolves.toContain('socket closed')
    } finally {
      errorSpy.mockRestore()
    }
  })

  it('returns the proxied response when a header value contains unicode', async () => {
    const svc: FetchService = {
      Fetch() {
        return (async function* () {
          yield {
            body: {
              case: 'responseInfo',
              value: {
                status: 200,
                statusText: 'OK',
                headers: {
                  'content-disposition':
                    'attachment; filename="Screenshot\u202f2026.png"',
                },
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new TextEncoder().encode('ok'),
                done: true,
              },
            },
          }
        })()
      },
    }

    const resp = await proxyFetch(
      svc,
      new Request('https://example.test/p/test'),
      'client-1',
    )

    expect(resp.status).toBe(200)
    expect(resp.headers.get('content-disposition')).toBeTruthy()
    await expect(resp.text()).resolves.toBe('ok')
  })
})
