// bus-peer.ts - DedicatedWorker that joins a SAB bus and relays messages.
//
// Receives init message with: { busSab, pluginId, targetId, payload }
// Registers on bus, sends payload to targetId, then reads one message
// and posts it back to main thread.

import { SabBusEndpoint, BROADCAST_ID } from '../../../../web/bldr/sab-bus.js'

declare const self: DedicatedWorkerGlobalScope

interface InitMsg {
  busSab: SharedArrayBuffer
  pluginId: number
  // If set, send this payload to targetId after registering.
  targetId?: number
  payload?: number[]
  // If true, read one message and post it back.
  readOne?: boolean
}

self.onmessage = async (ev: MessageEvent<InitMsg>) => {
  const { busSab, pluginId, targetId, payload, readOne } = ev.data
  const opts = { slotSize: 256, numSlots: 32 }
  const endpoint = new SabBusEndpoint(busSab, pluginId, opts)
  endpoint.register()

  self.postMessage({ type: 'registered', pluginId })

  // Send if requested.
  if (targetId != null && payload) {
    endpoint.write(targetId, new Uint8Array(payload))
    self.postMessage({ type: 'sent', pluginId, targetId })
  }

  // Read if requested.
  if (readOne) {
    const msg = await endpoint.read()
    if (msg) {
      self.postMessage({
        type: 'received',
        pluginId,
        sourceId: msg.sourceId,
        targetId: msg.targetId,
        data: Array.from(msg.data),
      })
    }
  }
}
