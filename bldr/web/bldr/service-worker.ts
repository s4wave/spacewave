import { castToError } from 'starpc'
import { ServiceWorkerHostClient } from '../runtime/sw/sw_srpc.pb.js'
import { proxyFetch } from '../fetch/fetch.js'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { BLDR_URI_PREFIXES } from './constants.js'
import {
  type BrowserReleaseDescriptor,
  type BrowserReleaseState,
  BROWSER_RELEASE_STATE_SCHEMA_VERSION,
  buildOfflineNavigationFallbacks,
  buildReleaseCachePaths,
  createEmptyBrowserReleaseState,
  normalizeReleasePath,
  promoteBrowserRelease,
  retainedGenerationIds,
  sameBrowserRelease,
} from './browser-release-state.js'
import { isCrossTabMessage, handleCrossTabMessage } from './cross-tab-broker.js'
import { randomId } from './random-id.js'
import { ServiceWorkerFetchTracker } from './service-worker-fetch-tracker.js'
import { WebDocumentTracker } from './web-document-tracker.js'
import { ServiceWorkerToWebDocument } from 'web/runtime/runtime.js'

declare let BLDR_DEBUG: boolean

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
declare let self: ServiceWorkerGlobalScope

// note: logs don't appear in console in firefox
const serviceWorkerLogicalId = `service-worker-${self.location.host.replace(/:/g, '-')}`
const serviceWorkerId = `${serviceWorkerLogicalId}-${randomId()}`

// baseURL is the base URL to use for paths.
const baseURL = new URL(self.location.toString())

const controlCacheName = 'bldr-control'
const browserReleasePath = '/browser-release.json'
const bootAssetPath = '/boot.mjs'
const browserReleaseStatePath = '/__bldr/browser-release-state.json'

// CACHES is the list of fixed caches.
const CACHES: Record<string, Cache | undefined> = { [controlCacheName]: undefined }
const serviceWorkerFetchTracker = new ServiceWorkerFetchTracker()
const proxyFetchHeaderTimeoutMs = 30_000

function buildCacheRequest(path: string): Request {
  return new Request(new URL(path, baseURL).toString())
}

function buildGenerationCacheName(generationId: string): string {
  return `bldr-generation-${generationId}`
}

async function notifyPromotedGenerationReload(
  previousGenerationId: string,
  promotedGenerationId: string,
): Promise<void> {
  if (previousGenerationId === promotedGenerationId) {
    return
  }
  const currClients = await self.clients.matchAll({ type: 'window' })
  for (const client of currClients) {
    client.postMessage({
      bldrPromotedGenerationId: promotedGenerationId,
    })
  }
}

async function getControlCache(): Promise<Cache> {
  const cached = CACHES[controlCacheName]
  if (cached) {
    return cached
  }
  const cache = await caches.open(controlCacheName)
  CACHES[controlCacheName] = cache
  return cache
}

function buildJsonResponse(method: string, value: unknown): Response {
  const response = new Response(
    method === 'HEAD' ? null : JSON.stringify(value),
    {
      status: 200,
      headers: {
        'Content-Type': 'application/json; charset=utf-8',
      },
    },
  )
  return response
}

function buildHeadResponse(response: Response): Response {
  return new Response(null, {
    status: response.status,
    statusText: response.statusText,
    headers: new Headers(response.headers),
  })
}

function responseForMethod(request: Request, response: Response): Response {
  if (request.method === 'HEAD') {
    return buildHeadResponse(response)
  }
  return response
}

async function readCachedJson<T>(path: string): Promise<T | null> {
  const cache = await getControlCache()
  const response = await cache.match(buildCacheRequest(path))
  if (!response) {
    return null
  }
  return (await response.json()) as T
}

async function writeCachedJson(path: string, value: unknown): Promise<void> {
  const cache = await getControlCache()
  await cache.put(buildCacheRequest(path), buildJsonResponse('GET', value))
}

async function loadBrowserReleaseState(): Promise<BrowserReleaseState> {
  const state = await readCachedJson<BrowserReleaseState>(browserReleaseStatePath)
  if (!state) {
    return createEmptyBrowserReleaseState()
  }
  if (state.schemaVersion !== BROWSER_RELEASE_STATE_SCHEMA_VERSION) {
    return createEmptyBrowserReleaseState()
  }
  return state
}

async function saveBrowserReleaseState(state: BrowserReleaseState): Promise<void> {
  await writeCachedJson(browserReleaseStatePath, state)
}

