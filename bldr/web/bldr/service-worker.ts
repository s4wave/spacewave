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
  isBrowserCacheSupportedURL,
  normalizeReleasePath,
  promoteBrowserRelease,
  retainedGenerationIds,
  sameBrowserRelease,
} from './browser-release-state.js'
import { isCrossTabMessage, handleCrossTabMessage } from './cross-tab-broker.js'
import { randomId } from './random-id.js'
import { ServiceWorkerFetchTracker } from './service-worker-fetch-tracker.js'
import { WebDocumentTracker } from './web-document-tracker.js'
import { markStartupBoundary } from './startup-marks.js'

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
const browserIndexPath = '/b/__index.html'
const rootNavigationPath = '/'
const browserReleaseStatePath = '/__bldr/browser-release-state.json'
const pluginDistPathPrefix = '/b/pd/'
const pluginAssetsPathPrefix = '/b/pa/'
const pluginWebPkgPathPrefix = '/b/pkg/'
const quickJSRuntimePathPrefix = '/b/qjs/'
const pluginHttpPathPrefix = '/p/'
const browserRuntimeFetchHeaderTimeoutMs = 30000
// browserRuntimeFetchRelayWaitMs bounds how long a static plugin asset fetch
// waits for the next runtime relay across a page-client close gap before failing.
const browserRuntimeFetchRelayWaitMs = 5000

// CACHES is the list of fixed caches.
const CACHES: Record<string, Cache | undefined> = {
  [controlCacheName]: undefined,
}
const serviceWorkerFetchTracker = new ServiceWorkerFetchTracker()
const browserReleaseNetworkRaceTimeoutMs = 800
const browserReleaseNetworkRaceTimedOut = Symbol(
  'browserReleaseNetworkRaceTimedOut',
)

// ServiceWorkerMessageDeps collects message-handler collaborators.
export interface ServiceWorkerMessageDeps {
  clients: Clients
  fetchTracker: Pick<ServiceWorkerFetchTracker, 'abortClient'>
  webDocumentTracker: Pick<WebDocumentTracker, 'handleWebDocumentMessage'>
  syncLatestBrowserRelease(): Promise<BrowserReleaseState>
  refreshBrowserIndexCache(clientId: string): Promise<Response>
  handleCrossTabMessage(
    clients: Clients,
    senderId: string,
    data: unknown,
  ): Promise<void>
}

// BrowserReleaseSyncMessage asks the SW to refresh the release manifest.
export interface BrowserReleaseSyncMessage {
  bldrSyncManifest?: boolean
}

// BrowserIndexRefreshMessage asks the SW to refresh the runtime browser shell.
export interface BrowserIndexRefreshMessage {
  bldrRefreshBrowserIndex?: boolean
}

// BrowserFetchSourceKind classifies the ServiceWorker fetch authority.
export type BrowserFetchSourceKind =
  | 'root-document'
  | 'browser-index'
  | 'boot-asset'
  | 'release-asset'
  | 'plugin-dist'
  | 'plugin-assets'
  | 'quickjs-runtime-asset'
  | 'bldr-runtime'
  | 'native-fetch'

// BrowserFetchSource records the typed owner for a ServiceWorker request.
export interface BrowserFetchSource {
  kind: BrowserFetchSourceKind
  path: string
  sameOrigin: boolean
  runtime: boolean
}

// BrowserControlCacheRowKind names fixed rows in the control cache.
export type BrowserControlCacheRowKind =
  | 'root-document'
  | 'browser-index'
  | 'stable-boot-asset'
  | 'browser-release-state'

// BrowserControlCacheRow records a fixed control-cache row.
export interface BrowserControlCacheRow {
  kind: BrowserControlCacheRowKind
  path: string
  cacheName: typeof controlCacheName
}

// BrowserRuntimeFetchErrorCode is the bounded failure vocabulary for runtime fetches.
export type BrowserRuntimeFetchErrorCode =
  | 'no-ready-document'
  | 'resume-unavailable'
  | 'stream-open-timeout'
  | 'generation-closed'
  | 'runtime-unavailable'
  | 'plugin-asset-missing'
  | 'plugin-asset-unavailable'
  | 'request-canceled'

// BrowserPluginAssetFetchResultCode is the plugin asset lease result surfaced
// to browser importers and QuickJS backend asset loading.
export type BrowserPluginAssetFetchResultCode =
  | 'live'
  | 'missing'
  | 'unavailable'
  | 'generation-closed'
  | 'runtime-unavailable'
  | 'canceled'

