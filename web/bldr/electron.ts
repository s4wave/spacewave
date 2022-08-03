// BLDR_ELECTRON is declared if this is Electron.
declare const BLDR_ELECTRON: {
  // initMessagePort initializes the message port.
  initMessagePort(
    webRuntimeUuid: string,
    callback: (data: Uint8Array) => void
  ): Promise<void>
  // writeMessage writes a message to the remote message port.
  writeMessage(data: Uint8Array): Promise<void>
}

// isElectron indicates this is electron.
export const isElectron = typeof BLDR_ELECTRON !== 'undefined'

// buildElectronPort builds & returns the MessagePort to use for the Electron main process.
export async function buildElectronPort(
  webRuntimeUuid: string
): Promise<MessagePort> {
  if (!BLDR_ELECTRON) {
    throw new Error('not running in electron')
  }

  // workaround for: https://github.com/electron/electron/issues/33086
  const channel = new MessageChannel()
  const workerChannel = channel.port1
  const remoteChannel = channel.port2

  remoteChannel.onmessage = (event) => {
    BLDR_ELECTRON.writeMessage(event.data)
  }
  remoteChannel.start()

  await BLDR_ELECTRON.initMessagePort(webRuntimeUuid, (data: Uint8Array) => {
    remoteChannel.postMessage(data)
  })

  return workerChannel
}
