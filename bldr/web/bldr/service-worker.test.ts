import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { proxyFetch } from '../fetch/fetch.js'
import { createEmptyBrowserReleaseState } from './browser-release-state.js'
import type {
  BrowserReleaseDescriptor,
  BrowserReleaseState,
} from './browser-release-state.js'
import {
  classifyBrowserFetchSource,
  classifyBrowserRuntimeFetchError,
  getBrowserControlCacheRow,
  handleBrowserReleaseRequest,
  handleServiceWorkerMessage,
  refreshBrowserIndexCache,
  resetServiceWorkerTestState,
  resolveBrowserRuntimeFetchClientId,
  swFetch,
} from './service-worker.js'

vi.mock('../fetch/fetch.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../fetch/fetch.js')>()
  return {
    ...actual,
    proxyFetch: vi.fn(),
  }
})

class FakeCache {
  private readonly entries = new Map<string, Response>()

  public async match(request: Request): Promise<Response | undefined> {
    return this.entries.get(request.url)?.clone()
  }

  public async put(request: Request, response: Response): Promise<void> {
    this.entries.set(request.url, response.clone())
  }
}

class FakeCacheStorage {
  private readonly caches = new Map<string, FakeCache>()

  public async open(name: string): Promise<FakeCache> {
    const existing = this.caches.get(name)
    if (existing) {
      return existing
    }
    const cache = new FakeCache()
    this.caches.set(name, cache)
    return cache
  }

  public async keys(): Promise<string[]> {
    return Array.from(this.caches.keys())
  }

  public async delete(name: string): Promise<boolean> {
    return this.caches.delete(name)
  }
}

interface FetchEventHarness {
  ev: FetchEvent
  waitUntilPromises: Promise<unknown>[]
}

interface Deferred<T> {
  promise: Promise<T>
  resolve(value: T): void
}

function buildRelease(generationId: string): BrowserReleaseDescriptor {
  return {
    schemaVersion: 1,
    generationId,
    shellAssets: {
      entrypoint: `/entrypoint/${generationId}/entrypoint.mjs`,
      serviceWorker: `/sw-${generationId}.mjs`,
      sharedWorker: `/shw-${generationId}.mjs`,
      wasm: `/entrypoint/${generationId}/runtime.wasm.gz`,
      css: [],
    },
    prerenderedRoutes: ['/'],
    requiredStaticAssets: [],
  }
}

