import { Engine } from '@s4wave/sdk/world/engine.js'
import { Blog } from './proto/blog.pb.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { MessageStream } from 'starpc'
import type {
  WatchBlogRequest,
  WatchBlogResponse,
} from './sdk/blog.pb.js'
import type { BlogResourceService } from './sdk/blog_srpc.pb.js'

// BlogResource serves BlogResourceService for a single blog
// world object. Reads the Blog block from the World via the Engine SDK.
class BlogResource implements BlogResourceService {
  private objectKey: string
  private engineRef: ClientResourceRef | undefined

  constructor(objectKey: string, engineRef: ClientResourceRef | undefined) {
    this.objectKey = objectKey
    this.engineRef = engineRef
  }

  // WatchBlog streams the Blog block, re-reading on every world change.
  // Uses engine.waitSeqno to block until the world state advances, then re-reads
  // the Blog block via a fresh read-only transaction.
  async *WatchBlog(
    _request: WatchBlogRequest,
    abortSignal?: AbortSignal,
  ): MessageStream<WatchBlogResponse> {
    if (!this.engineRef) {
      return
    }

    const engine = new Engine(this.engineRef)
    try {
      let lastSeqno = 0n
      for (;;) {
        if (abortSignal?.aborted) return

        const blog = await this.readBlogBlock(engine, abortSignal)
        if (blog) {
          yield { blog }
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

  // readBlogBlock reads the Blog block from the world via a
  // short-lived read-only transaction.
  private async readBlogBlock(
    engine: Engine,
    abortSignal?: AbortSignal,
  ): Promise<Blog | null> {
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
          return Blog.fromBinary(blockResp.data)
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

export { BlogResource }
