import { Client, Stream } from 'starpc'
import { ServiceWorkerHostClientImpl } from '../runtime/sw/sw.pb.js'
import { ChannelStream } from './channel.js'
import { proxyFetch } from '../fetch/fetch.js'

// Default type of `self` is `WorkerGlobalScope & typeof globalThis`
// https://github.com/microsoft/TypeScript/issues/14877
declare let self: ServiceWorkerGlobalScope

// note: logs don't appear in console in firefox
console.log('bldr: service worker loaded')

// CURRENT_CACHES is the list of expected cache names in the caches list.
const CURRENT_CACHES: { [name: string]: string } = {}

// webRuntimePort contains the MessagePort to the leader runtime.
// updated / resolved when the leader notifies us of a updated port.
let webRuntimePort: MessagePort | undefined
let webRuntimePortPromise: Promise<MessagePort>
let resolveWebRuntimePort:
  | ((val?: MessagePort, err?: Error) => void)
  | undefined
function resetWebRuntimePort() {
  if (resolveWebRuntimePort) {
    return
  }
  if (webRuntimePort) {
    webRuntimePort.onmessage = null
    webRuntimePort.onmessageerror = null
    webRuntimePort.close()
  }
  webRuntimePortPromise = new Promise<MessagePort>((resolve, reject) => {
    resolveWebRuntimePort = (val?: MessagePort, err?: Error) => {
      resolveWebRuntimePort = undefined
      if (val) {
        webRuntimePort = val
        val.onmessage = (ev) => {
          if (ev && ev.data && typeof ev.data === 'object') {
            handleWebRuntimeMessage(ev.data as WebRuntimeMessage, ev.ports)
          }
        }
        resolve(val)
      } else {
        reject(err)
      }
    }
  })
}
resetWebRuntimePort()

// WebRuntimeMessage is a message sent on the web runtime channel.
interface WebRuntimeMessage {
  // openRpcStream requests to open a RPC stream with the attached MessagePort.
  openRpcStream?: boolean
}

// postWebRuntimeMessage posts a message to the MessagePort.
async function postWebRuntimeMessage(
  data: WebRuntimeMessage,
  xfer?: MessagePort[]
) {
  const port = await webRuntimePortPromise
  if (xfer && xfer.length) {
    port.postMessage(data, xfer)
  } else {
    port.postMessage(data)
  }
  // TODO
}

// handleWebRuntimeMessage handles an incoming message on the MessagePort.
function handleWebRuntimeMessage(
  data: WebRuntimeMessage,
  xfer?: readonly MessagePort[]
) {
  console.log('bldr: service worker: got message on channel', data, xfer)
  // TODO
}

// openStreamViaWebRuntime opens a RPC stream via the leader.
async function openStreamViaWebRuntime(): Promise<Stream> {
  const channel = new MessageChannel()
  const ourPort = channel.port1
  const remotePort = channel.port2
  // construct the message channel backed stream.
  const stream = new ChannelStream<Uint8Array>('sw', ourPort, false)
  // notify the leader
  postWebRuntimeMessage({ openRpcStream: true }, [remotePort])
  // wait for the stream to be fully opened
  await stream.waitRemoteOpen
  // return the stream
  return stream
}

// swHostClient is the RPC client for the HostRuntime.
const swHostClient = new Client(openStreamViaWebRuntime)
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

  console.log('DEBUG: service worker proxying request', requestURL)
  return proxyFetch(swHost, request, ev.clientId)
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
    const data = ev.data
    if (data === 'BLDR_CLAIM') {
      self.clients.claim()
      return
    }
    if (data === 'BLDR_INIT' && ev.ports.length) {
      if (!resolveWebRuntimePort) {
        resetWebRuntimePort()
      }
      console.log('bldr: service worker: initialized port')
      resolveWebRuntimePort!(ev.ports[0])
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