// BrowserRuntimeFetchError is serialized to runtime fetch failure responses.
export interface BrowserRuntimeFetchError {
  code: BrowserRuntimeFetchErrorCode
  source: BrowserFetchSourceKind
  path: string
  message: string
  status: number
  pluginAssetFetchResult?: BrowserPluginAssetFetchResultCode
}

export interface BrowserRuntimeFetchFailureInput {
  status?: number
  message?: string
  aborted?: boolean
}

export interface BrowserRuntimeFetchClientTracker {
  hasRuntimeFetchRelay(): boolean
}

// resetServiceWorkerTestState clears module-level cache handles for unit tests.
export function resetServiceWorkerTestState(): void {
  CACHES[controlCacheName] = undefined
  browserReleaseSyncInFlight = null
  firstWebDocumentMessageMarked = false
}

let browserReleaseSyncInFlight: Promise<BrowserReleaseState> | null = null
let firstWebDocumentMessageMarked = false

const browserControlCacheRows: Record<
  BrowserControlCacheRowKind,
  BrowserControlCacheRow
> = {
  'root-document': {
    kind: 'root-document',
    path: rootNavigationPath,
    cacheName: controlCacheName,
  },
  'browser-index': {
    kind: 'browser-index',
    path: browserIndexPath,
    cacheName: controlCacheName,
  },
  'stable-boot-asset': {
    kind: 'stable-boot-asset',
    path: bootAssetPath,
    cacheName: controlCacheName,
  },
  'browser-release-state': {
    kind: 'browser-release-state',
    path: browserReleaseStatePath,
    cacheName: controlCacheName,
  },
}

// getBrowserControlCacheRow returns the typed fixed row for a control-cache entry.
export function getBrowserControlCacheRow(
  kind: BrowserControlCacheRowKind,
): BrowserControlCacheRow {
  return browserControlCacheRows[kind]
}

function buildCacheRequest(path: string): Request {
  return new Request(new URL(path, baseURL).toString())
}

function buildControlCacheRequest(row: BrowserControlCacheRow): Request {
  return buildCacheRequest(row.path)
}

function canCacheRequest(request: Request): boolean {
  return isBrowserCacheSupportedURL(request.url)
}

