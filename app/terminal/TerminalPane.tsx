import { FitAddon } from '@xterm/addon-fit'
import { Terminal as XTerm } from '@xterm/xterm'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import type { MessageStream } from 'starpc'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

import { resolveTerminalTheme } from './terminalTheme.js'

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
  onRetry?: () => void
  onBackToSettings?: () => void
}

type TerminalPaneStatus =
  | { kind: 'connecting' }
  | { kind: 'ready' }
  | { kind: 'failed'; detail: string }
  | { kind: 'closed' }

interface TerminalFrameQueue {
  push(frame: TerminalFrame): void
  started(): boolean
  close(): Promise<void>
  stream(): MessageStream<TerminalFrame>
}

interface TerminalFrameStateHandlers {
  onOutput: () => void
  onFailure: (detail: string) => void
  onClosed: () => void
}

const connectingStatus: TerminalPaneStatus = { kind: 'connecting' }

export function TerminalPane({
  connectTerminal,
  renderTrustChallenge,
  onRetry,
  onBackToSettings,
}: TerminalPaneProps) {
  const terminalHostRef = useRef<HTMLDivElement | null>(null)
  const terminalQueueRef = useRef<TerminalFrameQueue | null>(null)
  const [trustChallenge, setTrustChallenge] = useState<TerminalFrame | null>(
    null,
  )
  const [status, setStatus] = useState<TerminalPaneStatus>(connectingStatus)

  useEffect(() => {
    const host = terminalHostRef.current
    if (!host) return
    setStatus(connectingStatus)
    setTrustChallenge(null)
    if (!connectTerminal) return

    const rpcAbort = new AbortController()
    const renderAbort = new AbortController()
    const queue = createTerminalFrameQueue()
    terminalQueueRef.current = queue
    const term = new XTerm({
      cursorBlink: true,
      screenReaderMode: true,
      ...resolveTerminalTheme(host),
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    const fitAndReportSize = () => {
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
    term.open(host)
    fitAndReportSize()

    const disposeInput = term.onData((chunk) => {
      if (!chunk) return
      queue.push({
        kind: TerminalFrameKind.INPUT,
        data: terminalEncoder.encode(chunk),
      })
    })

    const resizeObserver =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver(() => fitAndReportSize())
    resizeObserver?.observe(host)

    const terminalDone = readTerminalFrames(
      connectTerminal(queue.stream(), rpcAbort.signal),
      term,
      renderAbort.signal,
      setTrustChallenge,
      {
        onOutput: () => setStatus({ kind: 'ready' }),
        onFailure: (detail) => setStatus({ kind: 'failed', detail }),
        onClosed: () => setStatus({ kind: 'closed' }),
      },
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
      resizeObserver?.disconnect()
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
        className={cn(
          'bg-background-dark relative flex min-h-0 min-w-0 flex-1 overflow-hidden',
          'select-none',
        )}
        data-terminal-state={status.kind}
      >
        <div
          ref={terminalHostRef}
          className="min-h-0 w-full min-w-0 flex-1 overflow-hidden"
        />
        {status.kind !== 'ready' ? (
          <TerminalPaneStatusLayer
            status={status}
            cliSession={!renderTrustChallenge}
            onRetry={onRetry}
            onBackToSettings={onBackToSettings}
          />
        ) : null}
      </div>
    </>
  )
}

function TerminalPaneStatusLayer({
  status,
  cliSession,
  onRetry,
  onBackToSettings,
}: {
  status: TerminalPaneStatus
  cliSession: boolean
  onRetry?: () => void
  onBackToSettings?: () => void
}) {
  const failed = status.kind === 'failed'
  const closed = status.kind === 'closed'
  const sessionName = cliSession ? 'CLI session' : 'Terminal session'
  const title = failed
    ? `${sessionName} failed`
    : closed
      ? `${sessionName} ended`
      : cliSession
        ? 'Connecting to Spacewave CLI…'
        : 'Connecting terminal…'
  const detail = failed
    ? status.detail
    : closed
      ? 'The command prompt has ended.'
      : cliSession
        ? 'Preparing a session-local command prompt.'
        : 'Connecting and waiting for the remote prompt.'

  return (
    <div
      className="bg-background-dark/95 absolute inset-0 flex items-center justify-center p-4"
      data-terminal-status-layer={status.kind}
      role={failed ? 'alert' : 'status'}
      aria-live="polite"
    >
      <div
        className={cn(
          'border-foreground/8 bg-background-card/50 w-full max-w-sm rounded-lg border p-3.5',
          failed && 'border-destructive/15 bg-destructive/5',
        )}
      >
        <div className="flex items-start gap-3">
          <div
            aria-hidden="true"
            className={cn(
              'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md',
              failed
                ? 'bg-destructive/10 text-destructive'
                : closed
                  ? 'bg-foreground/5 text-foreground-alt'
                  : 'bg-brand/10 text-brand',
            )}
          >
            {failed ? (
              '!'
            ) : closed ? (
              '×'
            ) : (
              <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" />
            )}
          </div>
          <div className="min-w-0">
            <h2 className="text-foreground text-sm font-semibold tracking-tight">
              {title}
            </h2>
            <p className="text-foreground-alt/70 mt-0.5 text-xs leading-relaxed">
              {detail}
            </p>
            {onRetry || (failed && onBackToSettings) ? (
              <div className="mt-2.5 flex flex-wrap gap-2">
                {onRetry && (
                  <button
                    type="button"
                    className="border-foreground/10 bg-foreground/5 text-foreground hover:bg-foreground/10 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors"
                    onClick={onRetry}
                  >
                    {closed ? 'Restart' : 'Retry'}
                  </button>
                )}
                {failed && onBackToSettings ? (
                  <button
                    type="button"
                    className="text-foreground-alt/70 hover:text-foreground rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
                    onClick={onBackToSettings}
                  >
                    Back to Settings
                  </button>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  )
}

async function readTerminalFrames(
  frames: MessageStream<TerminalFrame>,
  term: XTerm,
  signal: AbortSignal,
  onSshTrustChallenge: (frame: TerminalFrame | null) => void,
  handlers: TerminalFrameStateHandlers,
) {
  let receivedOutput = false
  const markOutput = () => {
    if (receivedOutput || signal.aborted) return
    receivedOutput = true
    term.focus?.()
    handlers.onOutput()
  }
  try {
    for await (const frame of frames) {
      switch (frame.kind) {
        case TerminalFrameKind.OUTPUT: {
          if (!signal.aborted && frame.data) {
            const output = terminalDecoder.decode(frame.data)
            term.write(output)
            if (frame.data.length > 0) markOutput()
          }
          break
        }
        case TerminalFrameKind.READY:
          onSshTrustChallenge(null)
          break
        case TerminalFrameKind.SSH_HOST_KEY_TRUST_CHALLENGE:
          onSshTrustChallenge(frame)
          break
        case TerminalFrameKind.ERROR:
          onSshTrustChallenge(null)
          if (!signal.aborted) {
            handlers.onFailure(safeTerminalFailureDetail(frame.error))
          }
          return
        case TerminalFrameKind.EXIT:
          onSshTrustChallenge(null)
          if (!signal.aborted) {
            handlers.onClosed()
          }
          return
      }
    }
    if (!signal.aborted && !receivedOutput) {
      handlers.onFailure(safeTerminalFailureDetail())
    }
  } catch {
    if (!signal.aborted) {
      handlers.onFailure(safeTerminalFailureDetail())
    }
  }
}

function safeTerminalFailureDetail(rawError?: string): string {
  const normalized = rawError?.toLowerCase() ?? ''
  if (normalized.includes('native runtime')) {
    return 'SSH needs a native connector. Open this terminal in the desktop/native runtime or use a managed Device.'
  }
  if (
    normalized.includes('runtime context') ||
    normalized.includes('runtime-unavailable')
  ) {
    return 'The Spacewave runtime is unavailable in this session. Try again or return to Settings.'
  }
  return 'The terminal session could not start. Try again from the owning surface.'
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
