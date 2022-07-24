// ClientToWebRuntime is a message sent to the WebRuntime.
export interface ClientToWebRuntime {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  openStream?: MessagePort
  // close indicates the client is closing.
  close?: boolean
}

// WebRuntimeToClient is a message sent to the runtime client.
export interface WebRuntimeToClient {
  // openStream contains a request to open a new stream.
  // receiver should ack the stream immediately.
  openStream?: MessagePort
}

// ServiceWorkerToWebDocument is a message sent from ServiceWorker to WebDocument.
export interface ServiceWorkerToWebDocument {
  // from is the identifier of the service worker.
  from: string
  // connectWebRuntime contains a request to connect the MessagePort as a client of WebRuntime.
  connectWebRuntime?: MessagePort
}

// WebDocumentToServiceWorker is a message sent from the WebDocument to the ServiceWorker.
export interface WebDocumentToServiceWorker {
  // from is the identifier of the WebDocument.
  from: string
  // initPort initializes the port to communicate with the WebDocument.
  // sends ServiceWorkerToWebDocument
  initPort?: MessagePort
}
