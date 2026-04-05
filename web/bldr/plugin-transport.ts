import { OpenStreamFunc, HandleStreamFunc } from 'starpc'

import type { WorkerCommsConfig } from './worker-comms-detect.js'

// PluginTransportFactory creates transport functions for plugin communication.
// Phase 1: wraps current MessagePort/ChannelStream path.
// Phase 2: will add SAB shared bus for intra-tab.
// Phase 3: will add sqlite/OPFS for cross-tab.
export interface PluginTransportFactory {
  // openStream creates an OpenStreamFunc for streams to the runtime.
  openStream: OpenStreamFunc

  // handleIncomingStream handles inbound streams from the runtime.
  handleIncomingStream: HandleStreamFunc

  // config is the detected worker communication config.
  config: WorkerCommsConfig
}

// MessagePortTransportOpts configures a MessagePort-backed transport factory.
export interface MessagePortTransportOpts {
  // openStream is the OpenStreamFunc from WebRuntimeClient.
  openStream: OpenStreamFunc
  // handleIncomingStream is the HandleStreamFunc for inbound streams.
  handleIncomingStream: HandleStreamFunc
}

// createMessagePortTransportFactory creates a PluginTransportFactory that
// delegates to the existing MessagePort/ChannelStream path. This is the
// baseline transport used by all configs as a fallback and by Config A/F
// as the primary transport.
export function createMessagePortTransportFactory(
  opts: MessagePortTransportOpts,
): PluginTransportFactory {
  return {
    openStream: opts.openStream,
    handleIncomingStream: opts.handleIncomingStream,
    config: 'A',
  }
}
