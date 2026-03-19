import type {
  FSCursor,
  FSCursorChangeCb,
  FSCursorOps,
} from '../../fs-cursor.js'
import {
  ErrHandleIDEmpty,
  ErrReleased,
  UnixFSError,
} from '../../errors/errors.js'
import type { FSCursorClient } from './fs-cursor-client.js'

// RemoteFSCursor implements FSCursor attached to a FSCursorClient.
export class RemoteFSCursor implements FSCursor {
  released = false
  readonly c: FSCursorClient
  readonly cursorHandleId: bigint
  cbs: FSCursorChangeCb[] = []

  constructor(c: FSCursorClient, cursorHandleId: bigint) {
    this.c = c
    this.cursorHandleId = cursorHandleId
  }

  // checkReleased checks if the fs cursor is currently released.
  checkReleased(): boolean {
    return this.released
  }

  // getProxyCursor returns a FSCursor to replace this one, if necessary.
  async getProxyCursor(signal?: AbortSignal): Promise<FSCursor | null> {
    const resp = await this.c.client.GetProxyCursor(
      {
        cursorHandleId: this.cursorHandleId,
        clientHandleId: this.c.clientHandleId,
      },
      signal,
    )

    const err = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (err) {
      if (err.isReleased) {
        this.release()
      }
      throw err
    }

    const id = resp.cursorHandleId
    if (!id) {
      return null
    }

    if (this.c.released) {
      throw ErrReleased
    }

    return this.c.ingestCursor(id)
  }

  // getCursorOps returns the FSCursorOps for this cursor.
  async getCursorOps(signal?: AbortSignal): Promise<FSCursorOps | null> {
    const resp = await this.c.client.GetCursorOps(
      {
        cursorHandleId: this.cursorHandleId,
      },
      signal,
    )

    const err = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (err) {
      if (err.isReleased) {
        this.release()
      }
      throw err
    }

    const opsHandleId = resp.opsHandleId
    if (!opsHandleId) {
      throw ErrHandleIDEmpty
    }

    if (this.c.released) {
      throw ErrReleased
    }

    const nodeType = resp.nodeType
    const name = resp.name ?? ''
    return this.c.resolveOps(this.cursorHandleId, opsHandleId, nodeType, name)
  }

  // addChangeCb adds a change callback to detect when the cursor has changed.
  addChangeCb(cb: FSCursorChangeCb): void {
    if (!this.released && !this.c.released) {
      this.cbs.push(cb)
    } else {
      queueMicrotask(() =>
        cb({ cursor: this, released: true, offset: 0n, size: 0n }),
      )
    }
  }

  // release releases the filesystem cursor.
  release(): void {
    if (this.released) {
      return
    }
    this.released = true
    this.c.client
      .ReleaseFSCursor({
        cursorHandleId: this.cursorHandleId,
        clientHandleId: this.c.clientHandleId,
      })
      .catch(() => {})
  }

  [Symbol.dispose](): void {
    this.release()
  }
}
