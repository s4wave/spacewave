// cross-tab-rpc.ts - StarPC echo RPC over brokered cross-tab MessagePort.
//
// Two browser pages register with the ServiceWorker broker and get direct
// MessagePort channels. When the Go test calls callEcho(), the page creates
// a sub-channel (MessageChannel), sends one end to the peer through the
// brokered port, wraps the local end as a ChannelStream, and makes a StarPC
// Echo call. The peer accepts the relay port, wraps it, and runs the echo
// server. Verifies the full RPC round-trip across tabs.

import {
  Server,
  Client,
  ChannelStream,
  createHandler,
  createMux,
} from 'starpc'
import {
  EchoerDefinition,
  EchoerClient,
  EchoerServer,
} from 'starpc/echo'
import type { ChannelStreamOpts } from 'starpc'
import type { CrossTabBrokerMessage } from '../../../web/bldr/cross-tab-broker.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      swRegistered: boolean
      peerCount: number
      echoBody: string
    }
    __peers: Map<string, MessagePort>
    // callEcho is called from the Go test to initiate a cross-tab RPC.
    callEcho: (peerId: string, body: string) => Promise<string>
  }
}

const streamOpts: ChannelStreamOpts = {
  keepAliveMs: 5000,
  idleTimeoutMs: 10000,
}

// Expose peers map on window for Go test to read peer IDs.
const peers = new Map<string, MessagePort>()
window.__peers = peers

function isCrossTabBrokerMessage(data: unknown): data is CrossTabBrokerMessage {
  if (typeof data !== 'object' || data === null) return false
  const msg = data as Record<string, unknown>
  return (
    (msg.crossTab === 'direct-port' || msg.crossTab === 'peer-gone') &&
    typeof msg.peerId === 'string'
  )
}

// Set up StarPC echo server for incoming relay connections.
const mux = createMux()
mux.register(createHandler(EchoerDefinition, new EchoerServer()))
const server = new Server(mux.lookupMethod)

function addPeer(peerId: string, port: MessagePort) {
  const existing = peers.get(peerId)
  if (existing) existing.close()
  peers.set(peerId, port)

  // Handle relay sub-channels from this peer.
  port.onmessage = (ev: MessageEvent) => {
    if (ev.data?.type === 'relay' && ev.ports?.[0]) {
      const subPort = ev.ports[0]
      const stream = new ChannelStream(peerId, subPort, streamOpts)
      server.rpcStreamHandler(stream).catch(() => {})
    }
  }
  port.start()
  updateResults()
}

function removePeer(peerId: string) {
  const port = peers.get(peerId)
  if (port) {
    port.close()
    peers.delete(peerId)
    updateResults()
  }
}

function updateResults() {
  window.__results = {
    ...window.__results,
    peerCount: peers.size,
  }
}

// callEcho opens a sub-channel to a peer and makes a StarPC Echo call.
window.callEcho = async (peerId: string, body: string): Promise<string> => {
  const port = peers.get(peerId)
  if (!port) throw new Error(`no peer: ${peerId}`)

  // Create sub-channel: local port1 for client, send port2 to peer server.
  const { port1, port2 } = new MessageChannel()
  port.postMessage({ type: 'relay', port: port2 }, [port2])

  const stream = new ChannelStream('local', port1, streamOpts)
  const client = new Client(async () => stream)
  const echoer = new EchoerClient(client)

  const response = await echoer.Echo({ body })
  return response.body ?? ''
}

async function run() {
  const log = document.getElementById('log')!

  window.__results = {
    pass: false,
    detail: 'initializing',
    swRegistered: false,
    peerCount: 0,
    echoBody: '',
  }

  // Set up SW message listener before registration.
  navigator.serviceWorker.addEventListener('message', (ev: MessageEvent) => {
    const data = ev.data
    if (!isCrossTabBrokerMessage(data)) return

    if (data.crossTab === 'direct-port') {
      const port = ev.ports[0]
      if (port) addPeer(data.peerId, port)
    } else if (data.crossTab === 'peer-gone') {
      removePeer(data.peerId)
    }
  })

  // Register the cross-tab SW.
  const reg = await navigator.serviceWorker.register('/cross-tab-sw.js')
  const sw = reg.active || reg.installing || reg.waiting
  if (!sw) {
    window.__results = { pass: false, detail: 'no SW', swRegistered: false, peerCount: 0, echoBody: '' }
    log.textContent = 'DONE'
    return
  }

  await new Promise<void>((resolve) => {
    if (sw.state === 'activated') { resolve(); return }
    sw.addEventListener('statechange', () => {
      if (sw.state === 'activated') resolve()
    })
  })

  // Wait for controller.
  if (!navigator.serviceWorker.controller) {
    await navigator.serviceWorker.ready
    await new Promise<void>((resolve) => {
      if (navigator.serviceWorker.controller) { resolve(); return }
      navigator.serviceWorker.addEventListener('controllerchange', () => resolve(), { once: true })
    })
  }

  navigator.serviceWorker.controller!.postMessage({ crossTab: 'hello' })

  window.__results = {
    pass: true,
    detail: 'sw registered, waiting for peers',
    swRegistered: true,
    peerCount: 0,
    echoBody: '',
  }
  log.textContent = 'DONE'
}

run().catch((err) => {
  window.__results = {
    pass: false,
    detail: `error: ${err}`,
    swRegistered: false,
    peerCount: 0,
    echoBody: '',
  }
  document.getElementById('log')!.textContent = 'DONE'
})
