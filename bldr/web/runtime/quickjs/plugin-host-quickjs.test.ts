import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  addAssetToFileSystem,
  canUseSynchronousBackendAssetFetch,
  collectBackendEntrypointAssetPaths,
  collectViteManifestStaticAssetPaths,
  createBackendAssetMount,
  createBackendAssetPreopens,
  loadBackendAssets,
  resolveBackendAssetPath,
  selectBackendAssetLoadingMode,
  shouldPreloadBackendAssets,
  type BackendAssetCacheEntry,
} from './plugin-host-quickjs.js'

describe('plugin-host-quickjs asset helpers', () => {
  const originalXMLHttpRequest = globalThis.XMLHttpRequest
  const originalFetch = globalThis.fetch
  const api = {
    startInfo: { pluginId: 'notes' },
    utils: {
      pluginAssetHttpPath(pluginId: string, path: string) {
        return `/asset/${pluginId}/${path}`
      },
    },
  }

  afterEach(() => {
    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: originalXMLHttpRequest,
      configurable: true,
      writable: true,
    })
    Object.defineProperty(globalThis, 'fetch', {
      value: originalFetch,
      configurable: true,
      writable: true,
    })
    vi.restoreAllMocks()
  })

  it('collects bounded static vite manifest asset paths for backend entrypoints', () => {
    const paths = collectViteManifestStaticAssetPaths(
      {
        'plugin/notes/backend.ts': {
          file: 'plugin/notes/backend-abc123.mjs',
          imports: ['_chunk-shared-1.mjs'],
          dynamicImports: ['_chunk-lazy-2.mjs'],
          css: ['assets/backend.css'],
          assets: ['assets/icon.svg'],
        },
        '_chunk-shared-1.mjs': {
          file: 'chunks/shared-1.mjs',
        },
        '_chunk-lazy-2.mjs': {
          file: 'chunks/lazy-2.mjs',
        },
        'plugin/vm/backend.ts': {
          file: 'plugin/vm/backend-def456.mjs',
        },
      },
      ['/assets/v/b/be/plugin/notes/backend-abc123.mjs'],
    )

    expect(paths).toEqual([
      'plugin/notes/backend-abc123.mjs',
      'assets/backend.css',
      'assets/icon.svg',
      'chunks/shared-1.mjs',
    ])
  })

  it('normalizes backend asset paths into the v/b/be tree', () => {
    expect(resolveBackendAssetPath('plugin/notes/backend-abc123.mjs')).toBe(
      'v/b/be/plugin/notes/backend-abc123.mjs',
    )
    expect(
      resolveBackendAssetPath('b/be/plugin/notes/backend-abc123.mjs'),
    ).toBe('v/b/be/plugin/notes/backend-abc123.mjs')
    expect(
      resolveBackendAssetPath('v/b/be/plugin/notes/backend-abc123.mjs'),
    ).toBe('v/b/be/plugin/notes/backend-abc123.mjs')
  })

  it('extracts backend entrypoint asset paths from the plugin wrapper', () => {
    const paths = collectBackendEntrypointAssetPaths(`
      const backendEntrypoints = [
        { importPath: "/assets/v/b/be/plugin/notes/backend-abc123.mjs" },
        { importPath: '/assets/v/b/be/plugin/notes/backend-abc123.mjs' },
        { importPath: "/assets/v/b/fe/plugin/notes/App-def456.mjs" },
      ]
    `)

    expect(paths).toEqual(['/assets/v/b/be/plugin/notes/backend-abc123.mjs'])
  })

  it('preloads backend assets from wrapper imports without treating /b/pd as an asset', async () => {
    const requests: string[] = []
    const manifest = JSON.stringify({
      'plugin/notes/backend.ts': {
        file: 'plugin/notes/backend-abc123.mjs',
        imports: ['_chunk-shared-1.mjs'],
        css: ['assets/backend.css'],
      },
      '_chunk-shared-1.mjs': {
        file: 'chunks/shared-1.mjs',
      },
    })
    const bodies = new Map<string, string>([
      ['/asset/notes/v/b/be/.vite/manifest.json', manifest],
      [
        '/asset/notes/v/b/be/plugin/notes/backend-abc123.mjs',
        'export default function backend() {}',
      ],
      ['/asset/notes/v/b/be/chunks/shared-1.mjs', 'export const shared = true'],
      ['/asset/notes/v/b/be/backend.css', '.backend{}'],
    ])

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        requests.push(url)
        const body = bodies.get(url)
        if (body == null) {
          return new Response('missing', { status: 404 })
        }
        return new Response(body, { status: 200 })
      }),
    )

    const files = new Map<string, string | Uint8Array>()
    const cache = new Map<string, BackendAssetCacheEntry>()
    const loaded = await loadBackendAssets(
      api,
      new AbortController().signal,
      files,
      collectBackendEntrypointAssetPaths(
        'import("/assets/v/b/be/plugin/notes/backend-abc123.mjs")',
      ),
      cache,
    )

    expect(loaded).toBe(true)
    expect(requests).toEqual([
      '/asset/notes/v/b/be/.vite/manifest.json',
      '/asset/notes/v/b/be/plugin/notes/backend-abc123.mjs',
      '/asset/notes/v/b/be/backend.css',
      '/asset/notes/v/b/be/chunks/shared-1.mjs',
    ])
    expect(requests.some((url) => url.includes('/v/b/pd/'))).toBe(false)
    expect(files.has('v/b/be/plugin/notes/backend-abc123.mjs')).toBe(true)
    expect(files.has('v/b/be/chunks/shared-1.mjs')).toBe(true)
    expect(cache.get('v/b/be/plugin/notes/backend-abc123.mjs')?.ok).toBe(true)
    expect(cache.get('v/b/be/chunks/shared-1.mjs')?.ok).toBe(true)
  })

  it('does not fall back to whole-manifest preload without backend entrypoints', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const files = new Map<string, string | Uint8Array>()
    const loaded = await loadBackendAssets(
      api,
      new AbortController().signal,
      files,
      [],
    )

    expect(loaded).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(files.size).toBe(0)
  })

  it('mirrors assets under both asset-relative and /assets paths', () => {
    const files = new Map<string, string | Uint8Array>()

    addAssetToFileSystem(
      files,
      'plugin/notes/backend-abc123.mjs',
      'export default {}',
    )

    expect(files.get('v/b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'export default {}',
    )
    expect(files.get('/assets/v/b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'export default {}',
    )
  })

  it('lazily fetches backend assets through synchronous XHR and caches reads', () => {
    const requests: string[] = []
    const enc = new TextEncoder()

    class MockXMLHttpRequest {
      status = 0
      response: ArrayBuffer | null = null
      responseText = ''
      responseType = ''
      private url = ''

      open(_method: string, url: string, async: boolean) {
        expect(async).toBe(false)
        this.url = url
      }

      send() {
        requests.push(this.url)
        if (this.url.endsWith('/v/b/be/plugin/app.mjs')) {
          const data = enc.encode('export const ok = true')
          this.status = 200
          this.response = data.buffer
          return
        }
        this.status = 404
      }
    }

    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    })

    expect(canUseSynchronousBackendAssetFetch()).toBe(true)
    const mount = createBackendAssetMount(api, new AbortController().signal)
    expect(mount).not.toBeNull()
    expect(requests).toEqual([])

    const file = mount?.getFile('v/b/be/plugin/app.mjs')
    expect(new TextDecoder().decode(file?.readAt(0n, 64))).toBe(
      'export const ok = true',
    )
    expect(mount?.getFile('v/b/be/plugin/app.mjs')?.size).toBe(22n)
    expect(mount?.getFile('v/b/be/plugin/missing.mjs')).toBeNull()
    expect(mount?.getFile('v/b/be/plugin/missing.mjs')).toBeNull()
    expect(requests).toEqual([
      '/asset/notes/v/b/be/plugin/app.mjs',
      '/asset/notes/v/b/be/plugin/missing.mjs',
    ])
  })

  it('selects lazy backend asset loading when sync XHR is available', () => {
    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: undefined,
      configurable: true,
      writable: true,
    })
    expect(canUseSynchronousBackendAssetFetch()).toBe(false)
    expect(selectBackendAssetLoadingMode()).toBe('bounded-preload')

    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: class MockXMLHttpRequest {},
      configurable: true,
      writable: true,
    })
    expect(canUseSynchronousBackendAssetFetch()).toBe(true)
    expect(selectBackendAssetLoadingMode()).toBe('lazy-http')
  })

  it('preloads backend assets only for bounded fallback mode', () => {
    const entrypoints = ['/assets/v/b/be/plugin/app.mjs']

    expect(shouldPreloadBackendAssets('lazy-http', entrypoints)).toBe(false)
    expect(shouldPreloadBackendAssets('bounded-preload', entrypoints)).toBe(
      true,
    )
    expect(shouldPreloadBackendAssets('bounded-preload', [])).toBe(false)
  })

  it('serves compiler-emitted backend import paths from lazy preopens', () => {
    const requests: string[] = []
    const enc = new TextEncoder()

    class MockXMLHttpRequest {
      status = 0
      response: ArrayBuffer | null = null
      responseText = ''
      responseType = ''
      private url = ''

      open(_method: string, url: string, async: boolean) {
        expect(async).toBe(false)
        this.url = url
      }

      send() {
        requests.push(this.url)
        const data = enc.encode('export const path = true')
        this.status = 200
        this.response = data.buffer
      }
    }

    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    })

    const preopens = createBackendAssetPreopens(
      api,
      new AbortController().signal,
    )

    const assetsOpen = preopens[0].path_open(
      0,
      'v/b/be/plugin/app.mjs',
      0,
      0n,
      0n,
      0,
    )
    const rootVOpen = preopens[1].path_open(
      0,
      'b/be/plugin/app.mjs',
      0,
      0n,
      0n,
      0,
    )

    expect(assetsOpen.ret).toBe(0)
    expect(rootVOpen.ret).toBe(0)
    expect(new TextDecoder().decode(assetsOpen.fd_obj?.fd_read(64).data)).toBe(
      'export const path = true',
    )
    expect(new TextDecoder().decode(rootVOpen.fd_obj?.fd_read(64).data)).toBe(
      'export const path = true',
    )
    expect(requests).toEqual(['/asset/notes/v/b/be/plugin/app.mjs'])
  })

  it('serves warmed backend assets from lazy preopens without sync XHR', () => {
    const enc = new TextEncoder()
    const cache = new Map<string, BackendAssetCacheEntry>([
      [
        'v/b/be/plugin/app.mjs',
        { ok: true, data: enc.encode('export const warmed = true') },
      ],
    ])

    class MockXMLHttpRequest {
      open() {}

      send() {
        throw new Error('unexpected XHR')
      }
    }

    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    })

    const preopens = createBackendAssetPreopens(
      api,
      new AbortController().signal,
      cache,
    )

    const assetsOpen = preopens[0].path_open(
      0,
      'v/b/be/plugin/app.mjs',
      0,
      0n,
      0n,
      0,
    )
    const rootVOpen = preopens[1].path_open(
      0,
      'b/be/plugin/app.mjs',
      0,
      0n,
      0n,
      0,
    )

    expect(assetsOpen.ret).toBe(0)
    expect(rootVOpen.ret).toBe(0)
    expect(new TextDecoder().decode(assetsOpen.fd_obj?.fd_read(64).data)).toBe(
      'export const warmed = true',
    )
    expect(new TextDecoder().decode(rootVOpen.fd_obj?.fd_read(64).data)).toBe(
      'export const warmed = true',
    )
  })

  it('surfaces lazy backend asset failures without whole-manifest fallback', () => {
    class MockXMLHttpRequest {
      status = 503
      response: ArrayBuffer | null = null
      responseText = ''
      responseType = ''

      open(_method: string, _url: string, async: boolean) {
        expect(async).toBe(false)
      }

      send() {}
    }

    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    })

    const mount = createBackendAssetMount(api, new AbortController().signal)
    expect(() => mount?.getFile('v/b/be/plugin/app.mjs')).toThrow(
      'Failed to fetch backend asset /asset/notes/v/b/be/plugin/app.mjs: 503',
    )
  })
})
