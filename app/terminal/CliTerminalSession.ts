import type { MessageStream } from 'starpc'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

import type { TerminalPaneConnector } from './TerminalPane.js'

const maxRetainedFrames = 256

interface QueuedFrame {
  frame: TerminalFrame
  delivered: () => void
}

interface FrameAttachment {
  channel: TerminalFrameChannel
  stopInput: AbortController
}

// CliTerminalSession owns one browser CLI stream across terminal pane attachments.
export class CliTerminalSession {
  readonly #connect: TerminalPaneConnector
  readonly #abort = new AbortController()
  readonly #input = new TerminalInputChannel()
  readonly #attachments = new Set<FrameAttachment>()
  readonly #history: TerminalFrame[] = []
  #readyFrame: TerminalFrame | null = null
  #promptFrame: TerminalFrame | null = null
  #started = false
  #disposed = false
  #ended = false

  constructor(connect: TerminalPaneConnector) {
    this.#connect = connect
  }

  // ended reports whether the owned stream reached a terminal state.
  get ended() {
    return this.#ended
  }

  // attach connects one pane without transferring ownership of the CLI stream.
  attach(
    frames: MessageStream<TerminalFrame>,
    signal: AbortSignal,
  ): MessageStream<TerminalFrame> {
    this.#start()
    const attachment: FrameAttachment = {
      channel: new TerminalFrameChannel(),
      stopInput: new AbortController(),
    }
    if (this.#readyFrame) attachment.channel.push(this.#readyFrame)
    if (this.#promptFrame) attachment.channel.push(this.#promptFrame)
    for (const frame of this.#history) {
      if (frame !== this.#readyFrame && frame !== this.#promptFrame) {
        attachment.channel.push(frame)
      }
    }
    if (this.#disposed) {
      attachment.channel.close()
      return attachment.channel.stream()
    }

    this.#attachments.add(attachment)
    const detach = () => this.#detach(attachment)
    signal.addEventListener('abort', detach, { once: true })
    void this.#forwardInput(frames, attachment).finally(() => {
      signal.removeEventListener('abort', detach)
      this.#detach(attachment)
    })
    return attachment.channel.stream()
  }

  // dispose closes the owned CLI stream and every attached pane.
  dispose() {
    if (this.#disposed) return
    this.#disposed = true
    for (const attachment of this.#attachments) {
      attachment.stopInput.abort()
      attachment.channel.close()
    }
    this.#attachments.clear()
    if (!this.#started || this.#ended) {
      this.#abort.abort()
      this.#input.discard()
      return
    }
    const closeDelivered = this.#input.push({ kind: TerminalFrameKind.CLOSE })
    this.#input.close()
    void closeDelivered.finally(() => this.#abort.abort())
  }

  #start() {
    if (this.#started || this.#disposed) return
    this.#started = true
    void this.#readOutput()
  }

  async #readOutput() {
    try {
      for await (const frame of this.#connect(
        this.#input.stream(),
        this.#abort.signal,
      )) {
        this.#retain(frame)
        for (const attachment of this.#attachments) {
          attachment.channel.push(frame)
        }
      }
      if (!this.#disposed && !this.#terminalFrameRetained()) {
        this.#fail()
      }
    } catch {
      if (!this.#disposed) this.#fail()
    } finally {
      if (!this.#disposed) {
        this.#ended = true
        for (const attachment of this.#attachments) {
          attachment.stopInput.abort()
          attachment.channel.close()
        }
        this.#input.discard()
        this.#attachments.clear()
      }
    }
  }

  async #forwardInput(
    frames: MessageStream<TerminalFrame>,
    attachment: FrameAttachment,
  ) {
    const iterator = frames[Symbol.asyncIterator]()
    const stop = attachment.stopInput.signal
    try {
      while (!stop.aborted) {
        const next = iterator.next()
        const { promise: stopped, resolve } =
          Promise.withResolvers<IteratorResult<TerminalFrame>>()
        const handleStop = () => resolve({ done: true, value: undefined })
        stop.addEventListener('abort', handleStop, { once: true })
        const result = await Promise.race([next, stopped]).finally(() => {
          stop.removeEventListener('abort', handleStop)
        })
        if (result.done) return
        if (result.value.kind === TerminalFrameKind.CLOSE) return
        await this.#input.push(result.value)
      }
    } finally {
      await iterator.return?.()
    }
  }

