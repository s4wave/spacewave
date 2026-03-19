import type {
  FSCursor,
  FSCursorChangeCb,
  FSCursorOps,
} from './fs-cursor.js'
import { ErrReleased } from './errors/errors.js'
import type { FSHandle } from './fs-handle.js'

// FSHandleCursor implements a FSCursor attached to a FSHandle.
export class FSHandleCursor implements FSCursor {
  private released = false
  private readonly handle: FSHandle
  private readonly releaseHandle: boolean
  private readonly relFunc: (() => void) | null

  // constructor constructs a new FSHandleCursor attached to the given FSHandle.
  // If releaseHandle is set, Release will also release the FSHandle.
  // If relFunc is set, the release function will be called when released.
  constructor(
    handle: FSHandle,
    releaseHandle: boolean,
    relFunc: (() => void) | null,
  ) {
    this.handle = handle
    this.releaseHandle = releaseHandle
    this.relFunc = relFunc
  }

  // checkReleased checks if the fscursor is released.
  checkReleased(): boolean {
    if (this.handle.checkReleased()) {
      this.released = true
      return true
    }
    return this.released
  }

  // getCursorOps returns the FSCursorOps by delegating to the handle.
  async getCursorOps(signal?: AbortSignal): Promise<FSCursorOps | null> {
    if (this.released) {
      throw ErrReleased
    }
    if (this.handle.checkReleased()) {
      this.released = true
      throw ErrReleased
    }
    const { ops } = await this.handle.getOps(signal ?? new AbortController().signal)
    return ops
  }

  // release releases the filesystem cursor.
  release(): void {
    if (this.released) {
      return
    }
    this.released = true
    if (this.releaseHandle) {
      this.handle.release()
      if (this.relFunc) {
        this.relFunc()
      }
    }
  }

  // addChangeCb is not applicable.
  addChangeCb(_cb: FSCursorChangeCb): void {}

  // getProxyCursor is not applicable to a FSHandle cursor.
  async getProxyCursor(
    _signal?: AbortSignal,
  ): Promise<FSCursor | null> {
    return null
  }

  // Symbol.dispose enables using-based cleanup.
  [Symbol.dispose](): void {
    this.release()
  }
}
