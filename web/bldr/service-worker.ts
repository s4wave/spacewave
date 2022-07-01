// mark this as a module
import {} from '.'

// TODO: We are limited by Snowpack currently and cannot bundle this properly.
// Use a separate esbuild step to bundle the service worker into a single ES2015 file.
// Currently snowpack is outputting module code, which is not allowed in ServiceWorker.
// For now, the limitation is that we ccannot use import or export statements here.

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
declare let self: ServiceWorkerGlobalScope

// note: logs don't appear in console in firefox
console.log('bldr: service worker loaded')

// CURRENT_CACHES is the list of expected cache names in the caches list.
const CURRENT_CACHES: { [name: string]: string } = {}

// install is the beginning of service worker registration.
// setup resources such as offline caches.
// note: does not activate until some time after this returns.
async function swInstall() {
  await self.skipWaiting()
  console.log('bldr: service worker installed')
}

// swActivate is called when the service worker becomes active.
async function swActivate() {
  // Claim all clients.
  await self.clients.claim()

  // Delete all caches that aren't named in CURRENT_CACHES.
  const expectedCacheNames = Object.keys(CURRENT_CACHES).map(function (key) {
    return CURRENT_CACHES[key]
  })

  const cacheNames = await caches.keys()
  for (const cacheName of cacheNames) {
    if (expectedCacheNames.indexOf(cacheName) === -1) {
      // If this cache name isn't present in the array of "expected" cache names, then delete it.
      console.log('bldr: service worker: deleting cache', cacheName)
      await caches.delete(cacheName)
    }
  }

  console.log('bldr: service worker activated')
}

// isSwOrigin checks if the given origin matches the local origin.
function isSwOrigin(origin: string): boolean {
  return origin === self.location.origin
}

// swFetch is called when the page attempts to fetch a resource.
async function swFetch(ev: FetchEvent): Promise<Response> {
  // Ignore any URLs that are outside of /b/.
  // (/b/ is short for /bldr/)
  const request = ev.request
  const requestURL = new URL(request.url)
  const requestOrigin = requestURL.origin
  const requestPath = requestURL.pathname
  if (!isSwOrigin(requestOrigin) || requestPath.indexOf('/b/') !== 0) {
    // Use the built-in browser fetch.
    return fetch(ev.request)
  }

  // throw new Error('TODO bldr: service worker: swFetch')
  const resp = new Response(null, {
    status: 500,
    statusText: 'TODO implement bldr service worker swFetch',
  })
  return resp
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

  // TODO: proof of concept of passing MessageChannel
  self.addEventListener('message', (ev: ExtendableMessageEvent) => {
    const data = ev.data
    console.log('service worker: got message', data)
  })

  // fetch event is called when a URL within the scope is accessed.
  self.addEventListener('fetch', (ev: FetchEvent) => {
    ev.respondWith(swFetch(ev))
  })
}

// IS_SERVICE_WORKER indicates if initServiceWorker was called.
const IS_SERVICE_WORKER = !!self && !!self.clients

// If we are not a service worker, don't register callbacks.
if (IS_SERVICE_WORKER) {
  initServiceWorker()
}
