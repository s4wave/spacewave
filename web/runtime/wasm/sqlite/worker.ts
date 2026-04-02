// sqlite-worker.ts is a dedicated Worker entry point that loads sqlite.wasm,
// initializes OPFS VFS, and serves SqliteBridge RPC.
//
// Created by the Go lifecycle controller via CreateWebWorker with shared=false.
// Uses WebDocumentTracker to connect back to the WebRuntime. Incoming RPC
// streams from Go (via GetWebWorkerOpenStream) are handled by the starpc Server.

import { Server, createMux, createHandler, HandleStreamCtr } from 'starpc'
import { SqliteBridgeServer } from '@go/github.com/aperturerobotics/hydra/sql/sqlite-wasm/rpc/server.js'
import { SqliteBridgeDefinition } from '@go/github.com/aperturerobotics/hydra/sql/sqlite-wasm/rpc/sqlite-bridge_srpc.pb.js'
import { WebDocumentTracker } from '../../../bldr/web-document-tracker.js'
import { WebRuntimeClientType } from '../../../runtime/runtime.pb.js'
import type { WebDocumentToWorker } from '../../../runtime/runtime.js'
import { loadSqlite } from './loader.js'

declare let self: DedicatedWorkerGlobalScope

let tracker: WebDocumentTracker | undefined
const pendingInitMessages: WebDocumentToWorker[] = []
let connectingToRuntime = false
let shutdownWorker:
  | ((reason: string, err?: unknown) => Promise<void>)
  | undefined

function handleWorkerInitMessage(data: WebDocumentToWorker) {
  if (!tracker) {
    pendingInitMessages.push(data)
    console.log(
      'sqlite-worker: buffered WebDocument init while sqlite loads:',
      data.from,
    )
    return
  }

  console.log('sqlite-worker: received WebDocument init from:', data.from)
  tracker.handleWebDocumentMessage(data)
  if (connectingToRuntime) {
    return
  }
  connectingToRuntime = true
  tracker.waitConn().catch((err) => {
    if (shutdownWorker) {
      void shutdownWorker('unable to connect to WebRuntime', err)
      return
    }
    console.error('sqlite-worker: unable to connect to WebRuntime:', err)
    tracker?.close()
    self.close()
  })
}

self.addEventListener('message', (ev) => {
  const data: WebDocumentToWorker = ev.data
  if (typeof data !== 'object' || !data.from || !data.initPort) {
    return
  }
  handleWorkerInitMessage(data)
})

async function main() {
  console.log('sqlite-worker: starting')

  // Load sqlite.wasm and initialize OPFS VFS.
  const { sqlite3, vfsName } = await loadSqlite()
  console.log('sqlite-worker: sqlite loaded, vfs:', vfsName)

  // Create the RPC server implementing SqliteBridge.
  const bridgeServer = new SqliteBridgeServer(sqlite3, vfsName)
  const mux = createMux()
  mux.register(createHandler(SqliteBridgeDefinition, bridgeServer))
  const server = new Server(mux.lookupMethod)
  let shuttingDown = false

  const shutdown = async (reason: string, err?: unknown) => {
    if (shuttingDown) {
      return
    }
    shuttingDown = true
    if (err) {
      console.error(`sqlite-worker: shutting down: ${reason}`, err)
    } else {
      console.log(`sqlite-worker: shutting down: ${reason}`)
    }
    try {
      await bridgeServer.dispose()
    } catch (disposeErr) {
      console.error('sqlite-worker: failed disposing bridge server:', disposeErr)
    }
    tracker?.close()
    self.close()
  }
  shutdownWorker = shutdown

  // Set up incoming stream handler: routes Go RPC streams to our server.
  const handleIncomingStreamCtr = new HandleStreamCtr()
  handleIncomingStreamCtr.set(server.rpcStreamHandler.bind(server))
  const handleIncomingStream = handleIncomingStreamCtr.handleStreamFunc

  // Use WebDocumentTracker to connect to the WebRuntime.
  tracker = new WebDocumentTracker(
    self.name || 'sqlite-worker',
    WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    async () => {
      await shutdown('no WebDocument available')
    },
    handleIncomingStream,
    async () => {
      await shutdown('hosting WebDocument closed')
    },
  )

  for (const msg of pendingInitMessages.splice(0)) {
    handleWorkerInitMessage(msg)
  }

  console.log('sqlite-worker: ready, waiting for WebDocument connection')
}

main().catch((err) => {
  console.error('sqlite-worker: fatal error:', err)
  self.close()
})
