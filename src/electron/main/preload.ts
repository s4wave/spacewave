const { contextBridge, ipcRenderer } = require('electron')

  // forwardElectronIPC forwards the tx and rx broadcast channels to electron ipc.
function forwardElectronIPC(tx: BroadcastChannel, rx: BroadcastChannel) {
  ipcRenderer.on('runtime-data', (_event: Electron.IpcRendererEvent, data: Uint8Array) => {
    rx.postMessage(data)
  })
  tx.onmessage = (ev: MessageEvent<Uint8Array>) => {
    ipcRenderer.emit('runtime-data', ev.data)
  }
}

contextBridge.exposeInMainWorld('BLDR_ELECTRON', {
  forwardElectronIPC: forwardElectronIPC,
})
