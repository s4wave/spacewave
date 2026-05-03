import { afterEach, describe, expect, it } from 'vitest'

import {
  addAssetToFileSystem,
  canUseSynchronousBackendAssetFetch,
  collectViteManifestAssetPaths,
  collectViteManifestStaticAssetPaths,
  createBackendAssetMount,
  createBackendAssetPreopens,
  resolveBackendAssetPath,
  selectBackendAssetLoadingMode,
} from './plugin-host-quickjs.js'

describe('plugin-host-quickjs asset helpers', () => {
  const originalXMLHttpRequest = globalThis.XMLHttpRequest
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
  })

  it('collects unique vite manifest asset paths across entry fields', () => {
    const paths = collectViteManifestAssetPaths({
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
        imports: ['_chunk-shared-1.mjs'],
      },
    })

    expect(paths).toEqual([
      'plugin/notes/backend-abc123.mjs',
      'assets/backend.css',
      'assets/icon.svg',
      'chunks/shared-1.mjs',
      'chunks/lazy-2.mjs',
      'plugin/vm/backend-def456.mjs',
    ])
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
    expect(resolveBackendAssetPath('b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'v/b/be/plugin/notes/backend-abc123.mjs',
    )
    expect(resolveBackendAssetPath('v/b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'v/b/be/plugin/notes/backend-abc123.mjs',
    )
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

  it('selects lazy or bounded backend asset loading from sync XHR availability', () => {
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
