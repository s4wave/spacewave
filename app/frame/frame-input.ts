import type { Input } from '@s4wave/core/frametrack/frametrack.pb.js'

/** maxPendingInputs bounds keyboard and button transitions waiting to be sent. */
const maxPendingInputs = 32

/** PendingInput records which pointer moves can be replaced without losing a button transition. */
interface PendingInput {
  input: Input
  motionButtons?: number
}

/** FrameInput is the input stream for one connection attempt. */
export class FrameInput implements AsyncIterable<Input> {
  private readonly pending: PendingInput[] = []
  private wake: (() => void) | undefined
  private ended: { detail?: Error } | undefined

  /** push preserves key and button order and coalesces successive motion. */
  public push(input: Input, motionButtons?: number): void {
    // Only motion with the same button state can replace an earlier position.
    if (this.ended) return
    const last = this.pending.at(-1)
    if (
      last &&
      motionButtons !== undefined &&
      last.motionButtons === motionButtons
    ) {
      last.input = input
      return
    }

    // End a blocked controller before dropping a key or button transition.
    if (this.pending.length === maxPendingInputs) {
      this.close(new Error('The Device stopped accepting input.'))
      return
    }
    this.pending.push({ input, motionButtons })
    this.wake?.()
  }

  /** close releases pending reads and drops input for the old connection. */
  public close(detail?: Error): void {
    this.ended ??= { detail }
    this.pending.length = 0
    this.wake?.()
  }

  /** The iterator waits for input or for the connection to close. */
  public async *[Symbol.asyncIterator](): AsyncIterator<Input> {
    while (!this.ended) {
      const next = this.pending.shift()
      if (next) {
        yield next.input
        continue
      }
      await new Promise<void>((resolve) => {
        this.wake = resolve
      })
      this.wake = undefined
    }
    if (this.ended.detail) throw this.ended.detail
  }
}
