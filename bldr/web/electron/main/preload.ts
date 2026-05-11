import { contextBridge, ipcRenderer } from 'electron'
import {
  WebRuntimeToClient,
  ClientToWebRuntime,
} from '../../runtime/runtime.js'
import type { BldrElectron } from '../../electron/electron.js'
import {
  MessagePortBridge,
  messagePortBridgeToMessagePort,
} from '../../bldr/message-port-bridge.js'

// openClientPort opens a client port to the WebRuntime.
async function openClientPort(
  // init is a WebRuntimeClientInit encoded.
  init: Uint8Array,
  // port is the client port bridge.
  port: MessagePortBridge<WebRuntimeToClient, ClientToWebRuntime>,
): Promise<void> {
  const clientPort = messagePortBridgeToMessagePort(port)
  ipcRenderer.postMessage('BLDR_ELECTRON_CLIENT_OPEN', init, [clientPort])
}

// openDirectory opens a native directory picker.
async function openDirectory(): Promise<string | null> {
  return await ipcRenderer.invoke('BLDR_ELECTRON_OPEN_DIRECTORY')
}

// quitDesktopRuntime requests a clean user-initiated desktop runtime quit.
async function quitDesktopRuntime(): Promise<void> {
  await ipcRenderer.invoke('BLDR_ELECTRON_QUIT_DESKTOP_RUNTIME')
}

const exposeContext: BldrElectron = {
  openClientPort,
  openDirectory,
  quitDesktopRuntime,
}
contextBridge.exposeInMainWorld('BLDR_ELECTRON', exposeContext)
