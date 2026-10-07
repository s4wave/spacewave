import { createHandler, createMux } from 'starpc'

import {
  GetStateRequest,
  GetStateResponse,
  SetStateRequest,
  SetStateResponse,
} from '../../bldr/resource/state/state.pb.js'
import { StateAtomResourceServiceDefinition } from '../../bldr/resource/state/state_srpc.pb.js'
import { WebDocument } from '../../bldr/web/bldr/web-document.js'
import { openElectronPort } from '../../bldr/web/electron/electron.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../../bldr/web/runtime/runtime.pb.js'

import { openSession } from './session.js'

// The fixture control uses existing protobuf messages over the real WebView RPC.
const definition = {
  typeName: 'test.SharedDesktop',
  methods: {
    Read: {
      ...StateAtomResourceServiceDefinition.methods.GetState,
      name: 'Read',
    },
    Rename: {
      ...StateAtomResourceServiceDefinition.methods.SetState,
      name: 'Rename',
    },
  },
} as const
const mux = createMux()
const documentId =
  new URL(location.href).searchParams.get('webDocumentId') || 'main'
const webDocument = new WebDocument({
  webRuntimeId: 'shared-fixture',
  webDocumentId: documentId,
  disableStoragePersist: true,
})
const view = webDocument.registerWebView({
  getUuid: () => 'shared-fixture-view',
  getParentUuid: () => undefined,
  getPermanent: () => false,
  lookupMethod: mux.lookupMethod,
  async setRenderMode() {},
  async setHtmlLinks() {},
  async resetView() {},
  async remove() {
    window.close()
    return true
  },
})

// Read waits for the Resource handshake and reads the displayed identity.
const ready = openSession(view.rpcClient).then((session) => {
  document.getElementById('identity')!.textContent = [
    session.sessionIndex,
    session.sessionId,
    session.spaceId,
    session.spaceName,
  ].join('\n')
  return session
})
ready.catch((error) => {
  document.getElementById('identity')!.textContent = String(error)
})

// Relay a real MessagePort from Electron preload to a real dedicated Worker.
const worker = new Worker('./worker.mjs', { type: 'module' })
const channel = new MessageChannel()
const workerReady = new Promise<{ sessionId: string; spaceId: string }>(
  (resolve, reject) => {
    worker.onerror = (event) => reject(new Error(event.message))
    worker.onmessage = (event) => {
      if (event.data.error) reject(new Error(event.data.error))
      else if (event.data.ready) resolve(event.data)
    }
  },
)
const init = WebRuntimeClientInit.toBinary(
  WebRuntimeClientInit.create({
    webRuntimeId: 'shared-fixture',
    clientUuid: 'shared-fixture-worker',
    disableWebLocks: true,
    clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
  }),
)
void openElectronPort(init, channel.port1)
  .then(() => {
    worker.postMessage({}, [channel.port2])
  })
  .catch((error) => {
    document.getElementById('identity')!.textContent = String(error)
  })

mux.register(
  createHandler(definition, {
    async Read(_request: GetStateRequest): Promise<GetStateResponse> {
      await ready
      return {
        stateJson: document.getElementById('identity')!.textContent || '',
      }
    },
    async Rename(request: SetStateRequest): Promise<SetStateResponse> {
      // Require the worker to mount the same Session and Space as the window.
      const session = await ready
      const workerIdentity = await workerReady
      if (
        workerIdentity.spaceId !== session.spaceId ||
        workerIdentity.sessionId !== session.sessionId
      ) {
        throw new Error(
          'worker and window mounted different Sessions or Spaces',
        )
      }

      // Send one rename to the dedicated Worker and require its committed reply.
      await new Promise<void>((resolve, reject) => {
        worker.onmessage = (event) => {
          if (event.data.error) reject(new Error(event.data.error))
          else if (
            event.data.renamed === request.stateJson &&
            event.data.spaceId === session.spaceId
          )
            resolve()
          else reject(new Error('worker returned a different rename'))
        }
        worker.postMessage(request.stateJson)
      })
      return {}
    },
  }),
)
