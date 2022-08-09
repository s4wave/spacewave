// const { contextBridge, ipcRenderer } = require('electron')
import { contextBridge, ipcRenderer } from 'electron'
import { WebRuntimeToClient, ClientToWebRuntime } from '../../runtime/runtime.js'
import type { BldrElectron } from '../../bldr/electron.js'
import { MessagePortBridge, messagePortBridgeToMessagePort } from '../../bldr/message-port-bridge.js'

// openClientPort opens a client port to the WebRuntime.
async function openClientPort(
  // init is a WebRuntimeClientInit encoded.
  init: Uint8Array,
  // port is the client port bridge.
  port: MessagePortBridge<WebRuntimeToClient, ClientToWebRuntime>
): Promise<void> {
  const clientPort = messagePortBridgeToMessagePort(port)
  ipcRenderer.postMessage('BLDR_PORT', init, [clientPort])
}

contextBridge.exposeInMainWorld('BLDR_ELECTRON', <BldrElectron>{
  openClientPort,
})
