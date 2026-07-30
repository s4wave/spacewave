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
  syncLatestBrowserRelease,
  swFetch,
} from './service-worker.js'
import type { OpenWebRuntimePortResult } from './web-document-tracker.js'

vi.mock('../fetch/fetch.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../fetch/fetch.js')>()
  return {
    ...actual,
    proxyFetch: vi.fn(),
  }
})

type FakeCachePutFailure = (
  cacheName: string,
  request: Request,
  response: Response,
) => Error | undefined
type FakeCacheDeleteFailure = (cacheName: string) => Error | undefined

class FakeCache {
  private readonly entries = new Map<string, Response>()

  public constructor(
    private readonly name: string,
    private readonly failPut?: FakeCachePutFailure,
  ) {}

  public async match(request: Request): Promise<Response | undefined> {
    return this.entries.get(request.url)?.clone()
  }

  public async put(request: Request, response: Response): Promise<void> {
    const error = this.failPut?.(this.name, request, response)
    if (error) {
      throw error
    }
    this.entries.set(request.url, response.clone())
  }
}

class FakeCacheStorage {
  private readonly caches = new Map<string, FakeCache>()

  public constructor(
    private readonly failPut?: FakeCachePutFailure,
    private readonly failDelete?: FakeCacheDeleteFailure,
  ) {}

  public async open(name: string): Promise<FakeCache> {
    const existing = this.caches.get(name)
    if (existing) {
      return existing
    }
    const cache = new FakeCache(name, this.failPut)
    this.caches.set(name, cache)
    return cache
  }

  public async keys(): Promise<string[]> {
    return Array.from(this.caches.keys())
  }