  #detach(attachment: FrameAttachment) {
    if (!this.#attachments.delete(attachment)) return
    attachment.stopInput.abort()
    attachment.channel.close()
  }

  #retain(frame: TerminalFrame) {
    if (this.#history.length === maxRetainedFrames) {
      this.#history.shift()
    }
    if (frame.kind === TerminalFrameKind.READY) {
      this.#readyFrame = frame
    } else if (frame.kind === TerminalFrameKind.OUTPUT && !this.#promptFrame) {
      this.#promptFrame = frame
    }
    this.#history.push(frame)
  }

  #terminalFrameRetained() {
    const kind = this.#history.at(-1)?.kind
    return kind === TerminalFrameKind.ERROR || kind === TerminalFrameKind.EXIT
  }

  #fail() {
    const frame: TerminalFrame = {
      kind: TerminalFrameKind.ERROR,
      error: 'cli-session-failed',
    }
    this.#retain(frame)
    for (const attachment of this.#attachments) {
      attachment.channel.push(frame)
    }
  }
}

class TerminalInputChannel {
  readonly #frames: QueuedFrame[] = []
  readonly #waiters: Array<(result: IteratorResult<TerminalFrame>) => void> = []
  #closed = false

  push(frame: TerminalFrame): Promise<void> {
    if (this.#closed) return Promise.resolve()
    const waiter = this.#waiters.shift()
    if (waiter) {
      waiter({ done: false, value: frame })
      return Promise.resolve()
    }
    const { promise, resolve } = Promise.withResolvers<void>()
    this.#frames.push({ frame, delivered: resolve })
    return promise
  }

  close() {
    this.#closed = true
    if (this.#frames.length !== 0) return
    for (const waiter of this.#waiters.splice(0)) {
      waiter({ done: true, value: undefined })
    }
  }

  discard() {
    this.#closed = true
    for (const queued of this.#frames.splice(0)) {
      queued.delivered()
    }
    for (const waiter of this.#waiters.splice(0)) {
      waiter({ done: true, value: undefined })
    }
  }

  stream(): MessageStream<TerminalFrame> {
    return {
      [Symbol.asyncIterator]: () => ({
        next: () => {
          const queued = this.#frames.shift()
          if (queued) {
            queued.delivered()
            if (this.#closed && this.#frames.length === 0) {
              for (const waiter of this.#waiters.splice(0)) {
                waiter({ done: true, value: undefined })
              }
            }
            return Promise.resolve({ done: false, value: queued.frame })
          }
          if (this.#closed) {
            return Promise.resolve({ done: true, value: undefined })
          }
          return new Promise<IteratorResult<TerminalFrame>>((resolve) => {
            this.#waiters.push(resolve)
          })
        },
      }),
    }
  }
}

class TerminalFrameChannel {
  readonly #frames: TerminalFrame[] = []
  readonly #waiters: Array<(result: IteratorResult<TerminalFrame>) => void> = []
  #closed = false

  push(frame: TerminalFrame) {
    if (this.#closed) return
    const waiter = this.#waiters.shift()
    if (waiter) {
      waiter({ done: false, value: frame })
      return
    }
    this.#frames.push(frame)
  }

  close() {
    this.#closed = true
    for (const waiter of this.#waiters.splice(0)) {
      waiter({ done: true, value: undefined })
    }
  }

  stream(): MessageStream<TerminalFrame> {
    return {
      [Symbol.asyncIterator]: () => ({
        next: () => {
          const frame = this.#frames.shift()
          if (frame) {
            return Promise.resolve({ done: false, value: frame })
          }
          if (this.#closed) {
            return Promise.resolve({ done: true, value: undefined })
          }
          return new Promise<IteratorResult<TerminalFrame>>((resolve) => {
            this.#waiters.push(resolve)
          })
        },
      }),
    }
  }
}
