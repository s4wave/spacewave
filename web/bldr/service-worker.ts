// @ts-check
/// <reference no-default-lib="true"/>
/// <reference lib="ES2015" />
/// <reference lib="webworker" />

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
const sw: ServiceWorkerGlobalScope = self as any;

// note: logs don't appear in console in firefox
console.log('bldr: service worker loaded')

// CURRENT_CACHES is the list of expected cache names in the caches list.
const CURRENT_CACHES: { [name: string]: string } = {}

// install is the beginning of service worker registration.
// setup resources such as offline caches.
// note: does not activate until some time after this returns.
async function install() {
  console.log('bldr: service worker installed')
}

// install event is called when service worker is installed.
sw.addEventListener('install', (ev: Event) => {
  const e = ev as ExtendableEvent
  e.waitUntil(install)
})

// activate is called when the service worker becomes active.
async function activate() {
  await sw.clients.claim()

  // Delete all caches that aren't named in CURRENT_CACHES.
  const expectedCacheNames = Object.keys(CURRENT_CACHES).map(function (key) {
    return CURRENT_CACHES[key];
  });

  const cacheNames = await caches.keys()
  for (const cacheName of cacheNames) {
    if (expectedCacheNames.indexOf(cacheName) === -1) {
      // If this cache name isn't present in the array of "expected" cache names, then delete it.
      console.log('bldr: service worker: deleting cache', cacheName);
      await caches.delete(cacheName)
    }
  }

  console.log('bldr: service worker activated')
}

// activate event is called when service worker is activated.
sw.addEventListener('activate', (ev: Event) => {
  const e = ev as ExtendableEvent
  e.waitUntil(activate)
})

// fetch is called when the page attempts to fetch a resource.
async function fetch(ev: FetchEvent): Promise<Response> {
  throw new Error('TODO bldr: service worker: fetch')
}

sw.addEventListener('fetch', (ev: FetchEvent) => {
  console.log('bldr: sw: handling fetch event', ev.request)
  ev.respondWith(fetch(ev))
})
