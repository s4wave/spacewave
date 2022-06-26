// const { contextBridge, ipcRenderer } = require('electron')
import { contextBridge, ipcRenderer } from 'electron'

// buildMessagePort builds the runtime message port.
async function buildMessagePort(webRuntimeUuid: string): Promise<MessagePort> {
  const portChannel = 'BLDR_PORT/' + webRuntimeUuid
  return new Promise<MessagePort>((resolve) => {
    ipcRenderer.once(portChannel, (event) => {
      const port = event.ports[0]
      resolve(port)
    })
    ipcRenderer.postMessage('BLDR_PORT', portChannel)
  })
}

let messagePort: Promise<MessagePort> | undefined

// initMessagePort initializes the message port.
async function initMessagePort(
  webRuntimeUuid: string,
  callback: (data: Uint8Array) => void
): Promise<void> {
  if (messagePort) {
    return
  }

  messagePort = buildMessagePort(webRuntimeUuid)
  const port = await messagePort
  port.onmessage = (event) => {
    if (event.data) {
      callback(event.data)
    }
  }
  port.start()
}

// writeMessage writes a message to the remote message port.
async function writeMessage(data: Uint8Array): Promise<void> {
  if (!messagePort) {
    throw new Error('message port not initialized')
  }

  const port = await messagePort
  port.postMessage(data)
}

contextBridge.exposeInMainWorld('BLDR_ELECTRON', {
  initMessagePort,
  writeMessage,
})
