import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { downloadURL } from './download.js'

describe('downloadURL', () => {
  const originalCreateObjectURL = URL.createObjectURL
  const originalRevokeObjectURL = URL.revokeObjectURL
  const originalFetch = globalThis.fetch
  const originalDocument = globalThis.document

  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    URL.createObjectURL = originalCreateObjectURL
    URL.revokeObjectURL = originalRevokeObjectURL
    globalThis.fetch = originalFetch
    globalThis.document = originalDocument
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('downloads a fetched response through a blob URL using the server filename', async () => {
    const click = vi.fn()
    const anchor = {
      click,
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    const appendChild = vi.fn()
    const removeChild = vi.fn()
    globalThis.document = {
      body: {
        appendChild,
        removeChild,
      },
      createElement: vi.fn(() => anchor),
    } as unknown as Document
    globalThis.fetch = vi.fn(() => {
      return Promise.resolve(
        new Response(new Blob(['hello'], { type: 'text/plain' }), {
          headers: {
            'content-disposition': 'attachment; filename="server.txt"',
          },
          status: 200,
        }),
      )
    })
    URL.createObjectURL = vi.fn(() => 'blob:download')
    URL.revokeObjectURL = vi.fn()

    await downloadURL('/p/spacewave-core/fs/demo')

    expect(globalThis.fetch).toHaveBeenCalledWith('/p/spacewave-core/fs/demo', {
      cache: 'no-store',
    })
    expect(URL.createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
    expect(anchor.href).toBe('blob:download')
    expect(anchor.download).toBe('server.txt')
    expect(appendChild).toHaveBeenCalledWith(anchor)
    expect(click).toHaveBeenCalledTimes(1)
    expect(removeChild).toHaveBeenCalledWith(anchor)
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()

    vi.advanceTimersByTime(10000)
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:download')
  })

  it('prefers the caller filename over Content-Disposition', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement: vi.fn(() => anchor),
    } as unknown as Document
    globalThis.fetch = vi.fn(() => {
      return Promise.resolve(
        new Response(new Blob(['hello']), {
          headers: {
            'content-disposition': "attachment; filename*=UTF-8''server.txt",
          },
          status: 200,
        }),
      )
    })
    URL.createObjectURL = vi.fn(() => 'blob:explicit')
    URL.revokeObjectURL = vi.fn()

    await downloadURL('/p/spacewave-core/fs/demo', 'explicit.txt')

    expect(anchor.download).toBe('explicit.txt')
  })

  it('keeps direct anchor downloads for cross-origin URLs', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement: vi.fn(() => anchor),
    } as unknown as Document
    globalThis.fetch = vi.fn()

    await downloadURL('https://downloads.example.test/file.zip', 'file.zip')

    expect(globalThis.fetch).not.toHaveBeenCalled()
    expect(anchor.href).toBe('https://downloads.example.test/file.zip')
    expect(anchor.download).toBe('file.zip')
    expect(anchor.click).toHaveBeenCalledTimes(1)
  })

  it('falls back to a direct anchor download when servable same-origin fetch fails', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement: vi.fn(() => anchor),
    } as unknown as Document
    globalThis.fetch = vi.fn(() => Promise.reject(new Error('offline')))

    await downloadURL('/downloads/missing.zip', 'missing.txt')

    expect(globalThis.fetch).toHaveBeenCalledWith('/downloads/missing.zip', {
      cache: 'no-store',
    })
    expect(anchor.href).toBe('/downloads/missing.zip')
    expect(anchor.download).toBe('missing.txt')
    expect(anchor.click).toHaveBeenCalledTimes(1)
  })

  it('rejects failed runtime-internal downloads without falling back to an anchor', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    const createElement = vi.fn(() => anchor)
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement,
    } as unknown as Document
    globalThis.fetch = vi.fn(() => Promise.reject(new Error('offline')))

    await expect(
      downloadURL('/p/spacewave-core/fs/missing', 'missing.txt'),
    ).rejects.toThrow()

    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/p/spacewave-core/fs/missing',
      {
        cache: 'no-store',
      },
    )
    expect(createElement).not.toHaveBeenCalled()
    expect(anchor.click).not.toHaveBeenCalled()
  })

  it('rejects non-ok runtime-internal downloads without falling back to an anchor', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    const createElement = vi.fn(() => anchor)
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement,
    } as unknown as Document
    globalThis.fetch = vi.fn(() => {
      return Promise.resolve(
        new Response('missing', {
          status: 503,
          statusText: 'Service Unavailable',
        }),
      )
    })

    await expect(
      downloadURL('/p/spacewave-core/fs/missing', 'missing.txt'),
    ).rejects.toThrow()

    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/p/spacewave-core/fs/missing',
      {
        cache: 'no-store',
      },
    )
    expect(createElement).not.toHaveBeenCalled()
    expect(anchor.click).not.toHaveBeenCalled()
  })

  it('keeps direct anchor downloads for known-large same-origin responses', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement: vi.fn(() => anchor),
    } as unknown as Document
    globalThis.fetch = vi.fn(() => {
      return Promise.resolve(
        new Response(null, {
          headers: {
            'content-length': String(65 * 1024 * 1024),
          },
          status: 200,
        }),
      )
    })
    URL.createObjectURL = vi.fn(() => 'blob:large')

    await downloadURL('/p/spacewave-core/export/large.zip', 'large.zip')

    expect(URL.createObjectURL).not.toHaveBeenCalled()
    expect(anchor.href).toBe('/p/spacewave-core/export/large.zip')
    expect(anchor.download).toBe('large.zip')
    expect(anchor.click).toHaveBeenCalledTimes(1)
  })

  it('falls back to direct anchor downloads when an unknown-size response exceeds the blob cap', async () => {
    const anchor = {
      click: vi.fn(),
      download: '',
      href: '',
    } as unknown as HTMLAnchorElement
    globalThis.document = {
      body: {
        appendChild: vi.fn(),
        removeChild: vi.fn(),
      },
      createElement: vi.fn(() => anchor),
    } as unknown as Document
    globalThis.fetch = vi.fn(() => {
      return Promise.resolve(
        new Response(
          new ReadableStream({
            start(controller) {
              controller.enqueue(new Uint8Array(65 * 1024 * 1024))
              controller.close()
            },
          }),
          { status: 200 },
        ),
      )
    })
    URL.createObjectURL = vi.fn(() => 'blob:oversized')

    await downloadURL('/p/spacewave-core/export/unknown.zip', 'unknown.zip')

    expect(URL.createObjectURL).not.toHaveBeenCalled()
    expect(anchor.href).toBe('/p/spacewave-core/export/unknown.zip')
    expect(anchor.download).toBe('unknown.zip')
    expect(anchor.click).toHaveBeenCalledTimes(1)
  })
})