function newDeferred<T>(): Deferred<T> {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

function buildFetchEvent(url: string): FetchEventHarness {
  const waitUntilPromises: Promise<unknown>[] = []
  return {
    ev: {
      request: new Request(url),
      waitUntil(promise: Promise<unknown>) {
        waitUntilPromises.push(promise)
      },
    } as FetchEvent,
    waitUntilPromises,
  }
}

async function writeBrowserReleaseState(
  caches: FakeCacheStorage,
  state: BrowserReleaseState,
): Promise<void> {
  const cache = await caches.open('bldr-control')
  await cache.put(
    new Request(
      new URL('/__bldr/browser-release-state.json', self.location.href),
    ),
    new Response(JSON.stringify(state)),
  )
}

async function writeGenerationCacheResponse(
  caches: FakeCacheStorage,
  generationId: string,
  path: string,
  response: Response,
): Promise<void> {
  const cache = await caches.open(`bldr-generation-${generationId}`)
  await cache.put(new Request(new URL(path, self.location.href)), response)
}

async function writeControlCacheResponse(
  caches: FakeCacheStorage,
  path: string,
  response: Response,
): Promise<void> {
  const cache = await caches.open('bldr-control')
  await cache.put(new Request(new URL(path, self.location.href)), response)
}

function buildMessageEvent(data: unknown): ExtendableMessageEvent {
  return {
    data,
    source: {
      id: 'client-a',
    },
    waitUntil: vi.fn(),
  } as unknown as ExtendableMessageEvent
}

function buildFetchOnlyEvent(
  path: string,
  init?: RequestInit,
  clientId?: string,
): FetchEvent {
  return {
    request: new Request(new URL(path, self.location.href), init),
    clientId,
  } as FetchEvent
}

function buildMalformedFetchEvent(url: string): FetchEvent {
  return {
    request: {
      url,
      method: 'GET',
      headers: new Headers(),
    } as unknown as Request,
  } as FetchEvent
}

describe('service worker browser release requests', () => {
  beforeEach(() => {
    resetServiceWorkerTestState()
    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', new FakeCacheStorage())
    vi.stubGlobal('fetch', vi.fn())
    Object.defineProperty(self, 'clients', {
      configurable: true,
      value: {
        claim: vi.fn(),
        matchAll: vi.fn().mockResolvedValue([]),
      },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('returns a fresh manifest when the network wins within the budget', async () => {
    const cachedRelease = buildRelease('gen-a')
    const freshRelease = buildRelease('gen-b')
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: cachedRelease,
      },
    )
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify(freshRelease), { status: 200 }),
    )
    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(new Response('asset', { status: 200 })),
    )
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    const response = await handleBrowserReleaseRequest(ev)

    expect(await response.json()).toEqual(freshRelease)
    expect(waitUntilPromises).toHaveLength(1)
    await waitUntilPromises[0]
  })

  it('returns the cached manifest when the network misses the budget', async () => {
    vi.useFakeTimers()
    const cachedRelease = buildRelease('gen-a')
    const freshRelease = buildRelease('gen-b')
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: cachedRelease,
      },
    )
    const network = newDeferred<Response>()
    vi.mocked(fetch).mockReturnValueOnce(network.promise)
    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(new Response('asset', { status: 200 })),
    )
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    const responsePromise = handleBrowserReleaseRequest(ev)
    await vi.advanceTimersByTimeAsync(800)
    const response = await responsePromise

    expect(await response.json()).toEqual(cachedRelease)
    expect(waitUntilPromises).toHaveLength(1)

    network.resolve(
      new Response(JSON.stringify(freshRelease), {
        status: 200,
      }),
    )
    await waitUntilPromises[0]

    expect(info).toHaveBeenCalledWith(
      'ServiceWorker: %s: browser release manifest fetch missed %dms budget: latency=%dms',
      expect.any(String),
      800,
      800,
    )
  })

  it('returns the cached manifest when the network is offline', async () => {
    const cachedRelease = buildRelease('gen-a')
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: cachedRelease,
      },
    )
    const pendingFetch = newDeferred<Response>()
    vi.mocked(fetch).mockRejectedValueOnce(new Error('offline'))
    vi.mocked(fetch).mockReturnValue(pendingFetch.promise)
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    const response = await handleBrowserReleaseRequest(ev)

    expect(await response.json()).toEqual(cachedRelease)
    expect(waitUntilPromises).toHaveLength(1)
  })

  it('returns the network manifest when no promoted cache exists', async () => {
    const release = buildRelease('gen-b')
    const pendingFetch = newDeferred<Response>()
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify(release), { status: 200 }),
    )
    vi.mocked(fetch).mockReturnValue(pendingFetch.promise)
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    const response = await handleBrowserReleaseRequest(ev)

    expect(await response.json()).toEqual(release)
    expect(waitUntilPromises).toHaveLength(1)
  })

  it('errors when the network is offline and no cache exists', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('offline'))
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    await expect(handleBrowserReleaseRequest(ev)).rejects.toThrow(
      'browser release manifest unavailable',
    )
    expect(waitUntilPromises).toHaveLength(0)
  })
})

