import type { FSCursor, FSCursorChangeCb, FSCursorOps } from './fs-cursor.js'
import { ErrReleased, ErrNotExist } from './errors/errors.js'
import type { FSHandle } from './fs-handle.js'
import { FSHandleCursor } from './fs-handle-cursor.js'

// FSCursorGetter implements a FSCursor with a getter function.
// The value from the getter is returned in getProxyCursor.
// If the getter returns null, throws ErrNotExist instead.
// If the getter function is null, throws ErrNotExist.
// checkReleased never returns true until release is called.
export class FSCursorGetter implements FSCursor {
  private released = false
  private readonly getter: ((signal?: AbortSignal) => Promise<FSCursor>) | null

  constructor(getter: ((signal?: AbortSignal) => Promise<FSCursor>) | null) {
    this.getter = getter
  }

  // checkReleased checks if the fscursor is released.
  checkReleased(): boolean {
    return this.released
  }

  // getCursorOps always returns null (this cursor always proxies).
  async getCursorOps(_signal?: AbortSignal): Promise<FSCursorOps | null> {
    if (this.released) {
      throw ErrReleased
    }
    return null
  }

  // getProxyCursor returns the value from the getter, if set.
  async getProxyCursor(signal?: AbortSignal): Promise<FSCursor | null> {
    if (this.released) {
      throw ErrReleased
    }
    if (!this.getter) {
      throw ErrNotExist
    }
    const value = await this.getter(signal)
    if (!value) {
      throw ErrNotExist
    }
    return value
  }

  // release releases the filesystem cursor.
  release(): void {
    this.released = true
  }

  // addChangeCb is not applicable.
  addChangeCb(_cb: FSCursorChangeCb): void {}

  // Symbol.dispose enables using-based cleanup.
  [Symbol.dispose](): void {
    this.release()
  }
}

// newFSCursorGetterWithHandle returns a new FSCursorGetter backed by a FSHandle.
// Constructs a FSCursor from the FSHandle when the cursor is accessed.
export function newFSCursorGetterWithHandle(handle: FSHandle): FSCursorGetter {
  return new FSCursorGetter(async (_signal?: AbortSignal) => {
    if (handle.checkReleased()) {
      throw new Error('fs cursor getter handle: cursor or inode released')
    }
    // The "false" here indicates to not release the handle.
    return new FSHandleCursor(handle, false, null)
  })
}
