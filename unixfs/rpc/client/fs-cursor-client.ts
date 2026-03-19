import type { FSCursorChange } from '../../fs-cursor.js'
import { ErrReleased, UnixFSError } from '../../errors/errors.js'
import type { NodeType } from '../../block/fstree.pb.js'
import type {
  FSCursorChange as ProtoFSCursorChange,
  FSCursorClientResponse,
} from '../rpc.pb.js'
import type { FSCursorService } from '../rpc_srpc.pb.js'
import { RemoteFSCursor } from './fs-cursor-remote.js'
import { RemoteFSCursorOps } from './fs-cursor-ops-remote.js'

// FSCursorClient is the cursor client session manager.
// It manages the event watching loop from the FSCursorClient streaming RPC.
// The cursor will be released if the streaming RPC is canceled.
export class FSCursorClient {
  released = false
  readonly client: FSCursorService
  readonly clientHandleId: bigint
  rootCursor!: RemoteFSCursor
  private ctrl: AbortController
  private cursors = new Map<bigint, RemoteFSCursor>()
  private ops = new Map<bigint, RemoteFSCursorOps>()
  private cursorOps = new Map<bigint, bigint>()

  private constructor(
    client: FSCursorService,
    clientHandleId: bigint,
    ctrl: AbortController,
  ) {
    this.client = client
    this.clientHandleId = clientHandleId
    this.ctrl = ctrl
  }

  // build constructs and initializes the FSCursorClient.
  // Does not return until the init message is received from the remote.
  static async build(
    svcClient: FSCursorService,
    signal?: AbortSignal,
  ): Promise<FSCursorClient> {
    const ctrl = new AbortController()
    if (signal) {
      signal.addEventListener('abort', () => ctrl.abort(), { once: true })
    }
    const stream = svcClient.FSCursorClient({}, ctrl.signal)
    const iter = stream[Symbol.asyncIterator]()

    const first = await iter.next()
    if (first.done) {
      ctrl.abort()
      throw new Error('stream closed before init')
    }

    const msg = first.value
    if (msg.body?.case === 'unixfsError') {
      ctrl.abort()
      const err = UnixFSError.fromProto(msg.body.value)
      if (err) {
        throw err
      }
      throw new Error('unknown unixfs error in init')
    }
    if (msg.body?.case !== 'init') {
      ctrl.abort()
      throw new Error('expected init')
    }

    const init = msg.body.value
    const clientId = init.clientHandleId ?? 0n
    const rootId = init.cursorHandleId ?? 0n
    if (!clientId || !rootId) {
      ctrl.abort()
      throw new Error('empty handle IDs in init')
    }

    const fsc = new FSCursorClient(svcClient, clientId, ctrl)
    const rootCursor = new RemoteFSCursor(fsc, rootId)
    fsc.rootCursor = rootCursor
    fsc.cursors.set(rootId, rootCursor)
    fsc.execute(iter)
    return fsc
  }

  // execute is the background loop managing cursor change events.
  private async execute(
    iter: AsyncIterator<FSCursorClientResponse>,
  ): Promise<void> {
    try {
      for (;;) {
        const { done, value } = await iter.next()
        if (done) {
          break
        }
        if (value.body?.case === 'cursorChange') {
          this.handleCursorChange(value.body.value)
        }
      }
    } finally {
      this.release()
    }
  }

  // handleCursorChange handles an incoming cursor change message.
  private handleCursorChange(ch: ProtoFSCursorChange): void {
    const id = ch.cursorHandleId
    if (!id) {
      return
    }

    const cursor = this.cursors.get(id)
    if (!cursor) {
      return
    }

    if (ch.released) {
      cursor.released = true
      this.cursors.delete(id)
      const opsId = this.cursorOps.get(id)
      if (opsId !== undefined) {
        const existingOps = this.ops.get(opsId)
        if (existingOps) {
          existingOps.released = true
          this.ops.delete(opsId)
        }
        this.cursorOps.delete(id)
      }
    }

    if (cursor.cbs.length) {
      const change: FSCursorChange = {
        cursor,
        released: ch.released ?? false,
        offset: ch.offset ?? 0n,
        size: ch.size ?? 0n,
      }
      cursor.cbs = cursor.cbs.filter((cb) => cb(change))
    }
  }

  // ingestCursor creates or retrieves a RemoteFSCursor for a handle ID.
  ingestCursor(id: bigint): RemoteFSCursor {
    let c = this.cursors.get(id)
    if (!c || c.released) {
      c = new RemoteFSCursor(this, id)
      this.cursors.set(id, c)
    }
    return c
  }

  // resolveOps creates or retrieves a RemoteFSCursorOps for a handle ID.
  resolveOps(
    cursorId: bigint,
    opsId: bigint,
    nodeType: NodeType | undefined,
    name: string,
  ): RemoteFSCursorOps {
    const existing = this.ops.get(opsId)
    if (
      existing &&
      !existing.released &&
      existing.name === name &&
      existing.nodeType === nodeType
    ) {
      return existing
    }
    const oldOpsId = this.cursorOps.get(cursorId)
    if (oldOpsId !== undefined) {
      const oldOps = this.ops.get(oldOpsId)
      if (oldOps) {
        oldOps.released = true
        this.ops.delete(oldOpsId)
      }
    }
    const cursor = this.cursors.get(cursorId)
    if (!cursor) {
      throw ErrReleased
    }
    const newOps = new RemoteFSCursorOps(cursor, opsId, nodeType ?? 0, name)
    this.ops.set(opsId, newOps)
    this.cursorOps.set(cursorId, opsId)
    return newOps
  }

  // removeOps removes an ops entry from the map.
  removeOps(opsId: bigint): void {
    this.ops.delete(opsId)
  }

  // release releases the filesystem cursor client.
  release(): void {
    if (this.released) {
      return
    }
    this.released = true
    this.ctrl.abort()
    for (const op of this.ops.values()) {
      op.released = true
    }
    this.ops.clear()
    for (const cursor of this.cursors.values()) {
      cursor.released = true
    }
    this.cursors.clear()
    this.cursorOps.clear()
  }

  [Symbol.dispose](): void {
    this.release()
  }
}