describe('service worker fetch release cache routing', () => {
  beforeEach(() => {
    resetServiceWorkerTestState()
    vi.clearAllMocks()
    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', new FakeCacheStorage())
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('serves promoted generation assets from the generation cache', async () => {
    const release = buildRelease('gen-a')
    const caches = globalThis.caches as unknown as FakeCacheStorage
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: release,
    })
    await writeGenerationCacheResponse(
      caches,
      release.generationId,
      release.shellAssets.wasm,
      new Response('cached wasm', { status: 200 }),
    )
    vi.mocked(fetch).mockResolvedValue(new Response('network', { status: 200 }))

    const response = await swFetch(
      buildFetchOnlyEvent(release.shellAssets.wasm),
    )

    expect(await response.text()).toBe('cached wasm')
    expect(fetch).not.toHaveBeenCalled()
  })

  it('uses native fetch for non-promoted paths', async () => {
    const release = buildRelease('gen-a')
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: release,
      },
    )
    vi.mocked(fetch).mockResolvedValue(new Response('network', { status: 200 }))

    const response = await swFetch(buildFetchOnlyEvent('/other.wasm'))

    expect(await response.text()).toBe('network')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('uses native fetch on promoted generation cache miss', async () => {
    const release = buildRelease('gen-a')
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: release,
      },
    )
    vi.mocked(fetch).mockResolvedValue(new Response('network', { status: 200 }))

    const response = await swFetch(
      buildFetchOnlyEvent(release.shellAssets.wasm),
    )

    expect(await response.text()).toBe('network')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('caches the runtime browser index and falls back to it on refresh failure', async () => {
    vi.mocked(proxyFetch)
      .mockResolvedValueOnce(new Response('runtime index', { status: 200 }))
      .mockResolvedValueOnce(
        new Response('runtime unavailable', { status: 503 }),
      )

    const firstResponse = await swFetch(buildFetchOnlyEvent('/b/__index.html'))

    const secondResponse = await swFetch(buildFetchOnlyEvent('/b/__index.html'))

    expect(await firstResponse.text()).toBe('runtime index')
    expect(await secondResponse.text()).toBe('runtime index')
    expect(proxyFetch).toHaveBeenCalledTimes(2)
  })

  it('uses native fetch for root navigation requests with only the runtime browser index cached', async () => {
    await writeControlCacheResponse(
      globalThis.caches as unknown as FakeCacheStorage,
      '/b/__index.html',
      new Response('runtime index', { status: 200 }),
    )
    vi.mocked(fetch).mockResolvedValue(
      new Response('network root', { status: 200 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent('/', {
        headers: { Accept: 'text/html' },
      }),
    )

    expect(await response.text()).toBe('network root')
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('serves root navigation requests from the cached root response', async () => {
    await writeControlCacheResponse(
      globalThis.caches as unknown as FakeCacheStorage,
      '/',
      new Response('cached root', { status: 200 }),
    )
    vi.mocked(fetch).mockRejectedValue(new Error('network unavailable'))

    const response = await swFetch(
      buildFetchOnlyEvent('/', {
        headers: { Accept: 'text/html' },
      }),
    )

    expect(await response.text()).toBe('cached root')
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('caches successful native root navigation responses', async () => {
    const caches = new FakeCacheStorage()
    vi.stubGlobal('caches', caches)
    vi.mocked(fetch).mockResolvedValue(
      new Response('network root', { status: 200 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent('/', {
        headers: { Accept: 'text/html' },
      }),
    )

    expect(await response.text()).toBe('network root')
    const cache = await caches.open('bldr-control')
    const rootResponse = await cache.match(
      new Request(new URL('/', self.location.href)),
    )
    expect(await rootResponse?.text()).toBe('network root')
  })

  it('uses native fetch for root navigation requests on runtime browser index cache miss', async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response('network root', { status: 200 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent('/', {
        headers: { Accept: 'text/html' },
      }),
    )

    expect(await response.text()).toBe('network root')
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('returns bad request for malformed request URLs', async () => {
    const response = await swFetch(buildMalformedFetchEvent(''))

    expect(response.status).toBe(400)
    expect(await response.text()).toBe('malformed request URL')
    expect(fetch).not.toHaveBeenCalled()
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('classifies browser fetch sources before routing', () => {
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/', self.location.href), {
          headers: { Accept: 'text/html' },
        }),
      ).kind,
    ).toBe('root-document')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/b/__index.html', self.location.href)),
      ).kind,
    ).toBe('browser-index')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/boot.mjs', self.location.href)),
      ).kind,
    ).toBe('boot-asset')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/browser-release.json', self.location.href)),
      ).kind,
    ).toBe('release-asset')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/b/pd/plugin/app.mjs', self.location.href)),
      ).kind,
    ).toBe('plugin-dist')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/b/pa/plugin/style.css', self.location.href)),
      ).kind,
    ).toBe('plugin-assets')
    expect(
      classifyBrowserFetchSource(
        new Request(
          new URL('/p/spacewave-core/fs/file.txt', self.location.href),
        ),
      ).kind,
    ).toBe('plugin-assets')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/b/qjs/qjs-wasi.wasm', self.location.href)),
      ).kind,
    ).toBe('quickjs-runtime-asset')
    expect(
      classifyBrowserFetchSource(
        new Request(new URL('/other.wasm', self.location.href)),
      ).kind,
    ).toBe('native-fetch')
  })

  it('keeps root navigation and browser index in distinct control cache rows', () => {
    const rootRow = getBrowserControlCacheRow('root-document')
    const indexRow = getBrowserControlCacheRow('browser-index')

    expect(rootRow.cacheName).toBe('bldr-control')
    expect(indexRow.cacheName).toBe('bldr-control')
    expect(rootRow.kind).toBe('root-document')
    expect(indexRow.kind).toBe('browser-index')
    expect(rootRow.path).toBe('/')
    expect(indexRow.path).toBe('/b/__index.html')
    expect(rootRow.path).not.toBe(indexRow.path)
  })

  it('serves retained root navigation with the app import map from the root row', async () => {
    await writeControlCacheResponse(
      globalThis.caches as unknown as FakeCacheStorage,
      '/',
      new Response(
        '<script type="importmap">{"imports":{"@spacewave/app":"/entrypoint/gen-a/app.mjs"}}</script>',
        { status: 200 },
      ),
    )
    await writeControlCacheResponse(
      globalThis.caches as unknown as FakeCacheStorage,
      '/b/__index.html',
      new Response(
        '<script type="importmap">{"imports":{"@bldr/runtime":"/b/runtime.mjs"}}</script>',
        { status: 200 },
      ),
    )
    vi.mocked(fetch).mockRejectedValue(new Error('network unavailable'))

    const response = await swFetch(
      buildFetchOnlyEvent('/', {
        headers: { Accept: 'text/html' },
      }),
    )

    const body = await response.text()
    expect(body).toContain('@spacewave/app')
    expect(body).not.toContain('@bldr/runtime')
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('returns typed runtime-unavailable for plugin fetches without a client', async () => {
    const response = await swFetch(
      buildFetchOnlyEvent('/p/spacewave-core/fs/u/1/so/space/-/file.txt'),
    )

    expect(response.status).toBe(503)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Runtime-Fetch-Error')).toBe(
      'runtime-unavailable',
    )
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'runtime-unavailable',
    )
    expect(await response.json()).toMatchObject({
      code: 'runtime-unavailable',
      source: 'plugin-assets',
      pluginAssetFetchResult: 'runtime-unavailable',
    })
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('uses the service worker runtime client for plugin fetches without a browser client when a relay exists', () => {
    const source = classifyBrowserFetchSource(
      new Request(
        new URL(
          '/p/spacewave-core/fs/u/1/so/space/-/file.txt',
          self.location.href,
        ),
      ),
    )

    expect(
      resolveBrowserRuntimeFetchClientId(
        '',
        source,
        { hasRuntimeFetchRelay: () => true },
        'service-worker-runtime',
      ),
    ).toBe('service-worker-runtime')
    expect(
      resolveBrowserRuntimeFetchClientId(
        'client-a',
        source,
        { hasRuntimeFetchRelay: () => true },
        'service-worker-runtime',
      ),
    ).toBe('client-a')
    expect(
      resolveBrowserRuntimeFetchClientId(
        '',
        source,
        { hasRuntimeFetchRelay: () => false },
        'service-worker-runtime',
      ),
    ).toBe('')
  })

  it('returns typed runtime-unavailable for plugin worker import timeouts', async () => {
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(
        'WebRuntimeClient: client-a: timeout opening stream with host',
        {
          status: 500,
        },
      ),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pd/spacewave-app/backend.mjs',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(503)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-dist')
    expect(response.headers.get('X-Bldr-Runtime-Fetch-Error')).toBe(
      'runtime-unavailable',
    )
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'runtime-unavailable',
    )
    expect(await response.json()).toMatchObject({
      code: 'runtime-unavailable',
      source: 'plugin-dist',
      path: '/b/pd/spacewave-app/backend.mjs',
      pluginAssetFetchResult: 'runtime-unavailable',
    })
  })

  it('returns live plugin asset lease state for successful plugin fetches', async () => {
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('export const ok = true', { status: 200 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pd/spacewave-app/backend.mjs',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(200)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-dist')
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'live',
    )
    expect(await response.text()).toBe('export const ok = true')
  })

  it('returns a typed plugin asset missing response for missing plugin assets', async () => {
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('plugin asset missing', { status: 404 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pa/spacewave-app/style.css',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(404)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Runtime-Fetch-Error')).toBe(
      'plugin-asset-missing',
    )
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'missing',
    )
    expect(await response.json()).toMatchObject({
      code: 'plugin-asset-missing',
      pluginAssetFetchResult: 'missing',
    })
  })

  it('returns a typed owner result for stale advertised frontend assets against a newer mount', async () => {
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('404 page not found', { status: 404 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pa/spacewave-app/v/b/fe/app/App-oldhash.mjs',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(404)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Runtime-Fetch-Error')).toBe(
      'plugin-asset-missing',
    )
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'missing',
    )
    expect(await response.json()).toMatchObject({
      code: 'plugin-asset-missing',
      source: 'plugin-assets',
      path: '/b/pa/spacewave-app/v/b/fe/app/App-oldhash.mjs',
      pluginAssetFetchResult: 'missing',
    })
  })

  it('returns a typed plugin asset unavailable response for non-missing failures', async () => {
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('plugin asset unavailable', { status: 503 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pa/spacewave-app/style.css',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(503)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Runtime-Fetch-Error')).toBe(
      'plugin-asset-unavailable',
    )
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'unavailable',
    )
  })

  it('returns a typed generation-closed response for closed plugin asset generations', async () => {
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('WebRuntimeClientInstance is closed', { status: 500 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pa/spacewave-app/style.css',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(410)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Runtime-Fetch-Error')).toBe(
      'generation-closed',
    )
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'generation-closed',
    )
    expect(await response.json()).toMatchObject({
      code: 'generation-closed',
      pluginAssetFetchResult: 'generation-closed',
    })
  })

  it('classifies retained runtime and cancellation failures with bounded codes', () => {
    expect(
      classifyBrowserRuntimeFetchError(
        { kind: 'plugin-assets', path: '/p/spacewave-core/fs/file.txt' },
        { message: 'resume-ready unavailable', status: 503 },
      ),
    ).toMatchObject({
      code: 'runtime-unavailable',
      status: 503,
      pluginAssetFetchResult: 'runtime-unavailable',
    })
    expect(
      classifyBrowserRuntimeFetchError(
        { kind: 'quickjs-runtime-asset', path: '/b/qjs/qjs-wasi.wasm' },
        { message: 'WebRuntimeClientInstance is closed', status: 500 },
      ),
    ).toMatchObject({
      code: 'generation-closed',
      status: 410,
      pluginAssetFetchResult: 'generation-closed',
    })
    expect(
      classifyBrowserRuntimeFetchError(
        { kind: 'plugin-dist', path: '/b/pd/plugin/app.mjs' },
        { message: 'service worker client closed', aborted: true, status: 500 },
      ),
    ).toMatchObject({
      code: 'request-canceled',
      status: 499,
      pluginAssetFetchResult: 'canceled',
    })
    expect(
      classifyBrowserRuntimeFetchError(
        { kind: 'plugin-assets', path: '/b/pa/plugin/missing.css' },
        { message: '404 page not found', status: 404 },
      ),
    ).toMatchObject({
      code: 'plugin-asset-missing',
      status: 404,
      pluginAssetFetchResult: 'missing',
    })
    expect(
      classifyBrowserRuntimeFetchError(
        { kind: 'plugin-assets', path: '/b/pa/plugin/closed.css' },
        { message: '404 not found', status: 404 },
      ),
    ).toMatchObject({
      code: 'plugin-asset-unavailable',
      status: 404,
      pluginAssetFetchResult: 'unavailable',
    })
  })
})

