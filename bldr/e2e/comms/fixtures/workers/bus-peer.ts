// bus-peer.ts - DedicatedWorker that joins a SAB bus and relays messages.
//
// Receives init message with: { busSab, pluginId, targetId, payload }
// Registers on bus, sends payload to targetId, then reads one message
// and posts it back to main thread.

import { SabBusEndpoint } from '../../../../web/bldr/sab-bus.js'

declare const self: DedicatedWorkerGlobalScope

interface InitMsg {
  busSab: SharedArrayBuffer
  pluginId: number
  stage?: string
  // If set, send this payload to targetId after registering.
  targetId?: number
  payload?: number[]
  // If true, read one message and post it back.
  readOne?: boolean
}

interface CloseMsg {
  type: 'close'
}

let endpoint: SabBusEndpoint | undefined
let pluginId = -1
let stage = 'unlabeled'

self.onmessage = async (ev: MessageEvent<InitMsg | CloseMsg>) => {
  if ('type' in ev.data && ev.data.type === 'close') {
    endpoint?.close()
    self.postMessage({ type: 'closed', pluginId, stage })
    return
  }

  const { busSab, targetId, payload, readOne } = ev.data
  pluginId = ev.data.pluginId
  stage = ev.data.stage ?? 'unlabeled'
  const opts = { slotSize: 256, numSlots: 32 }
  endpoint = new SabBusEndpoint(busSab, pluginId, opts)
  endpoint.register()

  self.postMessage({ type: 'registered', pluginId, stage })

  // Send if requested.
  if (targetId != null && payload) {
    self.postMessage({ type: 'write-started', pluginId, stage, targetId })
    await endpoint.write(targetId, new Uint8Array(payload))
    self.postMessage({ type: 'sent', pluginId, stage, targetId })
  }

  // Read if requested.
  if (readOne) {
    self.postMessage({ type: 'read-started', pluginId, stage })
    const msg = await endpoint.read()
    if (msg) {
      self.postMessage({
        type: 'received',
        pluginId,
        stage,
        sourceId: msg.sourceId,
        targetId: msg.targetId,
        data: Array.from(msg.data),
      })
      return
    }
    self.postMessage({ type: 'read-closed', pluginId, stage })
  }
}
