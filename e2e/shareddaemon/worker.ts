import { WebRuntimeClient } from '../../bldr/web/bldr/web-runtime-client.js'
import { WebRuntimeClientType } from '../../bldr/web/runtime/runtime.pb.js'

import { openSession } from './session.js'

// The renderer transfers the real Electron runtime port before issuing work.
self.onmessage = async (event: MessageEvent) => {
  try {
    const runtime = new WebRuntimeClient(
      'shared-fixture',
      'shared-fixture-worker',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      async () => event.ports[0]!,
      null,
      null,
      true,
    )
    const session = await openSession(runtime.rpcClient)
    self.onmessage = async (request: MessageEvent<string>) => {
      try {
        await session.session.RenameSpace({
          sharedObjectId: session.spaceId,
          displayName: request.data,
        })
        self.postMessage({ renamed: request.data, spaceId: session.spaceId })
      } catch (error) {
        self.postMessage({ error: String(error) })
      }
    }
    self.postMessage({
      ready: true,
      sessionId: session.sessionId,
      spaceId: session.spaceId,
    })
  } catch (error) {
    self.postMessage({ error: String(error) })
  }
}
