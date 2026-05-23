import type { WorkerCommsDetectResult } from '../bldr/worker-comms-detect.js'

import { WebRuntimeClientType } from './runtime.pb.js'

// NOTE: openStream is a boolean and not a MessagePort as MessagePort can only
// be passed in the event.ports field via the Electron ContextBridge, it is not
// possible to send the MessagePort as part of event.data, the message will be
// silently dropped when passed to postMessage.

// ClientToWebRuntime is a message sent to the WebRuntime.
export interface ClientToWebRuntime {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  // the port is passed in the event.ports field
  openStream?: true
  // close indicates the client is closing.
  close?: boolean
  // armWebLock tells the WebRuntime to start watching the Web Lock for disconnect detection.
  // The WebDocument sends this after acquiring its lock to avoid a race condition.
  armWebLock?: true
}

// buildWebDocumentLockName builds the liveness lock name for a WebDocument.
export function buildWebDocumentLockName(webDocumentId: string): string {
  return `bldr-doc-${webDocumentId}`
}

// buildWebWorkerLockName builds the liveness lock name for a WebWorker.
export function buildWebWorkerLockName(webWorkerId: string): string {
  return `bldr-worker-${webWorkerId}`
}

// buildWebRuntimeClientLockName builds the liveness lock name for a runtime client.
export function buildWebRuntimeClientLockName(
  clientType: WebRuntimeClientType,
  clientUuid: string,
): string | undefined {
  if (!clientUuid) {
    return undefined
  }
  if (clientType === WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT) {
    return buildWebDocumentLockName(clientUuid)
  }
  if (clientType === WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER) {
    return buildWebWorkerLockName(clientUuid)
  }
  return undefined
}

// WebRuntimeToClient is a message sent to the runtime client.
export interface WebRuntimeToClient {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  // the port is passed in the event.ports field
  openStream?: true
  // connected acks that the runtime registered this client successfully.
  // sent as the first message on the channel from WebRuntimeClientInstance.
  connected?: true
}

// WebDocumentToWebRuntime is a message sent to the WebRuntime from the WebDocument.
export interface WebDocumentToWebRuntime {
  // from is the identifier of the WebDocument.
  from: string
  // initWebRuntime contains a request to init the WebRuntime if necessary.
  // contains the web runtime id
  initWebRuntime?: {
    // webRuntimeId is the web runtime identifier.
    webRuntimeId: string
  }
  // connectWebRuntime contains a request to connect as a client of WebRuntime.
  connectWebRuntime?: {
    init: Uint8Array // WebRuntimeClientInit
    port: MessagePort
  }
}

// ClientToWebDocument is a message sent from ServiceWorker to WebDocument.
export interface ClientToWebDocument {
  // from is the identifier of the service worker.
  from: string
  // connectWebRuntime contains a request to connect as a client of WebRuntime.
  // the WebDocument should write a ConnectWebRuntimeAck message on the message port.
  connectWebRuntime?: {
    init: Uint8Array // WebRuntimeClientInit
    port: MessagePort
  }
  // connectWebRtcBridge requests a bridge MessagePort for WebRTC proxying.
  // The WebDocument creates a MessageChannel, sends one port back via
  // ConnectWebRtcBridgeAck, and creates a WebRTCBridgeEndpoint on the other.
  connectWebRtcBridge?: true
  // openSabPair requests a same-tab SAB pair stream to another worker.
  openSabPair?: OpenSabPairRequest
  // closeSabPair releases same-tab SAB pair metadata after a stream closes.
  closeSabPair?: CloseSabPairRequest
  // close indicates the client is closed.
  close?: true
  // failureReason indicates close was caused by a worker runtime failure.
  failureReason?: string
  // frontendReady indicates frontend handlers and links are registered.
  frontendReady?: true
  // capabilityReady indicates the selected backend startup capability is ready.
  capabilityReady?: true
  // ready indicates the worker finished startup and registered its runtime client.
  ready?: true
  // startupMark reports worker-local startup progress for the document timeline.
  startupMark?: {
    label: string
    startTimeMs?: number
    detail?: Record<string, unknown>
  }
}

export interface OpenSabPairRequest {
  requestId: string
  targetWorkerId: string
}

export interface CloseSabPairRequest {
  pairId: string
}

export interface SabPairClosed {
  pairId: string
  reason?: string
}

export interface SabPairEndpointDescriptor {
  pairId: string
  localWorkerId: string
  remoteWorkerId: string
  txSab: SharedArrayBuffer
  rxSab: SharedArrayBuffer
  mtuBytes: number
}

export interface OpenSabPairAck {
  from: string
  requestId: string
  endpoint?: SabPairEndpointDescriptor
  error?: string
}

// ConnectWebRuntimeAck is the acknowledgment of connectWebRuntime.
export interface ConnectWebRuntimeAck {
  // from is the identifier of the sender.
  from: string
  // webRuntimePort contains the port connected to the remote WebRuntime.
  webRuntimePort?: MessagePort
  // error reports that the WebDocument could not forward the port.
  error?: string
}

// ConnectWebRtcBridgeAck is sent from WebDocument to worker with the bridge port.
export interface ConnectWebRtcBridgeAck {
  // from is the identifier of the WebDocument.
  from: string
  // bridgePort is the MessagePort to use for WebRTC bridge commands.
  bridgePort: MessagePort
}

// WebDocumentToWorker is a message sent from the WebDocument to the ServiceWorker, Worker, or SharedWorker.
export interface WebDocumentToWorker {
  // from is the identifier of the WebDocument
  from: string
  // initData contains an optional message passed with addl. init data.
  initData?: Uint8Array
  // initPort initializes the port to communicate with the WebDocument.
  // Worker sends ClientToWebDocument
  // Document sends WebDocumentToClient
  initPort?: MessagePort
  // workerCommsDetect is the main-thread detection result.
  // Passed so workers use the authoritative config without re-detecting.
  workerCommsDetect?: WorkerCommsDetectResult
  // snapshotNow requests the worker to immediately snapshot WASM memory.
  // Sent from the WebDocument during beforeunload.
  snapshotNow?: true
}

// WebDocumentToClient is a message sent to a WebDocument client.
export interface WebDocumentToClient {
  // from is the identifier of the WebDocument
  from: string
  // close indicates the web document is about to close.
  close?: true
  // bridgePort is the MessagePort to use for WebRTC bridge commands.
  bridgePort?: MessagePort
  // resumeReady indicates whether the WebDocument is past its foreground
  // resume gate.
  resumeReady?: boolean
  // sabPairEndpoint delivers an endpoint opened by another worker.
  sabPairEndpoint?: SabPairEndpointDescriptor
  // sabPairClosed notifies a worker that broker metadata for a pair closed.
  sabPairClosed?: SabPairClosed
  // openSabPairAck returns this worker's endpoint for an openSabPair request.
  openSabPairAck?: OpenSabPairAck
}

// ServiceWorkerToWebDocument is a message sent from the ServiceWorker to a WebDocument.
export interface ServiceWorkerToWebDocument {
  // from is the identifier of the ServiceWorker.
  from: string
  // init indicates the service worker wants to initialize the client channel.
  init?: true
}
