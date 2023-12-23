import { Client } from 'starpc'
import { ServiceWorkerHostClientImpl } from '../runtime/sw/sw.pb.js'
import { proxyFetch } from '../fetch/fetch.js'
import {
  WebDocumentToServiceWorker,
  ServiceWorkerToWebDocument,
  ConnectWebRuntimeAck,
} from '../runtime/runtime.js'
import { WebRuntimeClient } from './web-runtime-client.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import { timeoutPromise } from './timeout.js'
import { BLDR_URI_PREFIXES } from './constants.js'

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
declare let self: ServiceWorkerGlobalScope

// note: logs don't appear in console in firefox
const serviceWorkerId = `service-worker:${self.location.host}`
console.log(`bldr: service worker loaded: ${serviceWorkerId}`)

// CACHES is the list of caches.
const CACHES: { [name: string]: Cache | undefined } = { bldr: undefined }

// WebDocuments is the list of active WebDocument MessagePorts.
const WebDocuments: Record<string, MessagePort> = {}
// WebDocumentWaiters are callbacks waiting for the next WebDocument.
const WebDocumentWaiters: ((docID: string) => void)[] = []

// lastWebDocumentIdx was the last index used from WebDocuments.
let lastWebDocumentIdx = 0
// lastWebDocumentId was the last web document id used from WebDocuments.
let lastWebDocumentId: string | undefined

// openWebRuntimeClient attempts to open a client via one of the WebDocuments.
async function openWebRuntimeClient(
  init: WebRuntimeClientInit,
): Promise<MessagePort> {
  const webDocumentIds = Object.keys(WebDocuments)
  for (let i = 0; i < webDocumentIds.length; i++) {
    const x = (i + lastWebDocumentIdx + 1) % webDocumentIds.length
    const webDocumentId = webDocumentIds[x]
    const webDocumentPort = WebDocuments[webDocumentId]
    if (!webDocumentPort) {
      delete WebDocuments[webDocumentId]
      continue
    }
    const ackChannel = new MessageChannel()
    const ackPromise = new Promise<ConnectWebRuntimeAck>((resolve) => {
      const ackPort = ackChannel.port1
      ackPort.onmessage = (ev) => {
        const data: ConnectWebRuntimeAck = ev.data
        if (!data || !data.from) {
          return
        }
        resolve(data)
      }
      ackPort.start()
    })
    try {
      console.log(
        `ServiceWorker: ${serviceWorkerId} connecting via ${webDocumentId}`,
      )
      // request that we open the connection to the web runtime.
      // NOTE: this does not necessarily throw an error if the remote WebDocument is closed.
      webDocumentPort.postMessage(
        <ServiceWorkerToWebDocument>{
          from: init.clientUuid,
          connectWebRuntime: ackChannel.port2,
        },
        [ackChannel.port2],
      )
      // wait for the ack.
      const result = await Promise.race([ackPromise, timeoutPromise(1000)])
      if (!result) {
        throw new Error('timed out waiting for ack from WebDocument')
      }
      console.log(
        `ServiceWorker: opened port with WebRuntime via WebDocument: ${webDocumentId}`,
      )
      lastWebDocumentIdx = x
      lastWebDocumentId = webDocumentId
      return result.webRuntimePort
    } catch (err) {
      // message port must be closed.
      console.error(
        `ServiceWorker: connecting via WebDocument failed: ${webDocumentId}`,
        err,
      )
      delete WebDocuments[webDocumentId]
      continue
    }
  }

  // construct a promise to catch any new incoming WebDocument client
  const waitPromise = new Promise<MessagePort>((resolve) => {
    // try again once a new WebDocument is added.
    WebDocumentWaiters.push(() => {
      resolve(openWebRuntimeClient(init))
    })
  })

  // notify all WebDocument that we are looking for a connection to them.
  await self.clients.claim()
  const currClients = await self.clients.matchAll({ type: 'window' })
  console.log(
    'ServiceWorker: notifying %d clients we want a connection',
    currClients.length,
  )
  for (const client of currClients) {
    client.postMessage({ BLDR_INIT_SW: serviceWorkerId })
  }

  console.log('ServiceWorker: waiting for next WebDocument to proxy conn')
  return waitPromise
}