describe('service worker messages', () => {
  beforeEach(() => {
    resetServiceWorkerTestState()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('runs one sync per in-flight window for bldrSyncManifest messages', async () => {
    const firstSync = newDeferred<BrowserReleaseState>()
    const secondSync = newDeferred<BrowserReleaseState>()
    const syncLatestBrowserRelease = vi
      .fn()
      .mockReturnValueOnce(firstSync.promise)
      .mockReturnValueOnce(secondSync.promise)
    const deps = {
      clients: {} as Clients,
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: {
        handleWebDocumentMessage: vi.fn(),
      },
      syncLatestBrowserRelease,
      refreshBrowserIndexCache: vi.fn(),
      handleCrossTabMessage: vi.fn(),
    }

    const firstEv = buildMessageEvent({ bldrSyncManifest: true })
    const duplicateEv = buildMessageEvent({ bldrSyncManifest: true })
    handleServiceWorkerMessage(firstEv, deps)
    handleServiceWorkerMessage(duplicateEv, deps)

    expect(syncLatestBrowserRelease).toHaveBeenCalledTimes(1)
    expect(firstEv.waitUntil).toHaveBeenCalledWith(expect.any(Promise))
    expect(duplicateEv.waitUntil).not.toHaveBeenCalled()
    expect(deps.handleCrossTabMessage).not.toHaveBeenCalled()
    expect(
      deps.webDocumentTracker.handleWebDocumentMessage,
    ).not.toHaveBeenCalled()

    firstSync.resolve(createEmptyBrowserReleaseState())
    await vi.mocked(firstEv.waitUntil).mock.calls[0][0]

    const rearmedEv = buildMessageEvent({ bldrSyncManifest: true })
    handleServiceWorkerMessage(rearmedEv, deps)

    expect(syncLatestBrowserRelease).toHaveBeenCalledTimes(2)
    expect(rearmedEv.waitUntil).toHaveBeenCalledWith(expect.any(Promise))

    secondSync.resolve(createEmptyBrowserReleaseState())
    await vi.mocked(rearmedEv.waitUntil).mock.calls[0][0]
  })

  it('refreshes the runtime browser index cache from a message', async () => {
    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', new FakeCacheStorage())
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('runtime index', { status: 200 }),
    )
    const deps = {
      clients: {} as Clients,
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: {
        handleWebDocumentMessage: vi.fn(),
      },
      syncLatestBrowserRelease: vi.fn(),
      refreshBrowserIndexCache,
      handleCrossTabMessage: vi.fn(),
    }

    const ev = buildMessageEvent({ bldrRefreshBrowserIndex: true })
    handleServiceWorkerMessage(ev, deps)

    expect(ev.waitUntil).toHaveBeenCalledWith(expect.any(Promise))
    await vi.mocked(ev.waitUntil).mock.calls[0][0]

    const cache = await (globalThis.caches as unknown as FakeCacheStorage).open(
      'bldr-control',
    )
    const rootResponse = await cache.match(
      new Request(new URL('/', self.location.href)),
    )
    const indexResponse = await cache.match(
      new Request(new URL('/b/__index.html', self.location.href)),
    )

    expect(rootResponse).toBeUndefined()
    expect(await indexResponse?.text()).toBe('runtime index')
    expect(proxyFetch).toHaveBeenCalledWith(
      expect.anything(),
      expect.any(Request),
      'client-a',
    )
  })

  it('aborts outstanding fetch waiters when a client says goodbye', () => {
    const deps = {
      clients: {} as Clients,
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: {
        handleWebDocumentMessage: vi.fn(),
      },
      syncLatestBrowserRelease: vi.fn(),
      refreshBrowserIndexCache: vi.fn(),
      handleCrossTabMessage: vi.fn().mockResolvedValue(undefined),
    }

    const ev = buildMessageEvent({ crossTab: 'goodbye' })
    handleServiceWorkerMessage(ev, deps)

    expect(deps.fetchTracker.abortClient).toHaveBeenCalledWith(
      'client-a',
      expect.objectContaining({
        message: 'service worker client closed',
      }),
    )
    expect(ev.waitUntil).toHaveBeenCalledWith(expect.any(Promise))
  })

  it('updates only cached browser index content when the browser index cache is refreshed', async () => {
    const caches = new FakeCacheStorage()

    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', caches)
    vi.mocked(proxyFetch)
      .mockResolvedValueOnce(new Response('stale index', { status: 200 }))
      .mockResolvedValueOnce(new Response('fresh index', { status: 200 }))

    await refreshBrowserIndexCache('client-a')
    await refreshBrowserIndexCache('client-a')

    const cache = await caches.open('bldr-control')
    const rootResponse = await cache.match(
      new Request(new URL('/', self.location.href)),
    )
    const indexResponse = await cache.match(
      new Request(new URL('/b/__index.html', self.location.href)),
    )

    expect(rootResponse).toBeUndefined()
    expect(await indexResponse?.text()).toBe('fresh index')
  })
})
