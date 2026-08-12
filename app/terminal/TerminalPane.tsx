import { FitAddon } from '@xterm/addon-fit'
import { Terminal as XTerm } from '@xterm/xterm'
import {
  useCallback,
  useEffect,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import type { MessageStream } from 'starpc'

import { terminalStatusToLoadingView } from '@s4wave/app/loading/status/terminal.js'
import type { TerminalStatus } from '@s4wave/app/loading/status/terminal.js'
import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

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
type TerminalPaneStatus = TerminalStatus | { kind: 'ready' }

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
  const stopTerminalRef = useRef<(() => Promise<void>) | null>(null)
  const [trustChallenge, setTrustChallenge] = useState<TerminalFrame | null>(
    null,
  )
  const [status, setStatus] = useState<TerminalPaneStatus>(connectingStatus)
  const [connectionAttempt, retryConnection] = useReducer(
    (attempt: number) => attempt + 1,
    0,
  )

  const handleLocalRetry = useCallback(async () => {
    await stopTerminalRef.current?.()
    retryConnection()
  }, [])

  useEffect(() => {
    let cancelled = false
    let stopAttempt = async () => {}
    const previousStopped = stopTerminalRef.current?.() ?? Promise.resolve()

    void previousStopped.then(() => {
      if (cancelled) return
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

      let stopping: Promise<void> | null = null
      stopAttempt = () => {
        if (stopping) return stopping
        stopping = (async () => {
          renderAbort.abort()
          if (terminalQueueRef.current === queue) {
            terminalQueueRef.current = null
          }
          const closeDelivered = queue.close()
          if (queue.started()) {
            await closeDelivered
            await terminalDone
          }
          rpcAbort.abort()
          resizeObserver?.disconnect()
          disposeInput.dispose()
          term.dispose()
        })()
        return stopping
      }
      stopTerminalRef.current = stopAttempt
      if (cancelled) void stopAttempt()
    })

    return () => {
      cancelled = true
      void stopAttempt()
    }
  }, [connectTerminal, connectionAttempt])

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
            onRetry={
              connectTerminal ? (onRetry ?? handleLocalRetry) : undefined
            }
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
  status: TerminalStatus
  cliSession: boolean
  onRetry?: () => void
  onBackToSettings?: () => void
}) {
  return (
    <div
      className="bg-background-dark/95 absolute inset-0 flex items-center justify-center p-4"
      data-terminal-status-layer={status.kind}
      role={status.kind === 'failed' ? 'alert' : 'status'}
      aria-live="polite"
    >
      <LoadingCard
        view={terminalStatusToLoadingView(status, {
          cliSession,
          onRetry,
          onBackToSettings,
        })}
        className="w-full max-w-sm"
      />
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
  } catch (error) {
    if (!signal.aborted) {
      handlers.onFailure(safeTerminalFailureDetail(errorMessage(error)))
    }
  }
}

function errorMessage(error: unknown): string | undefined {
  return error instanceof Error ? error.message : undefined
}

export function safeTerminalFailureDetail(rawError?: string): string {
  const normalized = rawError?.toLowerCase() ?? ''
  if (normalized.includes('native runtime')) {
    return 'SSH needs a native connector. Open this terminal in the desktop app or use a managed Device.'
  }
  if (
    normalized.includes('runtime context') ||
    normalized.includes('runtime-unavailable')
  ) {
    return 'The Spacewave runtime is unavailable in this session. Try again.'
  }
  if (
    normalized.includes('ssh: unable to authenticate') ||
    normalized.includes('ssh: handshake failed: unable to authenticate')
  ) {
    return 'The SSH host rejected the username or credentials. Check the host settings, then retry.'
  }
  if (
    normalized.includes('connection refused') ||
    normalized.includes('actively refused')
  ) {
    return 'The SSH host refused the connection. Check that SSH is running and the host and port are correct.'
  }
  if (
    normalized.includes('no route to host') ||
    normalized.includes('network is unreachable') ||
    normalized.includes('i/o timeout') ||
    normalized.includes('timed out')
  ) {
    return 'The SSH host could not be reached. Check its address and network connection, then retry.'
  }
  if (
    normalized.includes('host key') &&
    (normalized.includes('not pinned') || normalized.includes('not trusted'))
  ) {
    return 'The SSH host key was not trusted. Verify the host identity before trying again.'
  }
  if (
    normalized.includes('parse ssh private key') ||
    (normalized.includes('private key') && normalized.includes('invalid'))
  ) {
    return 'The SSH private key could not be used. Check the key and passphrase in the host settings.'
  }
  return 'The terminal session could not start. Check the host settings, then retry.'
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