async function cacheStableBootAsset(): Promise<void> {
  let response: Response
  try {
    response = await fetch(new Request(new URL(bootAssetPath, baseURL).toString(), {
      cache: 'reload',
    }))
  } catch (error) {
    console.warn(
      'ServiceWorker: %s: unable to refresh stable boot asset: %s',
      serviceWorkerId,
      castToError(error, 'unknown error').message,
    )
    return
  }
  if (!response.ok) {
    console.warn(
      'ServiceWorker: %s: stable boot asset fetch failed: %d',
      serviceWorkerId,
      response.status,
    )
    return
  }
  const cache = await getControlCache()
  await cache.put(buildCacheRequest(bootAssetPath), response.clone())
}

async function fetchLatestBrowserRelease(): Promise<BrowserReleaseDescriptor | null> {
  let response: Response
  try {
    response = await fetch(
      new Request(new URL(browserReleasePath, baseURL).toString(), {
        cache: 'no-cache',
      }),
    )
  } catch (error) {
    console.warn(
      'ServiceWorker: %s: unable to fetch browser release manifest: %s',
      serviceWorkerId,
      castToError(error, 'unknown error').message,
    )
    return null
  }
  if (!response.ok) {
    console.warn(
      'ServiceWorker: %s: browser release manifest fetch failed: %d',
      serviceWorkerId,
      response.status,
    )
    return null
  }
  return (await response.json()) as BrowserReleaseDescriptor
}

async function stageBrowserRelease(
  release: BrowserReleaseDescriptor,
): Promise<boolean> {
  const cache = await caches.open(buildGenerationCacheName(release.generationId))
  for (const path of buildReleaseCachePaths(release)) {
    const request = buildCacheRequest(path)
    let response: Response
    try {
      response = await fetch(new Request(request.url, { cache: 'reload' }))
    } catch (error) {
      console.warn(
        'ServiceWorker: %s: failed to stage %s for %s: %s',
        serviceWorkerId,
        path,
        release.generationId,
        castToError(error, 'unknown error').message,
      )
      return false
    }
    if (!response.ok) {
      console.warn(
        'ServiceWorker: %s: refusing to stage %s for %s, status=%d',
        serviceWorkerId,
        path,
        release.generationId,
        response.status,
      )
      return false
    }
    await cache.put(request, response.clone())
  }
  for (const path of buildReleaseCachePaths(release)) {
    const cached = await cache.match(buildCacheRequest(path))
    if (!cached) {
      console.warn(
        'ServiceWorker: %s: staged cache missing %s for %s',
        serviceWorkerId,
        path,
        release.generationId,
      )
      return false
    }
  }
  return true
}

async function pruneReleaseCaches(state: BrowserReleaseState): Promise<void> {
  const retainedCaches = new Set<string>([controlCacheName])
  for (const generationId of retainedGenerationIds(state)) {
    retainedCaches.add(buildGenerationCacheName(generationId))
  }
  const cacheNames = await caches.keys()
  for (const cacheName of cacheNames) {
    if (!retainedCaches.has(cacheName)) {
      await caches.delete(cacheName)
    }
  }
}

async function syncLatestBrowserRelease(
  discoveredRelease?: BrowserReleaseDescriptor | null,
): Promise<BrowserReleaseState> {
  await cacheStableBootAsset()

  let state = await loadBrowserReleaseState()
  const previousPromotedRelease = state.promotedCurrent
  const release = discoveredRelease ?? (await fetchLatestBrowserRelease())
  if (!release) {
    await pruneReleaseCaches(state)
    return state
  }

  if (
    sameBrowserRelease(state.discovered, release) &&
    sameBrowserRelease(state.staged, release) &&
    sameBrowserRelease(state.promotedCurrent, release)
  ) {
    await pruneReleaseCaches(state)
    return state
  }

  state = { ...state, discovered: release }
  await saveBrowserReleaseState(state)

  if (!(await stageBrowserRelease(release))) {
    await pruneReleaseCaches(state)
    return state
  }

  state = promoteBrowserRelease(state, release)
  await saveBrowserReleaseState(state)
  await pruneReleaseCaches(state)
  if (
    previousPromotedRelease &&
    state.promotedCurrent &&
    !sameBrowserRelease(previousPromotedRelease, state.promotedCurrent)
  ) {
    await notifyPromotedGenerationReload(
      previousPromotedRelease.generationId,
      state.promotedCurrent.generationId,
    )
  }
  return state
}

async function matchStableBootAsset(request: Request): Promise<Response | null> {
  const cache = await getControlCache()
  const response = await cache.match(buildCacheRequest(bootAssetPath))
  if (!response) {
    return null
  }
  return responseForMethod(request, response)
}

