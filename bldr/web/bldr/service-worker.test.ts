import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createEmptyBrowserReleaseState } from './browser-release-state.js'
import type {
  BrowserReleaseDescriptor,
  BrowserReleaseState,
} from './browser-release-state.js'
import {
  handleBrowserReleaseRequest,
  handleServiceWorkerMessage,
  resetServiceWorkerTestState,
} from './service-worker.js'

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
    let cache = this.caches.get(name)
    if (!cache) {
      cache = new FakeCache()
      this.caches.set(name, cache)
    }
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
    new Request(new URL('/__bldr/browser-release-state.json', self.location)),
    new Response(JSON.stringify(state)),
  )
}

function buildMessageEvent(data: unknown): ExtendableMessageEvent {
  return {
    data,
    waitUntil: vi.fn(),
  } as unknown as ExtendableMessageEvent
}

describe('service worker browser release requests', () => {
  beforeEach(() => {
    resetServiceWorkerTestState()
    vi.stubGlobal('BLDR_DEBUG', false)
    vi.stubGlobal('caches', new FakeCacheStorage())
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('returns cached promoted manifest synchronously when one exists', async () => {
    const release = buildRelease('gen-a')
    await writeBrowserReleaseState(
      globalThis.caches as unknown as FakeCacheStorage,
      {
        ...createEmptyBrowserReleaseState(),
        promotedCurrent: release,
      },
    )
    const pendingFetch = newDeferred<Response>()
    vi.mocked(fetch).mockReturnValue(pendingFetch.promise)
    const { ev, waitUntilPromises } = buildFetchEvent(
      'https://example.test/browser-release.json',
    )

    const response = await handleBrowserReleaseRequest(ev)

    expect(await response.json()).toEqual(release)
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
})

describe('service worker messages', () => {
  it('does not handle bldrSyncManifest messages yet', () => {
    const ev = buildMessageEvent({ bldrSyncManifest: true })
    const deps = {
      clients: {} as Clients,
      fetchTracker: {
        abortClient: vi.fn(),
      },
      webDocumentTracker: {
        handleWebDocumentMessage: vi.fn(),
      },
      handleCrossTabMessage: vi.fn(),
    }

    handleServiceWorkerMessage(ev, deps)

    expect(ev.waitUntil).not.toHaveBeenCalled()
    expect(deps.handleCrossTabMessage).not.toHaveBeenCalled()
    expect(
      deps.webDocumentTracker.handleWebDocumentMessage,
    ).toHaveBeenCalledWith({
      bldrSyncManifest: true,
    })
  })
})