  public async delete(name: string): Promise<boolean> {
    const error = this.failDelete?.(name)
    if (error) {
      throw error
    }
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

function newCachePutError(): Error {
  const error = new Error('Cache.put() encountered a network error')
  error.name = 'NetworkError'
  return error
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

function buildMessageDeps() {
  return {
    clients: buildTestClients(),
    fetchTracker: {
      abortClient: vi.fn(),
    },
    webDocumentTracker: {
      handleWebDocumentMessage: vi.fn(),
    },
    syncLatestBrowserRelease: vi.fn(),
    refreshBrowserIndexCache: vi.fn(),
    handleCrossTabMessage: vi.fn(),
  }
}

async function announcePluginRoot(
  pluginId: string,
  rootHash: string,
): Promise<void> {
  const ev = buildMessageEvent({
    bldrPluginManifestRoot: { pluginId, rootHash },
  })
  handleServiceWorkerMessage(ev, buildMessageDeps())
  expect(ev.waitUntil).toHaveBeenCalledWith(expect.any(Promise))
  await vi.mocked(ev.waitUntil).mock.calls[0][0]
}

function buildTestClients(): Clients {
  return {
    claim: () => Promise.resolve(),
    get: () => Promise.resolve(undefined),
    matchAll: () => Promise.resolve([]),
    openWindow: () => Promise.resolve(null),
  }
}

function buildFetchOnlyEvent(
  path: string,
  init?: RequestInit,
  clientId?: string,
): FetchEvent {
  return {
    request: new Request(new URL(path, self.location.href), init),
    clientId,
    waitUntil() {},
  } as unknown as FetchEvent
}

function buildExplicitCacheModeFetchEvent(
  path: string,
  cacheMode: RequestCache,
): FetchEvent {
  const ev = buildFetchOnlyEvent(path)
  Object.defineProperty(ev.request, 'cache', {
    configurable: true,
    value: cacheMode,
  })
  return ev
}

function buildClientFetchEvent(
  path: string,
  clientId: string,
): FetchEventHarness {
  const waitUntilPromises: Promise<unknown>[] = []
  return {
    ev: {
      request: new Request(new URL(path, self.location.href)),
      clientId,
      waitUntil(promise: Promise<unknown>) {
        waitUntilPromises.push(promise)
      },
    } as unknown as FetchEvent,
    waitUntilPromises,
  }
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

function requireShellWasm(release: BrowserReleaseDescriptor): string {
  const { wasm } = release.shellAssets
  if (!wasm) {
    throw new Error('test release missing wasm shell asset')
  }
  return wasm
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

  it('does not promote a browser release when generation cache writes fail', async () => {
    const caches = new FakeCacheStorage((cacheName) => {
      if (cacheName.startsWith('bldr-generation-')) {
        return newCachePutError()
      }
      return undefined
    })
    vi.stubGlobal('caches', caches)
    const release = buildRelease('gen-b')
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify(release), { status: 200 }),
    )
    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(new Response('asset', { status: 200 })),
    )
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    const response = await handleBrowserReleaseRequest(ev)

    expect(await response.json()).toEqual(release)
    expect(waitUntilPromises).toHaveLength(1)
    await expect(waitUntilPromises[0]).resolves.toBeUndefined()

    const cache = await caches.open('bldr-control')
    const stateResponse = await cache.match(
      new Request(
        new URL('/__bldr/browser-release-state.json', self.location.href),
      ),
    )
    if (!stateResponse) {
      throw new Error('browser release state was not written')
    }
    await expect(stateResponse.json()).resolves.toMatchObject({
      discovered: {
        generationId: 'gen-b',
      },
      staged: null,
      promotedCurrent: null,
    })
    expect(warn).toHaveBeenCalledWith(
      'ServiceWorker: %s: cache write failed: operation=%s cache=%s%s%s url=%s: %s',
      expect.any(String),
      'stage browser release',
      'bldr-generation-gen-b',
      ' generation=gen-b',
      '',
      expect.any(String),
      'Cache.put() encountered a network error',
    )
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

  it('budgets lifecycle release sync probes and keeps direct manifest refresh strong', async () => {
    vi.useFakeTimers()
    const release = buildRelease('gen-a')
    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL) => {
      const rawURL = input instanceof Request ? input.url : String(input)
      const pathname = new URL(rawURL, self.location.href).pathname
      if (pathname === '/browser-release.json') {
        return Promise.resolve(
          new Response(JSON.stringify(release), { status: 200 }),
        )
      }
      return Promise.resolve(new Response('asset', { status: 200 }))
    })

    await syncLatestBrowserRelease({ lifecycleProbe: true })
    await syncLatestBrowserRelease({ lifecycleProbe: true })

    const freshBudgetPaths = vi.mocked(fetch).mock.calls.map(([input]) => {
      const rawURL = input instanceof Request ? input.url : String(input)
      return new URL(rawURL, self.location.href).pathname
    })
    expect(
      freshBudgetPaths.filter((path) => path === '/boot.mjs'),
    ).toHaveLength(1)
    expect(
      freshBudgetPaths.filter((path) => path === '/browser-release.json'),
    ).toHaveLength(1)

    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )
    const response = await handleBrowserReleaseRequest(ev)
    expect(await response.json()).toEqual(release)
    expect(waitUntilPromises).toHaveLength(1)
    await waitUntilPromises[0]

    vi.advanceTimersByTime(30000)
    await syncLatestBrowserRelease({ lifecycleProbe: true })

    const allPaths = vi.mocked(fetch).mock.calls.map(([input]) => {
      const rawURL = input instanceof Request ? input.url : String(input)
      return new URL(rawURL, self.location.href).pathname
    })
    expect(allPaths.filter((path) => path === '/boot.mjs')).toHaveLength(3)
    expect(
      allPaths.filter((path) => path === '/browser-release.json'),
    ).toHaveLength(3)
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
    const wasm = requireShellWasm(release)
    const caches = globalThis.caches as unknown as FakeCacheStorage
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: release,
    })
    await writeGenerationCacheResponse(
      caches,
      release.generationId,
      wasm,
      new Response('cached wasm', { status: 200 }),
    )
    vi.mocked(fetch).mockResolvedValue(new Response('network', { status: 200 }))

    const response = await swFetch(buildFetchOnlyEvent(wasm))

