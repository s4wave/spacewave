import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { proxyFetch } from '../fetch/fetch.js'
import { createEmptyBrowserReleaseState } from './browser-release-state.js'
import type {
  BrowserReleaseDescriptor,
  BrowserReleaseState,
} from './browser-release-state.js'
import {
  handleBrowserReleaseRequest,
  handleServiceWorkerMessage,
  refreshBrowserIndexCache,
  resetServiceWorkerTestState,
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

function buildFetchOnlyEvent(path: string, init?: RequestInit): FetchEvent {
  return {
    request: new Request(new URL(path, self.location.href), init),
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

  it('serves root navigation requests from the cached runtime browser index', async () => {
    await writeControlCacheResponse(
      globalThis.caches as unknown as FakeCacheStorage,
      '/b/__index.html',
      new Response('runtime index', { status: 200 }),
    )
    vi.mocked(fetch).mockResolvedValue(new Response('network', { status: 200 }))

    const response = await swFetch(
      buildFetchOnlyEvent('/', {
        headers: { Accept: 'text/html' },
      }),
    )

    expect(await response.text()).toBe('runtime index')
    expect(fetch).not.toHaveBeenCalled()
    expect(proxyFetch).not.toHaveBeenCalled()
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
    const [rootResponse, indexResponse] = await Promise.all([
      cache.match(new Request(new URL('/', self.location.href))),
      cache.match(new Request(new URL('/b/__index.html', self.location.href))),
    ])

    expect(await rootResponse?.text()).toBe('runtime index')
    expect(await indexResponse?.text()).toBe('runtime index')
    expect(proxyFetch).toHaveBeenCalledWith(
      expect.anything(),
      expect.any(Request),
      'client-a',
      expect.objectContaining({ headerTimeoutMs: 30_000 }),
    )
  })

  it('updates cached root content when the browser index cache is refreshed', async () => {
    const caches = new FakeCacheStorage()

    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', caches)
    vi.mocked(proxyFetch)
      .mockResolvedValueOnce(new Response('stale index', { status: 200 }))
      .mockResolvedValueOnce(new Response('fresh index', { status: 200 }))

    await refreshBrowserIndexCache('client-a')
    await refreshBrowserIndexCache('client-a')

    const cache = await caches.open('bldr-control')
    const [rootResponse, indexResponse] = await Promise.all([
      cache.match(new Request(new URL('/', self.location.href))),
      cache.match(new Request(new URL('/b/__index.html', self.location.href))),
    ])

    expect(await rootResponse?.text()).toBe('fresh index')
    expect(await indexResponse?.text()).toBe('fresh index')
  })
})
