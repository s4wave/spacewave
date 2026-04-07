// cross-tab.ts - Cross-tab MessagePort brokering test fixture.
//
// Registers the cross-tab ServiceWorker, sends "hello", receives brokered
// MessagePorts from peer tabs, and exchanges messages directly.

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      swRegistered: boolean
      peerCount: number
      messagesReceived: string[]
    }
    // sendToPeers is called from the Go test to send a message to all peers.
    sendToPeers: (msg: string) => void
  }
}

const peers = new Map<string, MessagePort>()
const messagesReceived: string[] = []

function addPeer(peerId: string, port: MessagePort) {
  const existing = peers.get(peerId)
  if (existing) {
    existing.close()
  }
  peers.set(peerId, port)
  port.onmessage = (ev: MessageEvent) => {
    messagesReceived.push(JSON.stringify(ev.data))
    updateResults()
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
    pass: true,
    detail: 'ok',
    swRegistered: true,
    peerCount: peers.size,
    messagesReceived: [...messagesReceived],
  }
}

window.sendToPeers = (msg: string) => {
  for (const [, port] of peers) {
    port.postMessage({ text: msg })
  }
}

async function run() {
  const log = document.getElementById('log')!
  navigator.serviceWorker.addEventListener('message', (ev: MessageEvent) => {
    const data = ev.data
    if (typeof data !== 'object' || !data.crossTab) return

    if (data.crossTab === 'direct-port') {
      const port = ev.ports[0]
      if (port) {
        addPeer(data.peerId, port)
      }
    } else if (data.crossTab === 'peer-gone') {
      removePeer(data.peerId)
    }
  })

  // Register the cross-tab ServiceWorker.
  // Vite bundles the SW as a classic script (no import/export statements),
  // so no type: 'module' needed. This ensures Firefox/WebKit compatibility.
  const reg = await navigator.serviceWorker.register('/cross-tab-sw.js')

  // Wait for the SW to become active.
  const sw = reg.active || reg.installing || reg.waiting
  if (!sw) {
    window.__results = {
      pass: false,
      detail: 'no SW instance after registration',
      swRegistered: false,
      peerCount: 0,
      messagesReceived: [],
    }
    log.textContent = 'DONE'
    return
  }

  await new Promise<void>((resolve) => {
    if (sw.state === 'activated') {
      resolve()
      return
    }
    sw.addEventListener('statechange', () => {
      if (sw.state === 'activated') resolve()
    })
  })

  // Ensure this page is controlled by the SW.
  if (!navigator.serviceWorker.controller) {
    await navigator.serviceWorker.ready
    // Claim may still be pending, wait briefly for controller.
    await new Promise<void>((resolve) => {
      if (navigator.serviceWorker.controller) {
        resolve()
        return
      }
      navigator.serviceWorker.addEventListener('controllerchange', () => resolve(), { once: true })
    })
  }

  // Send "hello" to the cross-tab broker.
  navigator.serviceWorker.controller!.postMessage({ crossTab: 'hello' })

  window.__results = {
    pass: true,
    detail: 'sw registered, waiting for peers',
    swRegistered: true,
    peerCount: 0,
    messagesReceived: [],
  }

  log.textContent = 'DONE'
}

run().catch((err) => {
  window.__results = {
    pass: false,
    detail: `error: ${err}`,
    swRegistered: false,
    peerCount: 0,
    messagesReceived: [],
  }
  document.getElementById('log')!.textContent = 'DONE'
})