function canCacheBrowserReleaseRequests(): boolean {
  return isBrowserCacheSupportedURL(new URL(bootAssetPath, baseURL))
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

async function readCachedJson<T>(
  row: BrowserControlCacheRow,
): Promise<T | null> {
  const request = buildControlCacheRequest(row)
  if (!canCacheRequest(request)) {
    return null
  }
  const cache = await getControlCache()
  const response = await cache.match(request)
  if (!response) {
    return null
  }
  return (await response.json()) as T
}

async function writeCachedJson(
  row: BrowserControlCacheRow,
  value: unknown,
): Promise<void> {
  const request = buildControlCacheRequest(row)
  if (!canCacheRequest(request)) {
    return
  }
  const cache = await getControlCache()
  await cache.put(request, buildJsonResponse('GET', value))
}

async function loadBrowserReleaseState(): Promise<BrowserReleaseState> {
  const state = await readCachedJson<BrowserReleaseState>(
    getBrowserControlCacheRow('browser-release-state'),
  )
  if (!state) {
    return createEmptyBrowserReleaseState()
  }
  if (state.schemaVersion !== BROWSER_RELEASE_STATE_SCHEMA_VERSION) {
    return createEmptyBrowserReleaseState()
  }
  return state
}

async function saveBrowserReleaseState(
  state: BrowserReleaseState,
): Promise<void> {
  await writeCachedJson(
    getBrowserControlCacheRow('browser-release-state'),
    state,
  )
}

async function cacheStableBootAsset(): Promise<void> {
  const request = buildControlCacheRequest(
    getBrowserControlCacheRow('stable-boot-asset'),
  )
  if (!canCacheRequest(request)) {
    return
  }

  let response: Response
  try {
    response = await fetch(
      new Request(request.url, {
        cache: 'reload',
      }),
    )
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
  await cache.put(request, response.clone())
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
  const cache = await caches.open(
    buildGenerationCacheName(release.generationId),
  )
  for (const path of buildReleaseCachePaths(release)) {
    const request = buildCacheRequest(path)
    if (!canCacheRequest(request)) {
      console.warn(
        'ServiceWorker: %s: refusing to stage %s for %s, cache scheme unsupported',
        serviceWorkerId,
        path,
        release.generationId,
      )
      return false
    }
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
  if (!canCacheBrowserReleaseRequests()) {
    return createEmptyBrowserReleaseState()
  }

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

async function matchStableBootAsset(
  request: Request,
): Promise<Response | null> {
  const cacheRequest = buildControlCacheRequest(
    getBrowserControlCacheRow('stable-boot-asset'),
  )
  if (!canCacheRequest(cacheRequest)) {
    return null
  }
  const cache = await getControlCache()
  const response = await cache.match(cacheRequest)
  if (!response) {
    return null
  }
  return responseForMethod(request, response)
}

async function matchBrowserIndexCache(
  request: Request,
): Promise<Response | null> {
  const cacheRequest = buildControlCacheRequest(
    getBrowserControlCacheRow('browser-index'),
  )
  if (!canCacheRequest(cacheRequest)) {
    return null
  }
  const cache = await getControlCache()
  const response = await cache.match(cacheRequest)
  if (!response) {
    return null
  }
  return responseForMethod(request, response)
}

async function matchRootNavigationCache(
  request: Request,
): Promise<Response | null> {
  const cacheRequest = buildControlCacheRequest(
    getBrowserControlCacheRow('root-document'),
  )
  if (!canCacheRequest(cacheRequest)) {
    return null
  }
  const cache = await getControlCache()
  const response = await cache.match(cacheRequest)
  if (!response) {
    return null
  }
  return responseForMethod(request, response)
}

async function cacheBrowserIndexResponse(response: Response): Promise<void> {
  if (!response.ok) {
    return
  }
  const cache = await getControlCache()
  const request = buildControlCacheRequest(
    getBrowserControlCacheRow('browser-index'),
  )
  if (canCacheRequest(request)) {
    await cache.put(request, response.clone())
  }
}

async function cacheRootNavigationResponse(response: Response): Promise<void> {
  if (!response.ok) {
    return
  }
  const request = buildControlCacheRequest(
    getBrowserControlCacheRow('root-document'),
  )
  if (!canCacheRequest(request)) {
    return
  }
  const cache = await getControlCache()
  await cache.put(request, response.clone())
}

// refreshBrowserIndexCache fetches and stores the runtime browser shell.
export async function refreshBrowserIndexCache(
  clientId: string,
): Promise<Response> {
  const request = buildControlCacheRequest(
    getBrowserControlCacheRow('browser-index'),
  )
  const response = await proxyFetch(swHost, request, clientId)
  if (response.ok) {
    await cacheBrowserIndexResponse(response)
  }
  return response
}

async function matchPromotedGenerationResponse(
  request: Request,
): Promise<Response | null> {
  if (!canCacheBrowserReleaseRequests()) {
    return null
  }

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
    const cache = await caches.open(
      buildGenerationCacheName(release.generationId),
    )
    const candidates = isNavigation
      ? buildOfflineNavigationFallbacks(pathname, release)
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

async function matchPromotedCurrentGenerationResponse(
  request: Request,
): Promise<Response | null> {
  if (!canCacheBrowserReleaseRequests()) {
    return null
  }

  const pathname = normalizeReleasePath(new URL(request.url).pathname)
  const state = await loadBrowserReleaseState()
  const release = state.promotedCurrent
  if (!release) {
    return null
  }
  if (!new Set(buildReleaseCachePaths(release)).has(pathname)) {
    return null
  }

  const cache = await caches.open(
    buildGenerationCacheName(release.generationId),
  )
  const response = await cache.match(buildCacheRequest(pathname))
  if (!response) {
    return null
  }
  return responseForMethod(request, response)
}

// isStaticPluginAssetSource reports whether the fetch targets an immutable
// static plugin asset (plugin-assets / plugin-dist / quickjs-runtime-asset),
// excluding the dynamic plugin HTTP path, which is request-specific and must
// always reach the runtime.
function isStaticPluginAssetSource(source: BrowserFetchSource): boolean {
  return (
    isPluginRuntimeFetchSourceKind(source.kind) &&
    !source.path.startsWith(pluginHttpPathPrefix)
  )
}

// staticPluginAssetCacheRequest keys a static plugin asset by its path and
// query within the current generation cache. The asset URL is stable across
// builds (the plugin id prefix and lazy-chunk name are not content-hashed), so
// the generation cache scope is what keeps a promoted build from serving an
// earlier build's chunk.
function staticPluginAssetCacheRequest(request: Request): Request {
  const url = new URL(request.url)
  return buildCacheRequest(url.pathname + url.search)
}

// cacheStaticPluginAsset stores a successfully fetched static plugin asset in
// the current generation cache so it can be served across a runtime relay gap,
// the window where the page client that owned the relay is closing and the
// next document has not yet registered its relay. Without it, a lazy chunk
// import issued during that window has no live runtime client and fails.
async function cacheStaticPluginAsset(
  request: Request,
  response: Response,
): Promise<void> {
  if (request.method !== 'GET' || !response.ok) {
    return
  }
  if (!canCacheBrowserReleaseRequests()) {
    return
  }
  const state = await loadBrowserReleaseState()
  const release = state.promotedCurrent
  if (!release) {
    return
  }
  const cacheRequest = staticPluginAssetCacheRequest(request)
  if (!canCacheRequest(cacheRequest)) {
    return
  }
  const cache = await caches.open(
    buildGenerationCacheName(release.generationId),
  )
  await cache.put(cacheRequest, response)
}

// matchStaticPluginAsset returns a static plugin asset cached for the current
// generation, the relay-gap fallback. Scoped to the promoted-current
// generation so a stale chunk from an earlier build is never served; the cache
// for a superseded generation is pruned on promotion.
async function matchStaticPluginAsset(
  request: Request,
): Promise<Response | null> {
  if (!canCacheBrowserReleaseRequests()) {
    return null
  }
  const state = await loadBrowserReleaseState()
  const release = state.promotedCurrent
  if (!release) {
    return null
  }
  const cache = await caches.open(
    buildGenerationCacheName(release.generationId),
  )
  const response = await cache.match(staticPluginAssetCacheRequest(request))
  if (!response) {
    return null
  }
  return responseForMethod(request, response)
}

export async function handleBrowserReleaseRequest(
  ev: FetchEvent,
): Promise<Response> {
  const request = ev.request
  const state = await loadBrowserReleaseState()
  if (state.promotedCurrent) {
    const startTime = performance.now()
    const latestReleasePromise = fetchLatestBrowserRelease()
    const raceWinner = await Promise.race([
      latestReleasePromise,
      new Promise<typeof browserReleaseNetworkRaceTimedOut>((resolve) => {
        setTimeout(
          () => resolve(browserReleaseNetworkRaceTimedOut),
          browserReleaseNetworkRaceTimeoutMs,
        )
      }),
    ])
    if (raceWinner !== browserReleaseNetworkRaceTimedOut) {
      if (raceWinner) {
        ev.waitUntil(syncLatestBrowserRelease(raceWinner))
        return buildJsonResponse(request.method, raceWinner)
      }
      ev.waitUntil(syncLatestBrowserRelease())
      return buildJsonResponse(request.method, state.promotedCurrent)
    }
    ev.waitUntil(
      latestReleasePromise.then((lateRelease) => {
        if (lateRelease) {
          console.info(
            'ServiceWorker: %s: browser release manifest fetch missed %dms budget: latency=%dms',
            serviceWorkerId,
            browserReleaseNetworkRaceTimeoutMs,
            Math.round(performance.now() - startTime),
          )
          return syncLatestBrowserRelease(lateRelease)
        }
        return syncLatestBrowserRelease()
      }),
    )
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
    client.postMessage({
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
  markStartupBoundary('service-worker.install-start', {
    source: 'service-worker',
    serviceWorkerId,
  })
  await self.skipWaiting()
  markStartupBoundary('service-worker.install-ready', {
    source: 'service-worker',
    serviceWorkerId,
  })
}

// swActivate is called when the service worker becomes active.
async function swActivate() {
  markStartupBoundary('service-worker.activate-start', {
    source: 'service-worker',
    serviceWorkerId,
  })
  // Claim all clients.

  await self.clients.claim()
  await getControlCache()
  await syncLatestBrowserRelease()
  markStartupBoundary('service-worker.activate-ready', {
    source: 'service-worker',
    serviceWorkerId,
  })
}

// handleServiceWorkerMessage routes a page message to the ServiceWorker owner.
export function handleServiceWorkerMessage(
  ev: ExtendableMessageEvent,
  deps: ServiceWorkerMessageDeps,
): void {
  if (isBrowserReleaseSyncMessage(ev.data)) {
    if (browserReleaseSyncInFlight) {
      return
    }
    const syncPromise = deps.syncLatestBrowserRelease().finally(() => {
      if (browserReleaseSyncInFlight === syncPromise) {
        browserReleaseSyncInFlight = null
      }
    })
    browserReleaseSyncInFlight = syncPromise
    ev.waitUntil(syncPromise)
    return
  }

  if (isBrowserIndexRefreshMessage(ev.data)) {
    const senderId = (ev.source as Client)?.id || ''
    ev.waitUntil(deps.refreshBrowserIndexCache(senderId))
    return
  }

  // Cross-tab channel broker: handle hello/goodbye before WebDocument messages.
  if (isCrossTabMessage(ev.data)) {
    const senderId = (ev.source as Client)?.id
    if (senderId) {
      if (ev.data.crossTab === 'goodbye') {
        deps.fetchTracker.abortClient(
          senderId,
          new Error('service worker client closed'),
        )
      }
      ev.waitUntil(deps.handleCrossTabMessage(deps.clients, senderId, ev.data))
    }
    return
  }
  deps.webDocumentTracker.handleWebDocumentMessage(ev.data)
  if (!firstWebDocumentMessageMarked) {
    firstWebDocumentMessageMarked = true
    markStartupBoundary('service-worker.first-document-message', {
      source: 'service-worker',
      serviceWorkerId,
    })
  }
}

function isBrowserReleaseSyncMessage(
  data: unknown,
): data is BrowserReleaseSyncMessage {
  if (!data || typeof data !== 'object') {
    return false
  }
  return (data as BrowserReleaseSyncMessage).bldrSyncManifest === true
}

function isBrowserIndexRefreshMessage(
  data: unknown,
): data is BrowserIndexRefreshMessage {
  if (!data || typeof data !== 'object') {
    return false
  }
  return (data as BrowserIndexRefreshMessage).bldrRefreshBrowserIndex === true
}

function isNavigationRequest(request: Request): boolean {
  const accept = request.headers.get('Accept') ?? ''
  return (
    request.mode === 'navigate' ||
    request.destination === 'document' ||
    accept.includes('text/html')
  )
}

// classifyBrowserFetchSource returns the ServiceWorker owner for a request.
export function classifyBrowserFetchSource(
  request: Request,
  matchPrefixes: readonly string[] = BLDR_URI_PREFIXES,
): BrowserFetchSource {
  const requestURL = new URL(request.url)
  const path = requestURL.pathname
  const sameOrigin = isSwOrigin(requestURL.origin)
  const runtime =
    sameOrigin &&
    matchPrefixes.some((matchPrefix) => path.startsWith(matchPrefix))

  let kind: BrowserFetchSourceKind = 'native-fetch'
  if (
    sameOrigin &&
    path === rootNavigationPath &&
    isNavigationRequest(request)
  ) {
    kind = 'root-document'
  } else if (sameOrigin && path === browserIndexPath) {
    kind = 'browser-index'
  } else if (sameOrigin && path === bootAssetPath) {
    kind = 'boot-asset'
  } else if (sameOrigin && path === browserReleasePath) {
    kind = 'release-asset'
  } else if (sameOrigin && path.startsWith(pluginDistPathPrefix)) {
    kind = 'plugin-dist'
  } else if (
    sameOrigin &&
    (path.startsWith(pluginAssetsPathPrefix) ||
      path.startsWith(pluginWebPkgPathPrefix) ||
      path.startsWith(pluginHttpPathPrefix))
  ) {
    kind = 'plugin-assets'
  } else if (sameOrigin && path.startsWith(quickJSRuntimePathPrefix)) {
    kind = 'quickjs-runtime-asset'
  } else if (runtime) {
    kind = 'bldr-runtime'
  }

  return { kind, path, sameOrigin, runtime }
}

function isRuntimeFetchSource(source: BrowserFetchSource): boolean {
  return source.runtime || source.kind === 'browser-index'
}

function isPluginRuntimeFetchSource(source: BrowserFetchSource): boolean {
  return isPluginRuntimeFetchSourceKind(source.kind)
}

export function resolveBrowserRuntimeFetchClientId(
  clientId: string | undefined,
  source: BrowserFetchSource,
  tracker: BrowserRuntimeFetchClientTracker,
  serviceWorkerClientId: string,
): string {
  if (
    isPluginRuntimeFetchSource(source) &&
    !source.path.startsWith(pluginHttpPathPrefix) &&
    tracker.hasRuntimeFetchRelay()
  ) {
    return serviceWorkerClientId
  }
  if (clientId) {
    return clientId
  }
  if (isPluginRuntimeFetchSource(source) && tracker.hasRuntimeFetchRelay()) {
    return serviceWorkerClientId
  }
  return ''
}

function isPluginRuntimeFetchSourceKind(kind: BrowserFetchSourceKind): boolean {
  return (
    kind === 'plugin-assets' ||
    kind === 'plugin-dist' ||
    kind === 'quickjs-runtime-asset'
  )
}

function browserRuntimeFetchStatusForCode(
  code: BrowserRuntimeFetchErrorCode,
  fallbackStatus: number | undefined,
): number {
  if (code === 'request-canceled') {
    return 499
  }
  if (code === 'stream-open-timeout') {
    return 504
  }
  if (code === 'generation-closed') {
    return 410
  }
  if (code === 'plugin-asset-missing') {
    return 404
  }
  if (code === 'runtime-unavailable') {
    return 503
  }
  if (fallbackStatus && fallbackStatus >= 400 && fallbackStatus <= 599) {
    return fallbackStatus
  }
  return 503
}

function pluginAssetFetchResultForErrorCode(
  code: BrowserRuntimeFetchErrorCode,
): BrowserPluginAssetFetchResultCode | undefined {
  switch (code) {
    case 'request-canceled':
      return 'canceled'
    case 'generation-closed':
      return 'generation-closed'
    case 'runtime-unavailable':
    case 'no-ready-document':
    case 'resume-unavailable':
    case 'stream-open-timeout':
      return 'runtime-unavailable'
    case 'plugin-asset-missing':
      return 'missing'
    case 'plugin-asset-unavailable':
      return 'unavailable'
  }
}

function isAbortFailure(message: string): boolean {
  return (
    message.includes('aborterror') ||
    message.includes('aborted') ||
    message.includes('client closed') ||
    message.includes('service worker client closed')
  )
}

function isResumeUnavailableFailure(message: string): boolean {
  return (
    message.includes('resume') &&
    (message.includes('not ready') ||
      message.includes('unavailable') ||
      message.includes('timed out') ||
      message.includes('timeout'))
  )
}

function isStreamOpenTimeoutFailure(message: string): boolean {
  return (
    message.includes('timeout opening stream with host') ||
    message.includes('unable to open stream with host') ||
    message.includes('timed out waiting') ||
    message.includes('proxied fetch response headers')
  )
}

function isGenerationClosedFailure(message: string): boolean {
  return (
    message.includes('generation closed') ||
    message.includes('runtime closed') ||
    message.includes('worker closed') ||
    message.includes('webruntimeclientinstance is closed') ||
    message.includes('closed before resume-ready')
  )
}

function isPluginAssetMissingFailure(message: string): boolean {
  return message.includes('missing') || message.includes('404 page not found')
}

function shouldNormalizeRuntimeFetchFailure(
  source: BrowserFetchSource,
  failure: BrowserRuntimeFetchFailureInput,
): boolean {
  if (isPluginRuntimeFetchSource(source)) {
    return true
  }
  const lowerMessage = (failure.message ?? '').toLowerCase()
  return (
    failure.aborted ||
    isAbortFailure(lowerMessage) ||
    isResumeUnavailableFailure(lowerMessage) ||
    isStreamOpenTimeoutFailure(lowerMessage) ||
    isGenerationClosedFailure(lowerMessage)
  )
}

// classifyBrowserRuntimeFetchError maps proxy/runtime failures to bounded fetch errors.
export function classifyBrowserRuntimeFetchError(
  source: Pick<BrowserFetchSource, 'kind' | 'path'>,
  failure: BrowserRuntimeFetchFailureInput = {},
): BrowserRuntimeFetchError {
  const message = failure.message || 'runtime fetch unavailable'
  const lowerMessage = message.toLowerCase()
  const code = classifyBrowserRuntimeFetchErrorCode(
    source.kind,
    failure,
    lowerMessage,
  )
  const pluginAssetFetchResult = isPluginRuntimeFetchSourceKind(source.kind)
    ? pluginAssetFetchResultForErrorCode(code)
    : undefined
  return {
    code,
    source: source.kind,
    path: source.path,
    message,
    status: browserRuntimeFetchStatusForCode(code, failure.status),
    pluginAssetFetchResult,
  }
}

function classifyBrowserRuntimeFetchErrorCode(
  sourceKind: BrowserFetchSourceKind,
  failure: BrowserRuntimeFetchFailureInput,
  lowerMessage: string,
): BrowserRuntimeFetchErrorCode {
  if (failure.aborted || isAbortFailure(lowerMessage)) {
    return 'request-canceled'
  }
  if (isGenerationClosedFailure(lowerMessage)) {
    return 'generation-closed'
  }
  if (!isPluginRuntimeFetchSourceKind(sourceKind)) {
    if (isResumeUnavailableFailure(lowerMessage)) {
      return 'resume-unavailable'
    }
    if (isStreamOpenTimeoutFailure(lowerMessage)) {
      return 'stream-open-timeout'
    }
    return 'plugin-asset-unavailable'
  }
  if (
    isResumeUnavailableFailure(lowerMessage) ||
    isStreamOpenTimeoutFailure(lowerMessage)
  ) {
    return 'runtime-unavailable'
  }
  if (failure.status === 404 && isPluginAssetMissingFailure(lowerMessage)) {
    return 'plugin-asset-missing'
  }
  return 'plugin-asset-unavailable'
}

async function readRuntimeFetchFailureMessage(
  response: Response,
): Promise<string> {
  try {
    const message = await response.clone().text()
    return message.trim() || response.statusText || 'runtime fetch unavailable'
  } catch (error) {
    return castToError(error, 'runtime fetch unavailable').message
  }
}

function buildBrowserRuntimeFetchErrorResponse(
  error: BrowserRuntimeFetchError,
  method: string,
): Response {
  const body =
    method === 'HEAD'
      ? null
      : JSON.stringify({
          schemaVersion: 1,
          code: error.code,
          source: error.source,
          path: error.path,
          message: error.message,
          pluginAssetFetchResult: error.pluginAssetFetchResult,
        })
  const headers = new Headers({
    'Cache-Control': 'no-store',
    'Content-Type': 'application/json; charset=utf-8',
    'X-Bldr-Fetch-Source': error.source,
    'X-Bldr-Runtime-Fetch-Error': error.code,
  })
  if (error.pluginAssetFetchResult) {
    headers.set(
      'X-Bldr-Plugin-Asset-Fetch-Result',
      error.pluginAssetFetchResult,
    )
  }
  return new Response(body, {
    status: error.status,
    headers,
  })
}

function buildNoReadyDocumentResponse(
  source: BrowserFetchSource,
  method: string,
): Response {
  return buildBrowserRuntimeFetchErrorResponse(
    {
      code: isPluginRuntimeFetchSource(source)
        ? 'runtime-unavailable'
        : 'no-ready-document',
      source: source.kind,
      path: source.path,
      message: 'runtime fetch requires a ready browser document',
      status: 503,
      pluginAssetFetchResult: isPluginRuntimeFetchSource(source)
        ? 'runtime-unavailable'
        : undefined,
    },
    method,
  )
}

function addPluginAssetFetchResultHeader(
  response: Response,
  source: BrowserFetchSource,
  result: BrowserPluginAssetFetchResultCode,
): Response {
  if (!isPluginRuntimeFetchSource(source)) {
    return response
  }
  const headers = new Headers(response.headers)
  headers.set('X-Bldr-Fetch-Source', source.kind)
  headers.set('X-Bldr-Plugin-Asset-Fetch-Result', result)
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  })
}

async function normalizeRuntimeFetchResponse(
  source: BrowserFetchSource,
  request: Request,
  response: Response,
  abortSignal?: AbortSignal,
): Promise<Response> {
  if (response.ok) {
    return addPluginAssetFetchResultHeader(response, source, 'live')
  }
  if (!isRuntimeFetchSource(source)) {
    return response
  }
  if (
    !isPluginRuntimeFetchSource(source) &&
    source.kind !== 'browser-index' &&
    source.kind !== 'bldr-runtime'
  ) {
    return response
  }
  const message = await readRuntimeFetchFailureMessage(response)
  const failure = {
    status: response.status,
    message,
    aborted: request.signal.aborted || abortSignal?.aborted,
  }
  if (!shouldNormalizeRuntimeFetchFailure(source, failure)) {
    return response
  }
  const error = classifyBrowserRuntimeFetchError(source, {
    status: failure.status,
    message: failure.message,
    aborted: failure.aborted,
  })
  return buildBrowserRuntimeFetchErrorResponse(error, request.method)
}

async function proxyBrowserRuntimeFetch(
  source: BrowserFetchSource,
  request: Request,
  clientId: string,
  opts?: {
    abortSignal?: AbortSignal
    headerTimeoutMs?: number
  },
): Promise<Response> {
  const response = await proxyFetch(swHost, request, clientId, opts)
  return normalizeRuntimeFetchResponse(
    source,
    request,
    response,
    opts?.abortSignal,
  )
}

// isSwOrigin checks if the given origin matches the local origin.
function isSwOrigin(origin: string): boolean {
  return origin === self.location.origin
}

// swFetch is called when the page attempts to fetch a resource.
export async function swFetch(
  ev: FetchEvent,
  matchPrefixes = BLDR_URI_PREFIXES,
): Promise<Response> {
  const request = ev.request
  let requestURL: URL
  try {
    requestURL = new URL(request.url)
  } catch (error) {
    console.warn(
      'ServiceWorker: %s: malformed fetch request URL: %s: %s',
      serviceWorkerId,
      request.url,
      castToError(error, 'invalid URL').message,
    )
    return new Response('malformed request URL', {
      status: 400,
    })
  }
  const requestPath = requestURL.pathname
  const source = classifyBrowserFetchSource(request, matchPrefixes)

  if (source.kind === 'release-asset' && requestPath === browserReleasePath) {
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

  const useRuntimeFetch = source.runtime

  if (!useRuntimeFetch) {
    const promotedResponse =
      await matchPromotedCurrentGenerationResponse(request)
    if (promotedResponse) {
      return promotedResponse
    }

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

    let response: Response | null
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
      if (source.kind === 'root-document') {
        const cachedRoot = await matchRootNavigationCache(request)
        if (cachedRoot) {
          return cachedRoot
        }
      }

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

    if (source.kind === 'root-document' && response) {
      await cacheRootNavigationResponse(response)
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
  if (requestPath === browserIndexPath) {
    const response = await proxyBrowserRuntimeFetch(
      source,
      request,
      ev.clientId || '',
    )
    if (response.ok) {
      await cacheBrowserIndexResponse(response)
      return response
    }
    const cached = await matchBrowserIndexCache(request)
    if (cached) {
      return cached
    }
    return response
  }
  const staticPluginAsset = isStaticPluginAssetSource(source)
  // A static plugin asset is a content-hashed plugin frontend chunk served only
  // by the running plugin runtime; on first load it is in no cache. During the
  // page-client close / relay-establishment gap the relay is briefly absent or
  // resolves to a dying client, which fails the chunk fetch and breaks an
  // in-flight navigation. Wait once for the next relay (event-driven, bounded)
  // and retry instead of failing.
  for (let attempt = 0; attempt < 2; attempt++) {
    const runtimeFetchClientId = resolveBrowserRuntimeFetchClientId(
      ev.clientId || '',
      source,
      webDocumentTracker,
      serviceWorkerLogicalId,
    )
    if (!runtimeFetchClientId) {
      if (staticPluginAsset) {
        const cached = await matchStaticPluginAsset(request)
        if (cached) {
          return cached
        }
        if (
          attempt === 0 &&
          (await webDocumentTracker.waitForRuntimeFetchRelay(
            browserRuntimeFetchRelayWaitMs,
          ))
        ) {
          continue
        }
      }
      return buildNoReadyDocumentResponse(source, request.method)
    }

    const trackedFetch =
      serviceWorkerFetchTracker.trackFetch(runtimeFetchClientId)
    let response: Response
    try {
      response = await proxyBrowserRuntimeFetch(
        source,
        request,
        runtimeFetchClientId,
        {
          abortSignal: trackedFetch.abortController.signal,
          headerTimeoutMs: browserRuntimeFetchHeaderTimeoutMs,
        },
      ).finally(() => trackedFetch.release())
    } catch (err) {
      if (staticPluginAsset) {
        const cached = await matchStaticPluginAsset(request)
        if (cached) {
          return cached
        }
        // The proxy reached a relay document but its runtime client channel was
        // torn down (e.g. the relaying document closed and the client is
        // rerouting through a surviving document). Wait for the runtime client
        // to reconnect, not merely for a document to exist, before retrying.
        if (
          attempt === 0 &&
          (await webDocumentTracker.waitForRuntimeClientReady(
            browserRuntimeFetchRelayWaitMs,
          ))
        ) {
          continue
        }
      }
      throw err
    }

    if (staticPluginAsset) {
      if (response.ok) {
        ev.waitUntil(cacheStaticPluginAsset(request, response.clone()))
        return response
      }
      const cached = await matchStaticPluginAsset(request)
      if (cached) {
        return cached
      }
    }
    return response
  }
  // The loop returns or throws within two attempts; this satisfies control flow.
  return buildNoReadyDocumentResponse(source, request.method)

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
    handleServiceWorkerMessage(ev, {
      clients: self.clients,
      fetchTracker: serviceWorkerFetchTracker,
      webDocumentTracker,
      syncLatestBrowserRelease,
      refreshBrowserIndexCache,
      handleCrossTabMessage,
    })
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
