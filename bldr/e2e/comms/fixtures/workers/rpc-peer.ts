// rpc-peer.ts - DedicatedWorker that runs a StarPC echo server or client
// over a SAB pair stream.
//
// Receives init message with role and one pair endpoint descriptor.
// Server: registers echo handler, accepts one stream, handles RPC.
// Client: waits for 'start' signal, calls Echo, reports result.

import { Server, Client, createHandler, createMux } from 'starpc'
import {
  EchoerDefinition,
  EchoerClient,
  EchoerServer,
} from 'starpc/echo'

import { SabRingStream } from '../../../../web/bldr/sab-ring-stream.js'

declare const self: DedicatedWorkerGlobalScope

interface InitMsg {
  pairId: string
  txSab: SharedArrayBuffer
  rxSab: SharedArrayBuffer
  role: 'server' | 'client'
}

self.onmessage = async (ev: MessageEvent<InitMsg | { type: 'start' }>) => {
  if ('type' in ev.data && ev.data.type === 'start') {
    // Client start signal handled below via promise.
    return
  }

  const init = ev.data as InitMsg
  const { pairId, txSab, rxSab, role } = init

  const stream = new SabRingStream(txSab, rxSab)
  self.postMessage({ type: 'pair-ready', pairId, role })

  if (role === 'server') {
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

    // Create StarPC client using the pair stream.
    const client = new Client(async () => stream)
    const echoer = new EchoerClient(client)

    // Make an echo RPC call.
    const response = await echoer.Echo({ body: 'hello via SAB pair' })
    self.postMessage({
      type: 'rpc-result',
      body: response.body,
    })

    stream.close()
  }
}
