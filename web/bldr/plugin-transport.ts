import { OpenStreamFunc, HandleStreamFunc, type PacketStream } from 'starpc'

import type {
  WorkerCommsConfig,
  WorkerCommsDetectResult,
} from './worker-comms-detect.js'
import { SabBusEndpoint, SabBusStream } from './sab-bus.js'

// PluginTransportFactory creates transport functions for plugin communication.
// Config A/F: MessagePort/ChannelStream (baseline).
// Config B/C: SabBusStream intra-tab, MessagePort for runtime.
export interface PluginTransportFactory {
  // openStream opens a stream to the WebRuntime (MessagePort path).
  openStream: OpenStreamFunc

  // handleIncomingStream handles inbound streams from the runtime.
  handleIncomingStream: HandleStreamFunc

  // config is the detected worker communication config.
  config: WorkerCommsConfig

  // openBusStream opens a stream to a same-tab plugin via the SAB bus.
  // Returns null if bus is not available.
  openBusStream?: (targetPluginId: number) => Promise<PacketStream>

  // openCrossTabStream opens a stream to a different-tab plugin.
  // Returns null if cross-tab comms is not available.
  openCrossTabStream?: (targetPluginId: number) => Promise<PacketStream>

  // busEndpoint is the SAB bus endpoint for this plugin (config B/C only).
  busEndpoint?: SabBusEndpoint
}

// TransportFactoryOpts configures the transport factory.
export interface TransportFactoryOpts {
  // openStream is the OpenStreamFunc from WebRuntimeClient.
  openStream: OpenStreamFunc
  // handleIncomingStream is the HandleStreamFunc for inbound streams.
  handleIncomingStream: HandleStreamFunc
  // busEndpoint is the SAB bus endpoint (present on config B/C).
  busEndpoint?: SabBusEndpoint
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

  if (opts.busEndpoint) {
    factory.busEndpoint = opts.busEndpoint
    factory.openBusStream = async (
      targetPluginId: number,
    ): Promise<PacketStream> => {
      return new SabBusStream(opts.busEndpoint!, targetPluginId)
    }
    console.log('worker-comms: SAB bus transport available for intra-tab IPC')
  }

  return factory
}
