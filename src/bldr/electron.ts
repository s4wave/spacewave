// BLDR_ELECTRON is declared if this is Electron.
declare var BLDR_ELECTRON: {
  // txMessage transmits a message to the host runtime.
  txMessage(msg: Uint8Array): void
  // setMessageHandler sets the ipc message handler.
  setMessageHandler(cb: (data: Uint8Array) => void): void
}

// forwardElectronIPC forwards the tx and rx broadcast channels to electron ipc.
export function forwardElectronIPC(tx: BroadcastChannel, rx: BroadcastChannel) {
  tx.onmessage = (ev: MessageEvent<Uint8Array>) => {
    BLDR_ELECTRON.txMessage(ev.data)
  }
  BLDR_ELECTRON.setMessageHandler((data: Uint8Array) => {
    rx.postMessage(data)
  })
}

// isElectron indicates this is electron.
export const isElectron = typeof BLDR_ELECTRON !== 'undefined'
