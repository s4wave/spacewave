import { OpenStreamFunc, HandleStreamFunc } from 'starpc'

import type {
  WorkerCommsConfig,
  WorkerCommsDetectResult,
} from './worker-comms-detect.js'

// PluginTransportFactory creates transport functions for plugin communication.
// Config A/F: MessagePort/ChannelStream (baseline).
// Config B/C: SabRingStream intra-tab (Phase 2), sqlite/OPFS cross-tab (Phase 3).
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

// createTransportFactory creates a PluginTransportFactory using the detected
// worker communication config. Config A/F use MessagePort directly. Config
// B/C will use SabRingStream once Phase 2 wires DedicatedWorker hosting;
// until then they fall back to MessagePort.
export function createTransportFactory(
  detect: WorkerCommsDetectResult,
  opts: MessagePortTransportOpts,
): PluginTransportFactory {
  // Phase 2 will add SAB transport for config B/C here.
  // For now, all configs use the MessagePort path.
  if (detect.config === 'B' || detect.config === 'C') {
    console.log(
      'worker-comms: config',
      detect.config,
      'detected, SAB transport pending Phase 2, using MessagePort',
    )
  }
  return {
    openStream: opts.openStream,
    handleIncomingStream: opts.handleIncomingStream,
    config: detect.config,
  }
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
