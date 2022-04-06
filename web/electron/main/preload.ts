const { contextBridge, ipcRenderer } = require('electron')

let messageHandlerRegistered = false
let messageHandler: ((data: Uint8Array) => void) | undefined

// setMessageHandler sets the ipc message handler.
function setMessageHandler(cb: (data: Uint8Array) => void) {
  messageHandler = cb
  if (!messageHandlerRegistered) {
    messageHandlerRegistered = true
    ipcRenderer.on(
      'runtime-data',
      (_event: Electron.IpcRendererEvent, data: Uint8Array) => {
        if (messageHandler) {
          messageHandler(data)
        }
      }
    )
  }
}

// txMessage transmits a message to the host runtime.
function txMessage(msg: Uint8Array) {
  ipcRenderer.emit('runtime-data', msg)
}

contextBridge.exposeInMainWorld('BLDR_ELECTRON', {
  txMessage,
  setMessageHandler,
})
