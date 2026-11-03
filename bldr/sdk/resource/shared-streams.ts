/**
 * SharedStreams multicasts snapshot streams by key. The subscribers of one key
 * share one source stream, which opens with the first subscriber and is
 * aborted after the last subscriber leaves and its optional linger expires.
 * Each subscriber receives the latest snapshot, then each later one it is ready for: a slow subscriber skips to
 * the newest. Share only streams whose every message is a full snapshot.
 */
export class SharedStreams {
  private readonly streams = new Map<string, SharedStream<unknown>>()

  /** linger keeps an idle source available for a later subscriber. */
  constructor(private readonly linger = 0) {}

  /**
   * watch subscribes to the stream at key, opening it with open when no
   * subscriber holds it. The source's error or end reaches every subscriber,
   * and the next subscriber opens a fresh source. Aborting signal ends this
   * subscription with the signal's reason.
   */
  public async *watch<T>(
    key: string,
    open: (signal: AbortSignal) => AsyncIterable<T>,
    signal?: AbortSignal,
  ): AsyncIterable<T> {
    // An already aborted subscriber must not open a source.
    signal?.throwIfAborted()

    // Join the running source or open one for this subscriber.
    let stream = this.streams.get(key) as SharedStream<T> | undefined
    if (!stream) {
      const opened = new SharedStream<T>(
        open,
        () => {
          if (this.streams.get(key) === opened) {
            this.streams.delete(key)
          }
        },
        this.linger,
      )
      this.streams.set(key, opened)
      stream = opened
    }

    // Relay the shared snapshots until this subscriber leaves.
    yield* stream.subscribe(signal)
  }
}

/** SharedStream runs one source stream for its subscribers and keeps its latest snapshot. */
class SharedStream<T> {
  // abort stops the source once the last subscriber leaves.
  private readonly abort = new AbortController()
  // latest is the newest snapshot, numbered by seqno.
  private latest: T | undefined
  // seqno counts the snapshots the source has produced.
  private seqno = 0
  // done is set when the source ends.
  private done = false
  // error is the source's failure, if it failed.
  private error: unknown
  // waiters wake the subscribers waiting for a snapshot or the end.
  private waiters = new Set<() => void>()
  // subscribers counts the subscribers iterating the stream.
  private subscribers = 0
  // idle ends a source that stays without subscribers through linger.
  private idle: ReturnType<typeof setTimeout> | undefined

  constructor(
    open: (signal: AbortSignal) => AsyncIterable<T>,
    private readonly onEnd: () => void,
    private readonly linger: number,
  ) {
    void this.run(open)
  }

  /** subscribe yields the latest snapshot and each later one until the source ends or signal aborts. */
  public async *subscribe(signal?: AbortSignal): AsyncIterable<T> {
    // Count this subscriber while it iterates.
    clearTimeout(this.idle)
    this.subscribers++
    try {
      let seen = 0
      for (;;) {
        // Deliver an unseen snapshot before reporting the end.
        signal?.throwIfAborted()
        if (seen !== this.seqno) {
          seen = this.seqno
          yield this.latest as T
          continue
        }
        if (this.done) {
          if (this.error !== undefined) {
            throw this.error
          }
          return
        }
        await this.changed(signal)
      }
    } finally {
      // Stop the source with its last subscriber.
      this.subscribers--
      if (this.subscribers === 0 && !this.done) {
        if (this.linger > 0) {
          this.idle = setTimeout(() => this.close(), this.linger)
        } else {
          this.close()
        }
      }
    }
  }

  /** close aborts an idle source and releases its key. */
  private close() {
    this.abort.abort()
    this.onEnd()
  }

  /** run reads the source until it ends, fails or is aborted, then wakes every subscriber. */
  private async run(open: (signal: AbortSignal) => AsyncIterable<T>) {
    // Publish each snapshot, keeping a failure that was not an abort.
    try {
      for await (const value of open(this.abort.signal)) {
        this.latest = value
        this.seqno++
        this.wake()
      }
    } catch (err) {
      if (!this.abort.signal.aborted) {
        this.error = err ?? new Error('shared stream failed')
      }
    }

    // Release the key for a fresh source and report the end.
    clearTimeout(this.idle)
    this.done = true
    this.onEnd()
    this.wake()
  }

  /** changed resolves at the next snapshot or end, and rejects when signal aborts. */
  private changed(signal?: AbortSignal): Promise<void> {
    return new Promise((resolve, reject) => {
      const onAbort = () => {
        this.waiters.delete(wake)
        reject(signal?.reason)
      }
      const wake = () => {
        signal?.removeEventListener('abort', onAbort)
        resolve()
      }
      this.waiters.add(wake)
      signal?.addEventListener('abort', onAbort, { once: true })
    })
  }

  /** wake wakes every waiting subscriber. */
  private wake() {
    const waiters = this.waiters
    this.waiters = new Set()
    for (const wake of waiters) {
      wake()
    }
  }
}

/** watchKey names one method's snapshot stream and its request, including bigint values. */
export function watchKey(method: string, ...request: unknown[]): string {
  return JSON.stringify([method, ...request], (_key, value: unknown) =>
    typeof value === 'bigint' ? `${value}n` : value,
  )
}
