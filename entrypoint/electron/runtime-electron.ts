import { openElectronPort } from '../../web/bldr/electron.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope
// const global: any = self

self.addEventListener('connect', (ev) => {
  const ports = ev.ports
  if (!ports || !ports.length) {
    return
  }
  const port = ev.ports[0]
  if (!port) {
    return
  }
  port.onmessage = (msgEvent) => {
    const msg = msgEvent.data
    if (msg === 'close') {
      port.close()
      return
    }
    if (typeof msg !== 'object' || !(msg instanceof Uint8Array)) {
      console.log('runtime-electron: dropped invalid init message', msg)
      return
    }
    const connPort = msgEvent.ports[0]
    openElectronPort(msg, connPort).catch((err) => {
      console.error('runtime-electron: error opening port with runtime', err)
    })
  }
  port.start()
})