async function matchPromotedGenerationResponse(
  request: Request,
): Promise<Response | null> {
  const pathname = normalizeReleasePath(new URL(request.url).pathname)
  const accept = request.headers.get('Accept') ?? ''
  const isNavigation =
    request.mode === 'navigate' ||
    request.destination === 'document' ||
    accept.includes('text/html')
  const state = await loadBrowserReleaseState()

  for (const release of [state.promotedCurrent, state.promotedPrevious]) {
    if (!release) {
      continue
    }
    const cache = await caches.open(buildGenerationCacheName(release.generationId))
    const candidates =
      isNavigation ?
        buildOfflineNavigationFallbacks(pathname, release)
      : [pathname]
    for (const candidate of candidates) {
      const response = await cache.match(buildCacheRequest(candidate))
      if (response) {
        return responseForMethod(request, response)
      }
    }
  }

  return null
}

async function handleBrowserReleaseRequest(ev: FetchEvent): Promise<Response> {
  const request = ev.request
  const state = await loadBrowserReleaseState()
  if (state.promotedCurrent) {
    ev.waitUntil(syncLatestBrowserRelease())
    return buildJsonResponse(request.method, state.promotedCurrent)
  }

  const latestRelease = await fetchLatestBrowserRelease()
  if (!latestRelease) {
    const fallback = state.promotedPrevious
    if (fallback) {
      return buildJsonResponse(request.method, fallback)
    }
    throw new Error('browser release manifest unavailable')
  }

  ev.waitUntil(syncLatestBrowserRelease(latestRelease))
  return buildJsonResponse(request.method, latestRelease)
}

// onWebDocumentsExhausted notifies all web documents we need a new connection.
const onWebDocumentsExhausted = async () => {
  await self.clients.claim()
  const currClients = await self.clients.matchAll({ type: 'window' })
  if (BLDR_DEBUG) {
    console.log(
      'ServiceWorker: %s: notifying %d clients we want a connection',
      serviceWorkerLogicalId,
      currClients.length,
    )
  }
  for (const client of currClients) {
    client.postMessage(<ServiceWorkerToWebDocument>{
      from: serviceWorkerLogicalId,
      init: true,
    })
  }
}

// webDocumentTracker tracks the set of connected remote WebDocument.
const webDocumentTracker = new WebDocumentTracker(
  serviceWorkerId,
  WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
  onWebDocumentsExhausted,
  // We don't support calling the ServiceWorker from WebDocument.
  null,
  null,
  serviceWorkerLogicalId,
)

// webRuntimeClient manages the connection to the WebRuntime.
const webRuntimeClient = webDocumentTracker.webRuntimeClient

// swHostClient attempts to contact the WebRuntime over any of the WebDocument relays.
const swHostClient = webRuntimeClient.rpcClient

// swHost is the RPC client for the ServiceWorkerHost.
const swHost = new ServiceWorkerHostClient(swHostClient)

// install is the beginning of service worker registration.
// setup resources such as offline caches.
// note: does not activate until some time after this returns.
async function swInstall() {
  await self.skipWaiting()
}

// swActivate is called when the service worker becomes active.
async function swActivate() {
  // Claim all clients.
  await self.clients.claim()
  await getControlCache()
  await syncLatestBrowserRelease()
}

// isSwOrigin checks if the given origin matches the local origin.
function isSwOrigin(origin: string): boolean {
  return origin === self.location.origin
}

