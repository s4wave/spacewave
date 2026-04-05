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

// WebRuntimeToClient is a message sent to the runtime client.
export interface WebRuntimeToClient {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  // the port is passed in the event.ports field
  openStream?: true
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
  // close indicates the client is closed.
  close?: true
}

// ConnectWebRuntimeAck is the acknowledgment of connectWebRuntime.
export interface ConnectWebRuntimeAck {
  // from is the identifier of the sender.
  from: string
  // webRuntimePort contains the port connected to the remote WebRuntime.
  webRuntimePort: MessagePort
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
  // busSab is the SharedArrayBuffer for the intra-tab SAB bus.
  // Present when the worker is a plugin DedicatedWorker on config B/C.
  busSab?: SharedArrayBuffer
  // busPluginId is the numeric plugin ID assigned for the SAB bus.
  busPluginId?: number
}

// WebDocumentToClient is a message sent to a WebDocument client.
export interface WebDocumentToClient {
  // from is the identifier of the WebDocument
  from: string
  // close indicates the web document is about to close.
  close?: true
}

// ServiceWorkerToWebDocument is a message sent from the ServiceWorker to a WebDocument.
export interface ServiceWorkerToWebDocument {
  // from is the identifier of the ServiceWorker.
  from: string
  // init indicates the service worker wants to initialize the client channel.
  init?: true
}
