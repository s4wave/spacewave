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
  // startupMark carries compact runtime startup accounting for the document timeline.
  startupMark?: {
    label: string
    detail: Record<string, unknown>
  }
  // pluginManifestRoot grants the ServiceWorker cache authority for one root.
  pluginManifestRoot?: {
    pluginId: string
    rootHash: string
  }
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
    // env is passed to the host Go runtime when it starts.
    env?: Record<string, string>
  }
  // connectWebRuntime contains a request to connect as a client of WebRuntime.
  connectWebRuntime?: {
    init: Uint8Array // WebRuntimeClientInit
    port: MessagePort
  }
  // opfsBrokerPort is a MessagePort to the WebDocument for brokering a
  // DedicatedWorker OPFS bridge. Sent only when the runtime runs in a
  // SharedWorker (which cannot call navigator.storage.getDirectory()). The
  // runtime worker speaks ClientToWebDocument / WebDocumentToClient OPFS
  // messages over it. The port is transferred in event.ports.
  opfsBrokerPort?: MessagePort
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
  connectWebRtcBridge?: { requestId: string }
  // openSabPair requests a same-tab SAB pair stream to another worker.
  openSabPair?: OpenSabPairRequest
  // closeSabPair releases same-tab SAB pair metadata after a stream closes.
  closeSabPair?: CloseSabPairRequest
  // openOpfsWorker requests a same-tab DedicatedWorker OPFS bridge.
  // The WebDocument returns OpenOpfsWorkerAck and the bridge MessagePort in event.ports.
  openOpfsWorker?: { requestId: string }
  // dedicatedRuntimeHostLost reports that the WebDocument currently relaying
  // the DedicatedWorker host closed, so attached documents must re-elect.
  dedicatedRuntimeHostLost?: {
    webDocumentId: string
    reason?: string
  }
  // close indicates the client is closed.
  close?: true
  // failureReason indicates close was caused by a worker runtime failure.
  failureReason?: string
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
  // requestId identifies the bridge request this ack resolves.
  requestId: string
  // bridgePort is the MessagePort to use for WebRTC bridge commands.
  bridgePort: MessagePort
}

// OpenOpfsWorkerAck is sent from WebDocument to worker with the OPFS bridge port.
// The MessagePort is transferred in event.ports[0] so Electron/raw MessagePort
// paths do not depend on cloning the port inside event.data.
export interface OpenOpfsWorkerAck {
  // from is the identifier of the WebDocument.
  from: string
  // requestId identifies the OPFS worker request this ack resolves.
  requestId: string
  // error reports that the WebDocument could not open the OPFS worker.
  error?: string
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
  // connectDedicatedRuntimeHost asks the ServiceWorker tracker to connect this
  // non-host document to the elected DedicatedWorker runtime host generation.
  connectDedicatedRuntimeHost?: {
    webRuntimeId: string
    init: Uint8Array // WebRuntimeClientInit
    port: MessagePort
  }
  // workerCommsDetect is the main-thread detection result.
  // Passed so workers use the authoritative config without re-detecting.
  workerCommsDetect?: WorkerCommsDetectResult
  // runtimeWasmEnv contains environment variables for Go WASM processes
  // started from this worker.
  runtimeWasmEnv?: Record<string, string>
  // snapshotNow requests the worker to immediately snapshot WASM memory.
  // Sent from the WebDocument during beforeunload.
  snapshotNow?: true
}

// WebDocumentToClient is a message sent to a WebDocument client.
export interface WebDocumentToClient {
  // from is the identifier of the WebDocument
  from: string
  // requestId identifies direct acks that are not nested in an ack field.
  requestId?: string
  // close indicates the web document is about to close.
  close?: true
  // terminal marks a close as a runtime-directed discard of this worker (a
  // CreateWebWorker replacement or a RemoveWebWorker removal) rather than
  // ordinary page teardown. A discarded worker is orphaned forever, so its
  // runtime client must fail fast; a plain close, which also fires on reload,
  // gets a replacement WebDocument, so the client reroutes and waits. Without
  // this intent the two close signals are indistinguishable, and either choice
  // is wrong for the other case.
  terminal?: true
  // bridgePort is the MessagePort to use for WebRTC bridge commands.
  bridgePort?: MessagePort
  // resumeReady indicates whether the WebDocument is past its foreground
  // resume gate.
  resumeReady?: boolean
  // runtimeConnected indicates whether the WebDocument has a live runtime
  // channel.
  runtimeConnected?: boolean
  // sabPairEndpoint delivers an endpoint opened by another worker.
  sabPairEndpoint?: SabPairEndpointDescriptor
  // sabPairClosed notifies a worker that broker metadata for a pair closed.
  sabPairClosed?: SabPairClosed
  // openSabPairAck returns this worker's endpoint for an openSabPair request.
  openSabPairAck?: OpenSabPairAck
  // openOpfsWorkerAck returns the DedicatedWorker OPFS bridge port in event.ports.
  openOpfsWorkerAck?: OpenOpfsWorkerAck
  // opfsWorkerClosed notifies the requester that its OPFS bridge worker died
  // after startup. The requester re-hosts the bridge so the stale client is
  // closed (rejecting in-flight requests) and a fresh worker is installed.
  opfsWorkerClosed?: true
}

// ServiceWorkerToWebDocument is a message sent from the ServiceWorker to a WebDocument.
export interface ServiceWorkerToWebDocument {
  // from is the identifier of the ServiceWorker.
  from: string
  // init indicates the service worker wants to initialize the client channel.
  init?: true
}
