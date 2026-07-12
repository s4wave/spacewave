import type { IFSHandle, TreeUploadEntry } from './handle.js'

// TreeUploadTaskCallbacks projects one entry's lifecycle to an upload consumer.
export interface TreeUploadTaskCallbacks {
  onStart?: () => void
  onComplete?: () => void
  onError?: (err: unknown) => void
}

interface TreeUploadTask {
  handle: Pick<IFSHandle, 'uploadTree'>
  entry: TreeUploadEntry
  callbacks: TreeUploadTaskCallbacks
  signal?: AbortSignal
}

// TreeUploadPool limits concurrent UnixFS uploads and commits each entry through
// its own UploadTree call, so one completion or failure never gates another.
export class TreeUploadPool {
  private readonly queue: TreeUploadTask[] = []
  private active = 0

  public constructor(private readonly concurrency = 3) {
    if (!Number.isInteger(concurrency) || concurrency < 1) {
      throw new RangeError('upload concurrency must be a positive integer')
    }
  }

  // add queues one file or directory for an independent UnixFS commit.
  public add(
    handle: Pick<IFSHandle, 'uploadTree'>,
    entry: TreeUploadEntry,
    callbacks: TreeUploadTaskCallbacks,
    signal?: AbortSignal,
  ): void {
    this.queue.push({ handle, entry, callbacks, signal })
    this.drain()
  }

  private drain(): void {
    while (this.active < this.concurrency) {
      const task = this.queue.shift()
      if (!task) return
      if (task.signal?.aborted) continue

      this.active++
      task.callbacks.onStart?.()
      void this.upload(task)
    }
  }

  private async upload(task: TreeUploadTask): Promise<void> {
    try {
      await task.handle.uploadTree([task.entry], undefined, task.signal)
      if (!task.signal?.aborted) task.callbacks.onComplete?.()
    } catch (err) {
      if (!task.signal?.aborted) task.callbacks.onError?.(err)
    }

    this.active--
    this.drain()
  }
}
