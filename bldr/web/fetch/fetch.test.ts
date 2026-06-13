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
        return {
          [Symbol.asyncIterator]() {
            return {
              next: () =>
                new Promise<IteratorResult<never>>((_, reject) => {
                  signal?.addEventListener(
                    'abort',
                    () => reject(new Error('aborted by owner')),
                    { once: true },
                  )
                }),
            }
          },
        }
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

  it('errors the response body when the stream ends before the done packet', async () => {
    const svc: FetchService = {
      Fetch() {
        return (async function* () {
          yield {
            body: {
              case: 'responseInfo',
              value: {
                status: 200,
                statusText: 'OK',
                headers: { 'content-type': 'text/javascript' },
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new TextEncoder().encode('partial module body'),
                done: false,
              },
            },
          }
          // iterator ends without a final done packet: truncated body
        })()
      },
    }

    const resp = await proxyFetch(
      svc,
      new Request('https://example.test/b/pa/app/module.mjs'),
      'client-1',
    )

    expect(resp.status).toBe(200)
    await expect(resp.text()).rejects.toThrow(
      'fetch response stream ended before the final done packet',
    )
  })

  it('reads a complete Sonner-sized module response through the proxy stream', async () => {
    const moduleTail = 'export const toast = "sonner";\n'
      .repeat(3)
      .padEnd(105, '/')
    const body = `${'x'.repeat(32 * 1024)}${moduleTail}`
    expect(new TextEncoder().encode(body)).toHaveLength(32873)
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
                  'content-length': String(body.length),
                  'content-type': 'text/javascript; charset=utf-8',
                },
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new TextEncoder().encode(body.slice(0, 32 * 1024)),
                done: false,
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new TextEncoder().encode(body.slice(32 * 1024)),
                done: false,
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new Uint8Array(),
                done: true,
              },
            },
          }
        })()
      },
    }

    const resp = await proxyFetch(
      svc,
      new Request('https://example.test/b/pkg/sonner/dist/index.mjs'),
      'client-1',
    )

    expect(resp.status).toBe(200)
    expect(resp.headers.get('content-length')).toBe(String(body.length))
    await expect(resp.text()).resolves.toBe(body)
  })

  it('errors a Sonner-sized module response when the transport closes after headers and body bytes', async () => {
    const moduleTail = 'export const toast = "sonner";\n'
      .repeat(3)
      .padEnd(105, '/')
    const body = `${'x'.repeat(32 * 1024)}${moduleTail}`
    expect(new TextEncoder().encode(body)).toHaveLength(32873)
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
                  'content-length': String(body.length),
                  'content-type': 'text/javascript; charset=utf-8',
                },
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new TextEncoder().encode(body.slice(0, 32 * 1024)),
                done: false,
              },
            },
          }
          yield {
            body: {
              case: 'responseData',
              value: {
                data: new TextEncoder().encode(body.slice(32 * 1024)),
                done: false,
              },
            },
          }
        })()
      },
    }

    const resp = await proxyFetch(
      svc,
      new Request('https://example.test/b/pkg/sonner/dist/index.mjs'),
      'client-1',
    )

    expect(resp.status).toBe(200)
    await expect(resp.text()).rejects.toThrow(
      'fetch response stream ended before the final done packet',
    )
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
