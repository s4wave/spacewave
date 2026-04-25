import { Engine } from '@s4wave/sdk/world/engine.js'
import { Documentation } from './proto/docs.pb.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { MessageStream } from 'starpc'
import type {
  WatchDocsRequest,
  WatchDocsResponse,
} from './sdk/docs.pb.js'
import type { DocsResourceService } from './sdk/docs_srpc.pb.js'

// DocsResource serves DocsResourceService for a single documentation
// world object. Reads the Documentation block from the World via the Engine SDK.
class DocsResource implements DocsResourceService {
  private objectKey: string
  private engineRef: ClientResourceRef | undefined

  constructor(objectKey: string, engineRef: ClientResourceRef | undefined) {
    this.objectKey = objectKey
    this.engineRef = engineRef
  }

  // WatchDocs streams the Documentation block, re-reading on every world change.
  // Uses engine.waitSeqno to block until the world state advances, then re-reads
  // the Documentation block via a fresh read-only transaction.
  async *WatchDocs(
    _request: WatchDocsRequest,
    abortSignal?: AbortSignal,
  ): MessageStream<WatchDocsResponse> {
    if (!this.engineRef) {
      return
    }

    const engine = new Engine(this.engineRef)
    try {
      let lastSeqno = 0n
      for (;;) {
        if (abortSignal?.aborted) return

        const documentation = await this.readDocsBlock(engine, abortSignal)
        if (documentation) {
          yield { documentation }
        }

        // Get the seqno of the state we just read.
        const resp = await engine.getSeqno(abortSignal)
        lastSeqno = resp.seqno ?? 0n

        // Block until the world state advances past the snapshot we read.
        await engine.waitSeqno(lastSeqno + 1n, abortSignal)
      }
    } catch (err) {
      if (abortSignal?.aborted) return
      throw err
    } finally {
      engine.release()
    }
  }

  // readDocsBlock reads the Documentation block from the world via a
  // short-lived read-only transaction.
  private async readDocsBlock(
    engine: Engine,
    abortSignal?: AbortSignal,
  ): Promise<Documentation | null> {
    const tx = await engine.newTransaction(false, abortSignal)
    try {
      const objectState = await tx.getObject(this.objectKey, abortSignal)
      if (!objectState) return null
      try {
        const cursor = await objectState.accessWorldState(
          undefined,
          abortSignal,
        )
        try {
          const blockResp = await cursor.getBlock({}, abortSignal)
          if (!blockResp.found || !blockResp.data) return null
          return Documentation.fromBinary(blockResp.data)
        } finally {
          cursor.release()
        }
      } finally {
        objectState.release()
      }
    } finally {
      tx.release()
    }
  }

  // dispose releases the engine ref if still held.
  dispose(): void {
    this.engineRef?.release()
    this.engineRef = undefined
  }
}

export { DocsResource }
