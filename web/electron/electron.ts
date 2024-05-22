import {
  ClientToWebRuntime,
  WebDocumentToWebRuntime,
  WebRuntimeToClient,
} from '../runtime/runtime.js'
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

// isMac indicates this is a MacOS platform.
// WICG Spec: https://wicg.github.io/ua-client-hints
// Only expected to work reliably under Electron (where we test it).
export const isMac = (navigator as any)?.userAgentData?.platform === 'macOS' || false

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

// handleElectronWorkerPort handles the other end of the WebDocument.webRuntimePort.
export function handleElectronWorkerPort(port: MessagePort) {
  port.onmessage = (ev) => {
    if (ev.data === 'close') {
      port.close()
      return
    }

    const msg: WebDocumentToWebRuntime = ev.data
    if (typeof msg !== 'object' || !msg.from) {
      console.log(
        'electron: dropped invalid document to web runtime message',
        msg,
      )
      return
    }

    if (msg.connectWebRuntime && ev.ports.length) {
      openElectronPort(
        msg.connectWebRuntime.init,
        msg.connectWebRuntime.port ?? ev.ports[0],
      )
    }
  }

  port.start()
}
