// rpc-peer.ts - DedicatedWorker that runs a StarPC echo server or client
// over the SAB bus.
//
// Receives init message with role, bus SAB, plugin IDs.
// Server: registers echo handler, accepts one stream, handles RPC.
// Client: waits for 'start' signal, calls Echo, reports result.

import { SabBusEndpoint, SabBusStream } from '../../../../web/bldr/sab-bus.js'
import { Server, Client, createHandler, createMux } from 'starpc'
import {
  EchoerDefinition,
  EchoerClient,
  EchoerServer,
} from 'starpc/echo'

declare const self: DedicatedWorkerGlobalScope

interface InitMsg {
  busSab: SharedArrayBuffer
  pluginId: number
  targetId: number
  role: 'server' | 'client'
}

const busOpts = { slotSize: 8192, numSlots: 64 }

self.onmessage = async (ev: MessageEvent<InitMsg | { type: 'start' }>) => {
  if ('type' in ev.data && ev.data.type === 'start') {
    // Client start signal handled below via promise.
    return
  }

  const init = ev.data as InitMsg
  const { busSab, pluginId, targetId, role } = init

  const endpoint = new SabBusEndpoint(busSab, pluginId, busOpts)
  endpoint.register()

  self.postMessage({ type: 'registered', pluginId, role })

  if (role === 'server') {
    // Create SabBusStream targeting the client.
    const stream = new SabBusStream(endpoint, targetId)

    // Set up StarPC echo server.
    const mux = createMux()
    mux.register(createHandler(EchoerDefinition, new EchoerServer()))
    const server = new Server(mux.lookupMethod)

    self.postMessage({ type: 'server-ready' })

    // Handle the single stream (blocks until stream closes).
    await server.rpcStreamHandler(stream)
    self.postMessage({ type: 'server-done' })
  } else {
    // Client: wait for start signal.
    await new Promise<void>((resolve) => {
      self.onmessage = (startEv: MessageEvent) => {
        if (startEv.data?.type === 'start') resolve()
      }
    })

    // Create SabBusStream targeting the server.
    const stream = new SabBusStream(endpoint, targetId)

    // Create StarPC client using the bus stream.
    const client = new Client(async () => stream)
    const echoer = new EchoerClient(client)

    // Make an echo RPC call.
    const response = await echoer.Echo({ body: 'hello via SAB bus' })
    self.postMessage({
      type: 'rpc-result',
      body: response.body,
    })

    stream.close()
  }
}
