// ClientToWebRuntime is a message sent to the WebRuntime.
export interface ClientToWebRuntime {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  // the MessagePort used is passed in the event.ports field.
  openStream?: boolean
  // close indicates the client is closing.
  close?: boolean
}

// WebRuntimeToClient is a message sent to the runtime client.
export interface WebRuntimeToClient {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  // the MessagePort used is passed in the event.ports field.
  openStream?: boolean
}

// ServiceWorkerToWebDocument is a message sent from ServiceWorker to WebDocument.
export interface ServiceWorkerToWebDocument {
  // from is the identifier of the service worker.
  from: string
  // connectWebRuntime contains a request to connect as a client of WebRuntime.
  // the WebDocument should write a ConnectWebRuntimeAck message.
  connectWebRuntime?: MessagePort
}

// ConnectWebRuntimeAck is the acknowledgment of connectWebRuntime.
export interface ConnectWebRuntimeAck {
  // from is the identifier of the sender.
  from: string
  // webRuntimePort contains the port connected to the remote WebRuntime.
  webRuntimePort: MessagePort
}

// WebDocumentToServiceWorker is a message sent from the WebDocument to the ServiceWorker.
export interface WebDocumentToServiceWorker {
  // from is the identifier of the WebDocument.
  from: string
  // initPort initializes the port to communicate with the WebDocument.
  // sends ServiceWorkerToWebDocument
  initPort?: MessagePort
}
