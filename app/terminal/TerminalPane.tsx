import { FitAddon } from '@xterm/addon-fit'
import { Terminal as XTerm } from '@xterm/xterm'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import type { MessageStream } from 'starpc'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

const terminalEncoder = new TextEncoder()
const maxQueuedTerminalFrames = 256

const terminalDecoder = new TextDecoder()

export type TerminalPaneConnector = (
  frames: MessageStream<TerminalFrame>,
  signal: AbortSignal,
) => MessageStream<TerminalFrame>

export type TerminalPaneTrustChallengeRenderer = (
  challenge: TerminalFrame,
  onRespond: (accepted: boolean) => void,
) => ReactNode

export interface TerminalPaneProps {
  connectTerminal?: TerminalPaneConnector
  renderTrustChallenge?: TerminalPaneTrustChallengeRenderer
}

interface TerminalFrameQueue {
  push(frame: TerminalFrame): void
  started(): boolean
  close(): Promise<void>
  stream(): MessageStream<TerminalFrame>
}

export function TerminalPane({
  connectTerminal,
  renderTrustChallenge,
}: TerminalPaneProps) {
  const terminalHostRef = useRef<HTMLDivElement | null>(null)
  const terminalQueueRef = useRef<TerminalFrameQueue | null>(null)
  const [trustChallenge, setTrustChallenge] = useState<TerminalFrame | null>(
    null,
  )

  useEffect(() => {
    const host = terminalHostRef.current
    if (!connectTerminal || !host) return
    setTrustChallenge(null)

    const rpcAbort = new AbortController()
    const renderAbort = new AbortController()
    const queue = createTerminalFrameQueue()
    terminalQueueRef.current = queue
    const term = new XTerm({
      cursorBlink: true,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      fontSize: 13,
      screenReaderMode: true,
      theme: {
        background: '#09090b',
        foreground: '#f4f4f5',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    fit.fit()
    queue.push({
      kind: TerminalFrameKind.RESIZE,
      cols: term.cols,
      rows: term.rows,
    })

    const disposeInput = term.onData((chunk) => {
      if (!chunk) return
      queue.push({
        kind: TerminalFrameKind.INPUT,
        data: terminalEncoder.encode(chunk),
      })
    })

    const handleResize = () => {
      try {
        fit.fit()
      } catch {
        return
      }
      queue.push({
        kind: TerminalFrameKind.RESIZE,
        cols: term.cols,
        rows: term.rows,
      })
    }
    window.addEventListener('resize', handleResize)

    const terminalDone = readTerminalFrames(
      connectTerminal(queue.stream(), rpcAbort.signal),
      term,
      renderAbort.signal,
      setTrustChallenge,
    )

    return () => {
      renderAbort.abort()
      if (terminalQueueRef.current === queue) {
        terminalQueueRef.current = null
      }
      const closeDelivered = queue.close()
      if (!queue.started()) {
        rpcAbort.abort()
      } else {
        void closeDelivered
          .then(() => terminalDone)
          .finally(() => rpcAbort.abort())
      }
      window.removeEventListener('resize', handleResize)
      disposeInput.dispose()
      term.dispose()
    }
  }, [connectTerminal])

  const respondToSshTrust = useCallback((accepted: boolean) => {
    const queue = terminalQueueRef.current
    if (!queue) return
    queue.push({
      kind: TerminalFrameKind.SSH_HOST_KEY_TRUST_RESPONSE,
      sshTrustAccepted: accepted,
    })
    setTrustChallenge(null)
  }, [])

  return (
    <>
      {trustChallenge &&
        renderTrustChallenge?.(trustChallenge, respondToSshTrust)}
      <div
        ref={terminalHostRef}
        className="min-h-0 flex-1 overflow-hidden bg-zinc-950"
      />
    </>
  )
}

async function readTerminalFrames(
  frames: MessageStream<TerminalFrame>,
  term: XTerm,
  signal: AbortSignal,
  onSshTrustChallenge: (frame: TerminalFrame | null) => void,
) {
  try {
    for await (const frame of frames) {
      switch (frame.kind) {
        case TerminalFrameKind.OUTPUT:
          if (!signal.aborted) {
            term.write(terminalDecoder.decode(frame.data))
          }
          break
        case TerminalFrameKind.READY:
          onSshTrustChallenge(null)
          break
        case TerminalFrameKind.SSH_HOST_KEY_TRUST_CHALLENGE:
          onSshTrustChallenge(frame)
          break
        case TerminalFrameKind.ERROR:
          onSshTrustChallenge(null)
          if (!signal.aborted) {
            term.writeln(`\r\n${frame.error || 'terminal error'}`)
          }
          return
        case TerminalFrameKind.EXIT:
          onSshTrustChallenge(null)
          if (!signal.aborted) {
            term.writeln(`\r\nprocess exited ${frame.exitCode ?? 0}`)
          }
          return
      }
    }
  } catch (err) {
    if (!signal.aborted) {
      term.writeln(`\r\n${err instanceof Error ? err.message : String(err)}`)
    }
  }
}

function createTerminalFrameQueue(): TerminalFrameQueue {
  const frames: TerminalFrame[] = []
  const waiters: Array<() => void> = []
  const closeWaiters: Array<() => void> = []
  const closed = { value: false }
  const started = { value: false }
  const wake = () => {
    while (waiters.length !== 0) {
      waiters.shift()?.()
    }
  }
  const closePromise = new Promise<void>((resolve) => {
    closeWaiters.push(resolve)
  })
  const resolveClose = () => {
    closed.value = true
    while (closeWaiters.length !== 0) {
      closeWaiters.shift()?.()
    }
  }
  return {
    started() {
      return started.value
    },
    push(frame) {
      if (closed.value) return
      if (frames.length >= maxQueuedTerminalFrames) {
        frames.shift()
      }
      frames.push(frame)
      wake()
    },
    close() {
      if (!closed.value) {
        closed.value = true
        frames.push({ kind: TerminalFrameKind.CLOSE })
        wake()
      }
      return closePromise
    },
    stream() {
      const closeDelivered = { value: false }
      return {
        [Symbol.asyncIterator](): AsyncIterator<TerminalFrame> {
          started.value = true
          const next = (): Promise<IteratorResult<TerminalFrame>> => {
            if (closeDelivered.value) {
              resolveClose()
              return Promise.resolve({ done: true, value: undefined })
            }
            const frame = frames.shift()
            if (frame) {
              if (frame.kind === TerminalFrameKind.CLOSE) {
                closeDelivered.value = true
              }
              return Promise.resolve({ done: false, value: frame })
            }
            if (closed.value) {
              return Promise.resolve({ done: true, value: undefined })
            }
            return new Promise<IteratorResult<TerminalFrame>>((resolve) => {
              waiters.push(() => {
                resolve(next())
              })
            })
          }
          return { next }
        },
      }
    },
  }
}
