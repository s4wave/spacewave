import { ClientToWebRuntime, WebRuntimeToClient } from '../runtime/runtime.js'
import {
  MessagePortBridge,
  messagePortToMessagePortBridge,
} from '../bldr/message-port-bridge.js'

// BldrElectron is the ContextBridge between the WebRuntime and WebDocument.
//
// Transferring MessagePort over the ContextBridge is not supported:
// https://github.com/electron/electron/issues/27024
//
// Transferring MessagePort from preload -> main is supported.
// Transferring functions over ContextBridge is supported.
// The workaround below emulates MessagePort with a read/write callback.
//
// https://www.electronjs.org/docs/latest/api/context-bridge#api-objects
export interface BldrElectron {
  // openClientPort opens a client port to the WebRuntime.
  openClientPort(
    // init is a WebRuntimeClientInit encoded.
    init: Uint8Array,
    // port is the client port bridge.
    port: MessagePortBridge<WebRuntimeToClient, ClientToWebRuntime>,
  ): Promise<void>
}

// BLDR_ELECTRON is declared if this is Electron.
declare const BLDR_ELECTRON: BldrElectron | undefined

// isElectron indicates this is electron.
export const isElectron = typeof BLDR_ELECTRON !== 'undefined'

// openElectronPort connects a MessagePort to the remote Electron main WebRuntime.
export async function openElectronPort(
  init: Uint8Array,
  port: MessagePort,
): Promise<void> {
  if (!BLDR_ELECTRON) {
    throw new Error('not running in electron')
  }

  return BLDR_ELECTRON.openClientPort(
    init,
    messagePortToMessagePortBridge(port),
  )
}

// handleElectronWorkerPort handles a MessagePort as if it was the SharedWorker.
export function handleElectronWorkerPort(port: MessagePort) {
  port.onmessage = (ev) => {
    // expecting this to be sent from openHostWebDocumentClient.
    const data: Uint8Array = ev.data
    if (typeof data !== 'object' || !ev.ports.length) {
      return
    }
    openElectronPort(data, ev.ports[0])
  }
  port.start()
}
