// TODO: Implement Electron SharedWorker.

/*
import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../web/runtime/runtime.pb'
import { CreateWebDocumentFunc, WebRuntime } from '../../web/bldr/web-runtime'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope
const global: any = self

// TODO: create a new window?
const createDocCb: CreateWebDocumentFunc | null = null
const workerHost = new WebRuntime(`electron:main`, createDocCb)
const runtimePort = workerHost.goRuntimePort

// send the runtime port to the electron main process

self.addEventListener('connect', (ev) => {
  const ports = ev.ports
  if (!ports || !ports.length) {
    return
  }
  const port = ev.ports[0]
  if (!port) {
    return
  }
  // incoming message = open a connection with a Web
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
    const initMsg = WebRuntimeClientInit.decode(msg)
    if (!msgEvent.ports.length) {
      console.error(
        'runtime-electron: dropped invalid init message without port',
        msg
      )
      return
    }
    const connPort = msgEvent.ports[0]
    workerHost.handleClient(initMsg, connPort)
  }
  port.start()
})
*/
