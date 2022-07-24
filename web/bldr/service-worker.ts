import { Client, Stream } from 'starpc'
import { ServiceWorkerHostClientImpl } from '../runtime/sw/sw.pb.js'
import { proxyFetch } from '../fetch/fetch.js'
import {
  WebDocumentToServiceWorker,
  ServiceWorkerToWebDocument,
  ClientToWebRuntime,
} from '../runtime/runtime.js'
import { timeoutPromise } from './timeout.js'
import { ChannelStream } from './channel.js'

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
declare let self: ServiceWorkerGlobalScope

// note: logs don't appear in console in firefox
const serviceWorkerId = `service-worker:${self.location.host}`
console.log(`bldr: service worker loaded: ${serviceWorkerId}`)

// CACHES is the list of caches.
const CACHES: { [name: string]: Cache | undefined } = { bldr: undefined }

// WebDocuments is the list of active WebDocument MessagePorts.
// TODO: for each remote WebDocument client: create a Client and ServiceWorkerHostClientImpl
const WebDocuments: Record<string, MessagePort> = {}

// activeWebRuntimeClient is the active MessagePort client to the WebRuntime.
let activeWebRuntimeClient: MessagePort | null = null

// openWebRuntimeClient attempts to acquire an active WebRuntimeClient message port.
async function openWebRuntimeClient(): Promise<MessagePort> {
  if (activeWebRuntimeClient) {
    try {
      activeWebRuntimeClient.postMessage('ping')
      return activeWebRuntimeClient
    } catch {
      // closed by remote
      console.log('ServiceWorker: client for WebRuntime was closed')
      activeWebRuntimeClient = null
    }
  }
  for (const webDocumentId of Object.keys(WebDocuments)) {
    const webDocumentPort = WebDocuments[webDocumentId]
    if (!webDocumentPort) {
      continue
    }
    const msgChannel = new MessageChannel()
    try {
      // request that we open the connection to the web runtime.
      webDocumentPort.postMessage(
        <ServiceWorkerToWebDocument>{
          from: serviceWorkerId,
          connectWebRuntime: msgChannel.port2,
        },
        [msgChannel.port2]
      )
    } catch (err) {
      // message port must be closed.
      console.error(
        `ServiceWorker: sending message to WebDocument failed: ${webDocumentId}`,
        err
      )
      delete WebDocuments[webDocumentId]
      continue
    }
    // message was sent, it must have gone through correctly.
    console.log(
      `ServiceWorker: opened port with WebRuntime via WebDocument: ${webDocumentId}`
    )
    activeWebRuntimeClient = msgChannel.port1
    activeWebRuntimeClient.start()
    return activeWebRuntimeClient
  }
  throw new Error('unable to open web runtime client via any web document')
}

async function openWebRuntimeStream(): Promise<Stream> {
  const clientPort = await openWebRuntimeClient()
  const streamChannel = new MessageChannel()
  const streamConn = new ChannelStream<Uint8Array>(
    serviceWorkerId,
    streamChannel.port1,
    false
  )
  const msg = <ClientToWebRuntime>{
    from: serviceWorkerId,
    openStream: streamChannel.port2,
  }
  clientPort.postMessage(msg, [streamChannel.port2])
  await Promise.race([streamConn.waitRemoteOpen, timeoutPromise(3000)])
  return streamConn
}

// webRuntimeClient attempts to contact the WebRuntime over any of the WebDocument relays.
const webRuntimeClient = new Client(openWebRuntimeStream)
// swHost is the RPC client for the ServiceWorkerHost.
const swHost = new ServiceWorkerHostClientImpl(webRuntimeClient)

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
  console.log('bldr: service worker activated')
  await self.clients.claim()
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
      console.log(`ServiceWorker: added WebDocument client: ${msg.from}`)
      WebDocuments[msg.from] = msg.initPort
      msg.initPort.start()
      // open the client immediately if not already open.
      openWebRuntimeClient()
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
