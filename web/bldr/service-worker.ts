import { castToError } from 'starpc'
import { ServiceWorkerHostClientImpl } from '../runtime/sw/sw.pb.js'
import { proxyFetch } from '../fetch/fetch.js'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { BLDR_CACHE_PATHS, BLDR_URI_PREFIXES } from './constants.js'
import { WebDocumentTracker } from './web-document-tracker.js'
import { ServiceWorkerToWebDocument } from 'web/runtime/runtime.js'

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
declare let self: ServiceWorkerGlobalScope

// note: logs don't appear in console in firefox
const serviceWorkerId = `service-worker:${self.location.host}`

// baseURL is the base URL to use for paths.
const baseURL = new URL(self.location.toString())

// CACHES is the list of caches.
const CACHES: { [name: string]: Cache | undefined } = { bldr: undefined }

// onWebDocumentsExhausted notifies all web documents we need a new connection.
const onWebDocumentsExhausted = async () => {
  await self.clients.claim()
  const currClients = await self.clients.matchAll({ type: 'window' })
  console.log(
    'ServiceWorker: %s: notifying %d clients we want a connection',
    serviceWorkerId,
    currClients.length,
  )
  for (const client of currClients) {
    client.postMessage(<ServiceWorkerToWebDocument>{
      from: serviceWorkerId,
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
)

// webRuntimeClient manages the connection to the WebRuntime.
const webRuntimeClient = webDocumentTracker.webRuntimeClient

// swHostClient attempts to contact the WebRuntime over any of the WebDocument relays.
const swHostClient = webRuntimeClient.rpcClient

// swHost is the RPC client for the ServiceWorkerHost.
const swHost = new ServiceWorkerHostClientImpl(swHostClient)

// install is the beginning of service worker registration.
// setup resources such as offline caches.
// note: does not activate until some time after this returns.
async function swInstall() {
  await self.skipWaiting()
}

// swActivate is called when the service worker becomes active.
async function swActivate() {
  // Delete all caches that aren't named in CACHES.
  const expectedCacheNames = Object.keys(CACHES)
  const cacheNames = await caches.keys()
  for (const cacheName of cacheNames) {
    if (expectedCacheNames.indexOf(cacheName) === -1) {
      // If this cache name isn't present in the array of "expected" cache names, then delete it.
      console.log(
        'ServiceWorker: %s: deleting unrecognized cache: %s',
        serviceWorkerId,
        cacheName,
      )
      await caches.delete(cacheName)
    }
  }
  for (const cacheName of expectedCacheNames) {
    if (!CACHES[cacheName]) {
      CACHES[cacheName] = await caches.open(cacheName)
    }
  }

  // Claim all clients.
  await self.clients.claim()

  // Fetch index.html to the cache
  const bldrCache = CACHES['bldr']
  if (bldrCache) {
    for (const cachePath of BLDR_CACHE_PATHS) {
      const fullURL = new URL(cachePath, baseURL)
      console.log(
        'ServiceWorker: %s: caching path: %s',
        serviceWorkerId,
        cachePath,
      )
      bldrCache
        .add(fullURL)
        .catch((error) =>
          console.warn(
            'ServiceWorker: %s: unable to cache path %s: %s',
            serviceWorkerId,
            cachePath,
            error,
          ),
        )
    }
  }
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

  const useRuntimeFetch =
    isSwOrigin(requestOrigin) &&
    matchPrefixes.some((matchPrefix) => requestPath.startsWith(matchPrefix))

  if (!useRuntimeFetch) {
    // Check the cache (for e.x. index.html)
    const bldrCache = CACHES['bldr']
    if (bldrCache) {
      const cacheResp = await bldrCache.match(request)
      if (cacheResp) {
        return cacheResp
      }
    }

    // Use the built-in browser fetch.
    console.log(
      'ServiceWorker: %s: using native fetch: %s',
      serviceWorkerId,
      request.url.toString(),
    )
    return fetch(ev.request)
  }

  console.log(
    'ServiceWorker: %s: forwarding fetch to runtime: %s',
    serviceWorkerId,
    request.url.toString(),
  )
  return proxyFetch(swHost, request, ev.clientId)

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