// webRuntimeClient manages the connection to the WebRuntime.
// note: the webRuntimeId here is ignored.
const webRuntimeClient = new WebRuntimeClient(
  '',
  serviceWorkerId,
  WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
  openWebRuntimeClient,
  null,
)

// swHostClient attempts to contact the WebRuntime over any of the WebDocument relays.
const swHostClient = new Client(
  webRuntimeClient.openStream.bind(webRuntimeClient),
)

// swHost is the RPC client for the ServiceWorkerHost.
const swHost = new ServiceWorkerHostClientImpl(swHostClient)

// install is the beginning of service worker registration.
// setup resources such as offline caches.
// note: does not activate until some time after this returns.
async function swInstall() {
  await self.skipWaiting()
  console.log('bldr: service worker installed')
}

// swActivate is called when the service worker becomes active.
async function swActivate() {
  // Delete all caches that aren't named in CACHES.
  const expectedCacheNames = Object.keys(CACHES)
  const cacheNames = await caches.keys()
  for (const cacheName of cacheNames) {
    if (expectedCacheNames.indexOf(cacheName) === -1) {
      // If this cache name isn't present in the array of "expected" cache names, then delete it.
      console.log('bldr: service worker: deleting cache', cacheName)
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

  console.log('bldr: service worker activated')
}

// isSwOrigin checks if the given origin matches the local origin.
function isSwOrigin(origin: string): boolean {
  return origin === self.location.origin
}

// swFetch is called when the page attempts to fetch a resource.
async function swFetch(ev: FetchEvent): Promise<Response> {
  const matchPrefixes = BLDR_URI_PREFIXES
  const request = ev.request
  const requestURL = new URL(request.url)
  const requestOrigin = requestURL.origin
  const requestPath = requestURL.pathname

  let useRuntimeFetch = false
  if (isSwOrigin(requestOrigin)) {
    for (const matchPrefix of matchPrefixes) {
      if (requestPath.startsWith(matchPrefix)) {
        useRuntimeFetch = true
        break
      }
    }
  }
  if (!useRuntimeFetch) {
    // Use the built-in browser fetch.
    return fetch(ev.request)
  }

  console.log(
    'ServiceWorker: forwarding fetch request to runtime',
    request.url.toString(),
  )
  return proxyFetch(swHost, request, ev.clientId)

  /*
  Not working with custom app:// scheme in Electron.
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
    const msg: WebDocumentToServiceWorker = ev.data
    if (typeof msg !== 'object' || !msg.from) {
      return
    }
    if (msg.initPort && ev.ports.length) {
      const webDocumentId = msg.from
      const port = msg.initPort
      WebDocuments[msg.from] = port
      for (const waiter of WebDocumentWaiters) {
        waiter(webDocumentId)
      }
      WebDocumentWaiters.length = 0
      console.log(`ServiceWorker: added WebDocument client: ${webDocumentId}`)
      port.onmessage = (ev) => {
        const data = ev.data
        if (data === 'close') {
          const closePort = WebDocuments[webDocumentId]
          if (closePort) {
            closePort.close()
            console.log(
              `ServiceWorker: closed WebDocument client: ${webDocumentId}`,
            )
            delete WebDocuments[webDocumentId]
            if (lastWebDocumentId === webDocumentId) {
              lastWebDocumentId = undefined
              lastWebDocumentIdx = 0
              webRuntimeClient.close()
            }
          }
        }
      }
      port.start()
    }
  })

  // fetch event is called when a URL within the scope is accessed.
  self.addEventListener('fetch', (ev: FetchEvent) => {
    ev.respondWith(swFetch(ev))
  })
}

// IS_SERVICE_WORKER indicates if initServiceWorker was called.
// If we are not a service worker, don't register callbacks.
const IS_SERVICE_WORKER = !!self && !!self.clients
if (IS_SERVICE_WORKER) {
  initServiceWorker()
}