// swFetch is called when the page attempts to fetch a resource.
async function swFetch(
  ev: FetchEvent,
  matchPrefixes = BLDR_URI_PREFIXES,
): Promise<Response> {
  const request = ev.request
  const requestURL = new URL(request.url)
  const requestOrigin = requestURL.origin
  const requestPath = requestURL.pathname

  if (isSwOrigin(requestOrigin) && requestPath === browserReleasePath) {
    return handleBrowserReleaseRequest(ev)
  }

  // TODO: Browsers do not cancel request.signal when the request is canceled.
  // This is a long-standing browser bug and is not yet fixed.
  // See: https://github.com/w3c/ServiceWorker/issues/1544
  // See: https://bugzilla.mozilla.org/show_bug.cgi?id=1394102
  // See: https://bugzilla.mozilla.org/show_bug.cgi?id=1568422
  //
  // To view the effect of this:
  // 1. Browse to a bldr site in one tab.
  // 2. Browse to /p/does-not-exist/a/ in a new tab
  // 3. The request will wait forever
  // 4. Close the /p/does-not-exist tab.
  // 5. Notice the request is not canceled.
  /*
  const requestSignal = ev.request.signal
  requestSignal.addEventListener('abort', () => {
    // This line is never printed!
    console.error('requestSignal: aborted for ' + ev.request.url.toString())
  })
  */

  const useRuntimeFetch =
    isSwOrigin(requestOrigin) &&
    matchPrefixes.some((matchPrefix) => requestPath.startsWith(matchPrefix))

  if (!useRuntimeFetch) {
    // Check the cache (for e.x. index.html)
    // NOTE: We do not want this, we want the latest index.html if possible.
    /*
    const bldrCache = CACHES['bldr']
    if (bldrCache) {
      const cacheResp = await bldrCache.match(request)
      if (cacheResp) {
        return cacheResp
      }
    }
    */

    // Use the built-in browser fetch.
    if (BLDR_DEBUG) {
      console.log(
        'ServiceWorker: %s: using native fetch: %s',
        serviceWorkerId,
        request.url.toString(),
      )
    }

    let response: Response | null = null
    let responseErr: unknown | null = null
    try {
      response = await fetch(ev.request)
    } catch (err) {
      responseErr = err
      console.warn(
        'ServiceWorker: %s: native fetch failed: %s: %s',
        serviceWorkerId,
        request.url.toString(),
        castToError(err, 'unknown error').message,
      )
      response = null
    }

    // request failed, attempt to fall back to cache.
    if (!response || response.status < 200 || response.status >= 300) {
      if (requestPath === bootAssetPath) {
        const bootResponse = await matchStableBootAsset(request)
        if (bootResponse) {
          return bootResponse
        }
      }

      const cacheResp = await matchPromotedGenerationResponse(request)
      if (cacheResp) {
        return cacheResp
      }
    }

    // finally throw err if any
    if (responseErr) {
      throw responseErr
    }

    return response!
  }

  if (BLDR_DEBUG) {
    console.log(
      'ServiceWorker: %s: forwarding fetch to runtime: %s',
      serviceWorkerId,
      request.url.toString(),
    )
  }
  if (!ev.clientId) {
    return proxyFetch(swHost, request, ev.clientId, {
      headerTimeoutMs: proxyFetchHeaderTimeoutMs,
    })
  }

  const trackedFetch = serviceWorkerFetchTracker.trackFetch(ev.clientId)
  return proxyFetch(swHost, request, ev.clientId, {
    abortSignal: trackedFetch.abortController.signal,
    headerTimeoutMs: proxyFetchHeaderTimeoutMs,
  }).finally(() => trackedFetch.release())

  /*
  Not working with custom app:// scheme in Electron.
  https://github.com/electron/electron/issues/35033
  response.then((resp) => {
    if (resp.ok) {
      bldrCache().then((bcache) => {
        console.log('BLDR_CACHE', requestURL.toString(), resp)
        bcache.put(request, resp)
      })
    }
  })
  */
}

function initServiceWorker() {
  // install event is called when service worker is installed.
  self.addEventListener('install', (ev: Event) => {
    const e = ev as ExtendableEvent
    e.waitUntil(swInstall())
  })

  // activate event is called when service worker is activated.
  self.addEventListener('activate', (ev: Event) => {
    const e = ev as ExtendableEvent
    e.waitUntil(swActivate())
  })

  // message event is called when receiving a message from the page.
  self.addEventListener('message', (ev: ExtendableMessageEvent) => {
    // Cross-tab channel broker: handle hello/goodbye before WebDocument messages.
    if (isCrossTabMessage(ev.data)) {
      const senderId = (ev.source as Client)?.id
      if (senderId) {
        if (ev.data.crossTab === 'goodbye') {
          serviceWorkerFetchTracker.abortClient(
            senderId,
            new Error('service worker client closed'),
          )
        }
        ev.waitUntil(handleCrossTabMessage(self.clients, senderId, ev.data))
      }
      return
    }
    webDocumentTracker.handleWebDocumentMessage(ev.data)
  })

  // fetch event is called when a URL within the scope is accessed.
  self.addEventListener('fetch', (ev: FetchEvent) => {
    ev.respondWith(
      swFetch(ev).catch((e) => {
        const err = castToError(e, '500 internal error')
        console.warn(
          'ServiceWorker: %s: error handling fetch: %s',
          serviceWorkerId,
          ev.request.url.toString(),
          err,
        )
        return new Response(err.message, {
          status: 500,
        })
      }),
    )
  })
}

// IS_SERVICE_WORKER indicates if initServiceWorker was called.
// If we are not a service worker, don't register callbacks.
const IS_SERVICE_WORKER = !!self && !!self.clients
if (IS_SERVICE_WORKER) {
  initServiceWorker()
}
