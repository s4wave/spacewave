// BLDR_ELECTRON is declared if this is Electron.
declare var BLDR_ELECTRON: {
  // setMessagePort sets the runtime message port.
  setMessagePort(port: MessagePort): void
}

// isElectron indicates this is electron.
export const isElectron = typeof BLDR_ELECTRON !== 'undefined'

// setElectronPort sets the MessagePort for the Main process to write to.
export function setElectronPort(port: MessagePort) {
  if (!BLDR_ELECTRON) {
    throw new Error('setElectronPort: not running in electron')
  }
  BLDR_ELECTRON.setMessagePort(port)
}