    expect(await response.text()).toBe('cached wasm')
    expect(fetch).not.toHaveBeenCalled()
  })

  it('uses native fetch for explicit cache refresh requests to promoted generation assets', async () => {
    const release = buildRelease('gen-a')
    const wasm = requireShellWasm(release)
    const caches = globalThis.caches as unknown as FakeCacheStorage
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: release,
    })
    await writeGenerationCacheResponse(
      caches,
      release.generationId,
      wasm,
      new Response('stale cached wasm', { status: 200 }),
    )
    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(new Response('fresh network wasm', { status: 200 })),
    )

    for (const cacheMode of ['reload', 'no-cache'] as const) {
      const response = await swFetch(
        buildExplicitCacheModeFetchEvent(wasm, cacheMode),
      )

      expect(await response.text()).toBe('fresh network wasm')
    }
    expect(fetch).toHaveBeenCalledTimes(2)
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
    const wasm = requireShellWasm(release)
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: release,
      },
    )
    vi.mocked(fetch).mockResolvedValue(new Response('network', { status: 200 }))

    const response = await swFetch(buildFetchOnlyEvent(wasm))

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
        new Request(
          new URL('/b/pkg/sonner/dist/index.mjs', self.location.href),
        ),
      ).kind,
    ).toBe('plugin-assets')
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

  it('uses the service worker runtime client for static plugin assets when a relay exists', () => {
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

    const assetSource = classifyBrowserFetchSource(
      new Request(
        new URL(
          '/b/pa/spacewave-app/v/b/fe/app/App-next.mjs',
          self.location.href,
        ),
      ),
    )

    expect(
      resolveBrowserRuntimeFetchClientId(
        'client-a',
        assetSource,
        { hasRuntimeFetchRelay: () => true },
        'service-worker-runtime',
      ),
    ).toBe('service-worker-runtime')
    expect(
      resolveBrowserRuntimeFetchClientId(
        'client-a',
        assetSource,
        { hasRuntimeFetchRelay: () => false },
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

  it('preserves successful web package module bodies through runtime fetch', async () => {
    const body = `${'x'.repeat(32 * 1024)}export const toast = "sonner"\n`
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: {
          'Content-Length': String(body.length),
          'Content-Type': 'application/javascript',
        },
      }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pkg/sonner/dist/index.mjs',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(200)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'live',
    )
    expect(response.headers.get('Content-Length')).toBe(String(body.length))
    expect(await response.text()).toBe(body)
  })

  it('preserves successful frontend asset module bodies through runtime fetch', async () => {
    const body = `${'x'.repeat(32 * 1024)}const App = () => null\nexport { App as default }\n`
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: {
          'Content-Length': String(body.length),
          'Content-Type': 'application/javascript',
        },
      }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(
        '/b/pa/spacewave-app/v/b/fe/app/App-livehash.mjs',
        undefined,
        'client-a',
      ),
    )

    expect(response.status).toBe(200)
    expect(response.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe(
      'live',
    )
    expect(response.headers.get('Content-Length')).toBe(String(body.length))
    expect(await response.text()).toBe(body)
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
        { kind: 'plugin-dist', path: '/b/pd/plugin/app.mjs' },
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

  it('caches a successful static plugin asset and serves it across a relay gap', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: buildRelease('gen-a'),
    })
    await announcePluginRoot('spacewave-app', '2abc')
    const body = 'export const App2 = () => null\n'
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'application/javascript' },
      }),
    )

    const warm = buildClientFetchEvent(
      '/b/pa/spacewave-app/v/b/fe/app/App2.mjs',
      'client-a',
    )
    const warmResponse = await swFetch(warm.ev)
    expect(warmResponse.status).toBe(200)
    expect(await warmResponse.text()).toBe(body)
    await Promise.all(warm.waitUntilPromises)
    const generationCache = await caches.open('bldr-generation-gen-a')
    const cached = await generationCache.match(
      new Request(
        new URL('/b/pa/spacewave-app/v/b/fe/app/App2.mjs', self.location.href),
      ),
    )
    expect(await cached?.text()).toBe(body)

    // Relay gap: no client id and no relay, so no runtime fetch can be issued.
    // The cached asset must serve instead of failing the lazy chunk import.
    vi.mocked(proxyFetch).mockClear()
    const gapResponse = await swFetch(
      buildFetchOnlyEvent('/b/pa/spacewave-app/v/b/fe/app/App2.mjs'),
    )
    expect(gapResponse.status).toBe(200)
    expect(await gapResponse.text()).toBe(body)
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('serves a current-generation static hit before runtime and revalidates it', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const release = buildRelease('gen-a')
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: release,
    })
    await announcePluginRoot('spacewave-app', '2abc')
    const path = '/b/pd/spacewave-app/backend.mjs'
    await writeGenerationCacheResponse(
      caches,
      release.generationId,
      path,
      new Response('cached app', { status: 200 }),
    )
    const revalidation = newDeferred<Response>()
    vi.mocked(proxyFetch).mockReturnValue(revalidation.promise)

    const fetchEvent = buildClientFetchEvent(path, 'client-a')
    const response = await swFetch(fetchEvent.ev)

    expect(await response.text()).toBe('cached app')
    expect(response.headers.get('X-Bldr-Plugin-Asset-Cache')).toBe('generation')
    expect(proxyFetch).toHaveBeenCalledOnce()
    expect(fetchEvent.waitUntilPromises).toHaveLength(1)
    revalidation.resolve(new Response('fresh app', { status: 200 }))
    await fetchEvent.waitUntilPromises[0]

    const cache = await caches.open('bldr-generation-gen-a')
    const cached = await cache.match(
      new Request(new URL(path, self.location.href)),
    )
    expect(await cached?.text()).toBe('fresh app')
  })

  it('rejects a cache hit when promotion changes between resolve and match', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const oldRelease = buildRelease('gen-a')
    const currentRelease = buildRelease('gen-b')
    const path = '/b/pd/spacewave-app/backend.mjs'
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: oldRelease,
    })
    await announcePluginRoot('spacewave-app', '2abc')
    await writeGenerationCacheResponse(
      caches,
      oldRelease.generationId,
      path,
      new Response('old generation', { status: 200 }),
    )
    const controlCache = await caches.open('bldr-control')
    const originalMatch = controlCache.match.bind(controlCache)
    let releaseStateReads = 0
    vi.spyOn(controlCache, 'match').mockImplementation(async (request) => {
      if (
        new URL(request.url).pathname !== '/__bldr/browser-release-state.json'
      ) {
        return originalMatch(request)
      }
      releaseStateReads++
      return new Response(
        JSON.stringify({
          ...createEmptyBrowserReleaseState(),
          promotedCurrent:
            releaseStateReads === 1 ? oldRelease : currentRelease,
        }),
      )
    })
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('current generation', { status: 200 }),
    )

    const fetchEvent = buildClientFetchEvent(path, 'client-a')
    const response = await swFetch(fetchEvent.ev)

    expect(await response.text()).toBe('current generation')
    expect(releaseStateReads).toBeGreaterThanOrEqual(2)
    expect(proxyFetch).toHaveBeenCalledOnce()
    await Promise.all(fetchEvent.waitUntilPromises)
  })

  it('does not cache a response after its resolved generation is replaced', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const oldRelease = buildRelease('gen-a')
    const currentRelease = buildRelease('gen-b')
    const path = '/b/pd/spacewave-app/backend.mjs'
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: oldRelease,
    })
    await announcePluginRoot('spacewave-app', '2abc')
    const runtimeResponse = newDeferred<Response>()
    vi.mocked(proxyFetch).mockReturnValue(runtimeResponse.promise)

    const fetchEvent = buildClientFetchEvent(path, 'client-a')
    const responsePromise = swFetch(fetchEvent.ev)
    await vi.waitFor(() => expect(proxyFetch).toHaveBeenCalledOnce())
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: currentRelease,
    })
    runtimeResponse.resolve(
      new Response('old generation response', { status: 200 }),
    )

    const response = await responsePromise
    expect(await response.text()).toBe('old generation response')
    await Promise.all(fetchEvent.waitUntilPromises)
    const oldCache = await caches.open('bldr-generation-gen-a')
    const cached = await oldCache.match(
      new Request(new URL(path, self.location.href)),
    )
    expect(cached).toBeUndefined()
  })

  it('does not serve a static asset from a previous promoted generation', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const oldRelease = buildRelease('gen-a')
    const currentRelease = buildRelease('gen-b')
    const path = '/b/pa/spacewave-app/v/b/fe/app/App.mjs'
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: oldRelease,
    })
    await announcePluginRoot('spacewave-app', '2abc')
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('old generation', { status: 200 }),
    )

    const warm = buildClientFetchEvent(path, 'client-a')
    const warmResponse = await swFetch(warm.ev)
    expect(await warmResponse.text()).toBe('old generation')
    await Promise.all(warm.waitUntilPromises)

    vi.mocked(proxyFetch).mockClear()
    const cachedOldResponse = await swFetch(buildFetchOnlyEvent(path))
    expect(await cachedOldResponse.text()).toBe('old generation')
    expect(proxyFetch).not.toHaveBeenCalled()

    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: currentRelease,
    })
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('current generation', { status: 200 }),
    )

    const fetchEvent = buildClientFetchEvent(path, 'client-a')
    const response = await swFetch(fetchEvent.ev)

    expect(await response.text()).toBe('current generation')
    expect(proxyFetch).toHaveBeenCalledOnce()
    await Promise.all(fetchEvent.waitUntilPromises)
  })

  it('repairs a stable-name asset after one same-generation stale response', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const release = buildRelease('gen-a')
    const path = '/b/pd/spacewave-app/backend.mjs'
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: release,
    })
    await announcePluginRoot('spacewave-app', '2abc')
    await writeGenerationCacheResponse(
      caches,
      release.generationId,
      path,
      new Response('stale stable entry', { status: 200 }),
    )
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('refreshed stable entry', { status: 200 }),
    )

    const first = buildClientFetchEvent(path, 'client-a')
    const firstResponse = await swFetch(first.ev)
    expect(await firstResponse.text()).toBe('stale stable entry')
    await Promise.all(first.waitUntilPromises)

    const secondResponse = await swFetch(buildFetchOnlyEvent(path))
    expect(await secondResponse.text()).toBe('refreshed stable entry')
  })

  it('keeps dynamic plugin HTTP outside the static asset cache-first path', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const release = buildRelease('gen-a')
    const path = '/p/spacewave-core/fs/file.txt'
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: release,
    })
    await writeGenerationCacheResponse(
      caches,
      release.generationId,
      path,
      new Response('stale dynamic response', { status: 200 }),
    )
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('runtime dynamic response', { status: 200 }),
    )

    const fetchEvent = buildClientFetchEvent(path, 'client-a')
    const response = await swFetch(fetchEvent.ev)

    expect(await response.text()).toBe('runtime dynamic response')
    expect(proxyFetch).toHaveBeenCalledOnce()
    expect(fetchEvent.waitUntilPromises).toHaveLength(0)
  })

  it('rehydrates persisted plugin root authority after service worker restart', async () => {
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: buildRelease('gen-a'),
      },
    )
    await announcePluginRoot('spacewave-app', '2abc')
    const body = 'export const App2 = () => null\n'
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'application/javascript' },
      }),
    )

    const warm = buildClientFetchEvent(
      '/b/pd/spacewave-app/backend.mjs',
      'client-a',
    )
    const warmResponse = await swFetch(warm.ev)
    expect(warmResponse.status).toBe(200)
    expect(await warmResponse.text()).toBe(body)
    await Promise.all(warm.waitUntilPromises)

    // Simulate a ServiceWorker restart without deleting CacheStorage.
    resetServiceWorkerTestState()
    vi.mocked(proxyFetch).mockClear()
    const restartedResponse = await swFetch(
      buildFetchOnlyEvent('/b/pd/spacewave-app/backend.mjs'),
    )
    expect(restartedResponse.status).toBe(200)
    expect(await restartedResponse.text()).toBe(body)
    expect(proxyFetch).not.toHaveBeenCalled()
  })

  it('rejects plugin root metadata from another service worker release', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    const path = '/b/pd/spacewave-app/backend.mjs'
    await writeControlCacheResponse(
      caches,
      '/__bldr/plugin-manifest-root/spacewave-app.json',
      new Response(
        JSON.stringify({
          pluginId: 'spacewave-app',
          rootHash: '2abc',
          serviceWorkerURL: `${self.location.href}?stale=1`,
        }),
      ),
    )
    resetServiceWorkerTestState()
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('current app', { status: 200 }),
    )

    const response = await swFetch(
      buildFetchOnlyEvent(path, undefined, 'client-a'),
    )
    expect(response.status).toBe(200)
    expect(await response.text()).toBe('current app')
    expect(proxyFetch).toHaveBeenCalledOnce()
  })

  it('does not activate a plugin root when durable metadata write fails', async () => {
    const caches = new FakeCacheStorage((cacheName, request) => {
      if (
        cacheName === 'bldr-control' &&
        new URL(request.url).pathname.startsWith(
          '/__bldr/plugin-manifest-root/',
        )
      ) {
        return newCachePutError()
      }
      return undefined
    })
    vi.stubGlobal('caches', caches)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    await announcePluginRoot('spacewave-app', '2abc')
    const body = 'export const App2 = () => null\n'
    vi.mocked(proxyFetch).mockImplementation(() =>
      Promise.resolve(
        new Response(body, {
          status: 200,
          headers: { 'Content-Type': 'application/javascript' },
        }),
      ),
    )

    for (let attempt = 0; attempt < 2; attempt++) {
      const response = await swFetch(
        buildFetchOnlyEvent(
          '/b/pd/spacewave-app/backend.mjs',
          undefined,
          'client-a',
        ),
      )
      expect(response.status).toBe(200)
      expect(await response.text()).toBe(body)
    }
    expect(proxyFetch).toHaveBeenCalledTimes(2)

    resetServiceWorkerTestState()
    vi.mocked(proxyFetch).mockClear()
    const restartedResponse = await swFetch(
      buildFetchOnlyEvent(
        '/b/pd/spacewave-app/backend.mjs',
        undefined,
        'client-a',
      ),
    )
    expect(restartedResponse.status).toBe(200)
    expect(await restartedResponse.text()).toBe(body)
    expect(proxyFetch).toHaveBeenCalledOnce()
    expect(warn).toHaveBeenCalled()
  })

  it('keeps static plugin asset fetches successful when cache writes fail', async () => {
    const caches = new FakeCacheStorage((cacheName) => {
      if (cacheName.startsWith('bldr-generation-')) {
        return newCachePutError()
      }
      return undefined
    })
    vi.stubGlobal('caches', caches)
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: buildRelease('gen-a'),
    })
    await announcePluginRoot('spacewave-app', '2abc')
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const body = 'export const App2 = () => null\n'
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'application/javascript' },
      }),
    )

    const warm = buildClientFetchEvent(
      '/b/pa/spacewave-app/v/b/fe/app/App2.mjs',
      'client-a',
    )
    const response = await swFetch(warm.ev)

    expect(response.status).toBe(200)
    expect(await response.text()).toBe(body)
    await expect(Promise.all(warm.waitUntilPromises)).resolves.toEqual([
      undefined,
    ])
    const cache = await caches.open('bldr-generation-gen-a')
    const cached = await cache.match(
      new Request(
        new URL('/b/pa/spacewave-app/v/b/fe/app/App2.mjs', self.location.href),
      ),
    )
    expect(cached).toBeUndefined()
    expect(warn).toHaveBeenCalledWith(
      'ServiceWorker: %s: cache write failed: operation=%s cache=%s%s%s url=%s: %s',
      expect.any(String),
      'cache static plugin asset',
      'bldr-generation-gen-a',
      ' generation=gen-a',
      '',
      expect.any(String),
      'Cache.put() encountered a network error',
    )
  })

  it('falls back to the cached static plugin asset when a mid-handover runtime fetch fails', async () => {
    const caches = globalThis.caches as unknown as FakeCacheStorage
    await writeBrowserReleaseState(caches, {
      ...createEmptyBrowserReleaseState(),
      promotedCurrent: buildRelease('gen-a'),
    })
    await announcePluginRoot('spacewave-app', '2abc')
    const body = 'export const chunk = 1\n'
    vi.mocked(proxyFetch).mockResolvedValueOnce(
      new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'application/javascript' },
      }),
    )
    const warm = buildClientFetchEvent(
      '/b/pd/spacewave-app/backend.mjs',
      'client-a',
    )
    expect((await swFetch(warm.ev)).status).toBe(200)
    await Promise.all(warm.waitUntilPromises)

    // The next page still has a client id, but its runtime relay is tearing
    // down, so the forwarded fetch fails. The cached asset must still serve.
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response(
        'WebRuntimeClient: client-a: timeout opening stream with host',
        { status: 500 },
      ),
    )
    const handoverResponse = await swFetch(
      buildFetchOnlyEvent(
        '/b/pd/spacewave-app/backend.mjs',
        undefined,
        'client-a',
      ),
    )
    expect(handoverResponse.status).toBe(200)
    expect(await handoverResponse.text()).toBe(body)
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
      clients: buildTestClients(),
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
    expect(syncLatestBrowserRelease).toHaveBeenCalledWith({
      lifecycleProbe: true,
    })
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

  it('owns bldrSyncManifest sync failures inside waitUntil', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const syncLatestBrowserRelease = vi
      .fn()
      .mockRejectedValueOnce(new Error('cache write failed'))
      .mockResolvedValueOnce(createEmptyBrowserReleaseState())
    const deps = {
      clients: buildTestClients(),
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
    handleServiceWorkerMessage(firstEv, deps)

    expect(firstEv.waitUntil).toHaveBeenCalledWith(expect.any(Promise))
    await expect(
      vi.mocked(firstEv.waitUntil).mock.calls[0][0],
    ).resolves.toBeUndefined()
    expect(warn).toHaveBeenCalledWith(
      'ServiceWorker: %s: browser release sync failed: %s',
      expect.any(String),
      'cache write failed',
    )

    const secondEv = buildMessageEvent({ bldrSyncManifest: true })
    handleServiceWorkerMessage(secondEv, deps)

    expect(syncLatestBrowserRelease).toHaveBeenCalledTimes(2)
    expect(secondEv.waitUntil).toHaveBeenCalledWith(expect.any(Promise))
    await vi.mocked(secondEv.waitUntil).mock.calls[0][0]
  })

  it('refreshes the runtime browser index cache from a message', async () => {
    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', new FakeCacheStorage())
    vi.mocked(proxyFetch).mockResolvedValue(
      new Response('runtime index', { status: 200 }),
    )
    const deps = {
      clients: buildTestClients(),
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

  it('proxies attached DedicatedWorker documents to the elected runtime host relay', async () => {
    const runtimeChannel = new MessageChannel()
    const responseChannel = new MessageChannel()
    const tracker = {
      handleWebDocumentMessage: vi.fn(),
      openWebRuntimePortWithResult: vi.fn().mockResolvedValue({
        webRuntimePort: runtimeChannel.port1,
        hostDocumentId: 'host-document',
        hostGeneration: 'host-generation-1',
      } satisfies OpenWebRuntimePortResult),
    }
    const deps = {
      clients: buildTestClients(),
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: tracker,
      syncLatestBrowserRelease: vi.fn(),
      refreshBrowserIndexCache: vi.fn(),
      handleCrossTabMessage: vi.fn(),
    }
    const ack = new Promise<MessageEvent>((resolve) => {
      responseChannel.port1.onmessage = resolve
      responseChannel.port1.start()
    })
    const init = new Uint8Array([1, 2, 3])
    const ev = buildMessageEvent({
      from: 'attached-document',
      connectDedicatedRuntimeHost: {
        webRuntimeId: 'runtime-1',
        init,
        port: responseChannel.port2,
      },
    })

    handleServiceWorkerMessage(ev, deps)
    await vi.mocked(ev.waitUntil).mock.calls[0][0]
    expect(tracker.openWebRuntimePortWithResult).toHaveBeenCalledWith(
      init,
      'attached-document',
      expect.any(AbortSignal),
    )
    const ackEvent = await ack
    expect(ackEvent.data).toMatchObject({
      from: expect.stringMatching(/^service-worker-/),
    })
    const openedPort = ackEvent.data.webRuntimePort ?? ackEvent.ports?.[0]
    expect(openedPort).toBeDefined()
    const delivered = new Promise<unknown>((resolve) => {
      runtimeChannel.port2.onmessage = (messageEvent: MessageEvent) =>
        resolve(messageEvent.data)
      runtimeChannel.port2.start()
    })
    openedPort.postMessage({ relayed: true })
    await expect(delivered).resolves.toEqual({ relayed: true })

    openedPort.close()
    runtimeChannel.port2.close()
    responseChannel.port1.close()
  })

  it('reports relay failure without returning warm connection metadata', async () => {
    const responseChannel = new MessageChannel()
    const ack = new Promise<MessageEvent>((resolve) => {
      responseChannel.port1.onmessage = resolve
      responseChannel.port1.start()
    })
    const tracker = {
      handleWebDocumentMessage: vi.fn(),
      openWebRuntimePortWithResult: vi
        .fn()
        .mockRejectedValue(new Error('relay unavailable')),
    }
    const deps = {
      clients: buildTestClients(),
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: tracker,
      syncLatestBrowserRelease: vi.fn(),
      refreshBrowserIndexCache: vi.fn(),
      handleCrossTabMessage: vi.fn(),
    }
    const ev = buildMessageEvent({
      from: 'attached-document',
      connectDedicatedRuntimeHost: {
        webRuntimeId: 'runtime-1',
        init: new Uint8Array([1, 2, 3]),
        port: responseChannel.port2,
      },
    })

    handleServiceWorkerMessage(ev, deps)
    await vi.mocked(ev.waitUntil).mock.calls[0][0]
    const ackEvent = await ack
    expect(ackEvent.data).toMatchObject({
      error: 'relay unavailable',
    })
    expect(ackEvent.data.webRuntimePort).toBeUndefined()
    expect(ackEvent.data.hostDocumentId).toBeUndefined()
    expect(ackEvent.data.hostGeneration).toBeUndefined()
    responseChannel.port1.close()
  })

  it('aborts a pending elected-host lookup when the requester cancels', async () => {
    let connectSignal: AbortSignal | undefined
    const responseChannel = new MessageChannel()
    const openWebRuntimePortWithResult = vi.fn(
      (_init: Uint8Array, _from?: string, signal?: AbortSignal) =>
        new Promise<OpenWebRuntimePortResult>((_resolve, reject) => {
          connectSignal = signal
          signal?.addEventListener(
            'abort',
            () => {
              const reason = signal.reason
              reject(
                reason instanceof Error
                  ? reason
                  : new DOMException('relay canceled', 'AbortError'),
              )
            },
            { once: true },
          )
        }),
    )
    const deps = {
      clients: buildTestClients(),
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: {
        handleWebDocumentMessage: vi.fn(),
        openWebRuntimePortWithResult,
      },
      syncLatestBrowserRelease: vi.fn(),
      refreshBrowserIndexCache: vi.fn(),
      handleCrossTabMessage: vi.fn(),
    }
    const ev = buildMessageEvent({
      from: 'attached-document',
      connectDedicatedRuntimeHost: {
        webRuntimeId: 'runtime-1',
        init: new Uint8Array([1, 2, 3]),
        port: responseChannel.port2,
      },
    })

    handleServiceWorkerMessage(ev, deps)
    await vi.waitFor(() => {
      expect(connectSignal).toBeDefined()
    })
    responseChannel.port1.postMessage({
      cancelDedicatedRuntimeHostConnect: true,
    })
    await vi.mocked(ev.waitUntil).mock.calls[0][0]

    expect(connectSignal?.aborted).toBe(true)
    responseChannel.port1.close()
  })

  it('aborts outstanding fetch waiters when a client says goodbye', () => {
    const deps = {
      clients: buildTestClients(),
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
