import { OpenStreamFunc, HandleStreamFunc, type PacketStream } from 'starpc'

import type { SabPairEndpointDescriptor } from '../runtime/runtime.js'
import type {
  WorkerCommsConfig,
  WorkerCommsDetectResult,
} from './worker-comms-detect.js'
import { SabRingStream } from './sab-ring-stream.js'

// PluginTransportFactory creates transport functions for plugin communication.
// Config A/F: MessagePort/ChannelStream (baseline).
// Config B/C: SAB Pair Stream intra-tab, MessagePort for runtime.
export interface PluginTransportFactory {
  // openStream opens a stream to the WebRuntime (MessagePort path).
  openStream: OpenStreamFunc

  // handleIncomingStream handles inbound streams from the runtime.
  handleIncomingStream: HandleStreamFunc

  // config is the detected worker communication config.
  config: WorkerCommsConfig

  // openPairStream opens a brokered same-tab SAB pair stream.
  openPairStream?: (targetWorkerId: string) => Promise<PacketStream>

  // openCrossTabStream opens a stream to a peer tab via the brokered
  // cross-tab MessagePort channel. peerId is the ServiceWorker client ID.
  // Returns null if no channel exists for that peer.
  openCrossTabStream?: (peerId: string) => PacketStream | null
}

// TransportFactoryOpts configures the transport factory.
export interface TransportFactoryOpts {
  // openStream is the OpenStreamFunc from WebRuntimeClient.
  openStream: OpenStreamFunc
  // handleIncomingStream is the HandleStreamFunc for inbound streams.
  handleIncomingStream: HandleStreamFunc
  // openCrossTabStream opens a ChannelStream to a peer tab.
  // Provided by the CrossTabManager when cross-tab channels are available.
  openCrossTabStream?: (peerId: string) => PacketStream | null
  // openPairEndpoint requests a SAB pair endpoint descriptor from WebDocument.
  openPairEndpoint?: (
    targetWorkerId: string,
  ) => Promise<SabPairEndpointDescriptor>
  // closePairEndpoint releases WebDocument broker metadata for a pair.
  closePairEndpoint?: (pairId: string) => void
}

// MessagePortTransportOpts configures a MessagePort-backed transport factory.
export type MessagePortTransportOpts = TransportFactoryOpts

// createTransportFactory creates a PluginTransportFactory using the detected
// worker communication config. Config A/F use MessagePort for everything.
// Config B/C use MessagePort for runtime streams and SAB bus for same-tab
// plugin-to-plugin streams.
export function createTransportFactory(
  detect: WorkerCommsDetectResult,
  opts: TransportFactoryOpts,
): PluginTransportFactory {
  const factory: PluginTransportFactory = {
    openStream: opts.openStream,
    handleIncomingStream: opts.handleIncomingStream,
    config: detect.config,
  }

  if (
    opts.openPairEndpoint &&
    (detect.config === 'B' || detect.config === 'C')
  ) {
    factory.openPairStream = async (
      targetWorkerId: string,
    ): Promise<PacketStream> => {
      const endpoint = await opts.openPairEndpoint!(targetWorkerId)
      const stream = new SabRingStream(endpoint.txSab, endpoint.rxSab)
      const close = stream.close.bind(stream)
      stream.close = (err?: Error) => {
        close(err)
        opts.closePairEndpoint?.(endpoint.pairId)
      }
      return stream
    }
    console.log('worker-comms: SAB pair transport available for intra-tab IPC')
  }

  if (opts.openCrossTabStream) {
    factory.openCrossTabStream = opts.openCrossTabStream
    console.log('worker-comms: cross-tab transport available')
  }

  return factory
}
