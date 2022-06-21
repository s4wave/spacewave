const { contextBridge, ipcRenderer } = require('electron')

// txMessage transmits a message to the host runtime.
function txMessage(msg: Uint8Array) {
  ipcRenderer.emit('runtime-data', msg)
}

// setMessagePort sets the runtime message port.
function setMessagePort(port: MessagePort): void {
  ipcRenderer.postMessage('BLDR_PORT', {}, [port])
}

contextBridge.exposeInMainWorld('BLDR_ELECTRON', {
  setMessagePort,
})
