import { afterEach, describe, expect, it, vi } from 'vitest'

import { fetchWithDecompress } from './fetch-decompress.js'

async function gzipText(text: string): Promise<ArrayBuffer> {
  const stream = new Blob([text])
    .stream()
    .pipeThrough(new CompressionStream('gzip'))
  return new Response(stream).arrayBuffer()
}

describe('fetchWithDecompress', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('decompresses gzipped assets when the browser has not decoded them', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(await gzipText('wasm-bytes'), {
        status: 200,
        headers: { 'content-type': 'application/wasm' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const response = await fetchWithDecompress('/entrypoint/runtime.wasm.gz')

    expect(fetchMock).toHaveBeenCalledWith('/entrypoint/runtime.wasm.gz', {
      method: 'GET',
      cache: 'force-cache',
    })
    await expect(response.text()).resolves.toBe('wasm-bytes')
  })

  it('does not decompress responses already decoded by fetch', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response('decoded wasm', {
          status: 200,
          headers: {
            'content-encoding': 'gzip',
            'content-type': 'application/wasm',
          },
        }),
      ),
    )

    const response = await fetchWithDecompress('/entrypoint/runtime.wasm.gz')

    await expect(response.text()).resolves.toBe('decoded wasm')
  })

  it('decompresses gzip bodies even when the response advertises gzip encoding', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(await gzipText('wasm-bytes'), {
          status: 200,
          headers: {
            'content-encoding': 'gzip',
            'content-type': 'application/wasm',
          },
        }),
      ),
    )

    const response = await fetchWithDecompress('/entrypoint/runtime.wasm.gz')

    expect(response.headers.get('content-encoding')).toBeNull()
    await expect(response.text()).resolves.toBe('wasm-bytes')
  })
})
