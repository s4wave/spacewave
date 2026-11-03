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

import {
  terminalStatusToLoadingView,
  type TerminalSessionKind,
  type TerminalStatus,
} from '@s4wave/app/loading/status/terminal.js'
import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { resolveTerminalTheme } from './terminalTheme.js'

import * as stylex from '@stylexjs/stylex'

import {
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-dark'],
    display: 'flex',
    position: 'relative',
    userSelect: 'none',
    minHeight: 0,
    minWidth: 0,
  },
  rule2: {
    flex: 1,
    overflow: 'hidden',
    minHeight: 0,
    minWidth: 0,
    width: '100%',
  },
  rule3: {
    inset: 0,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-dark']} 95%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    position: 'absolute',
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-sm)', width: '100%' },
})

const terminalEncoder = new TextEncoder()
const maxQueuedTerminalFrames = 256

export type TerminalPaneConnector = (
  frames: MessageStream<TerminalFrame>,
  signal: AbortSignal,
) => MessageStream<TerminalFrame>

export type TerminalPaneTrustChallengeRenderer = (
  challenge: TerminalFrame,
  onRespond: (accepted: boolean) => void,
) => ReactNode

export interface TerminalPaneProps {
  // session names what the terminal attaches to, for its status copy.
  session: TerminalSessionKind
  connectTerminal?: TerminalPaneConnector
  renderTrustChallenge?: TerminalPaneTrustChallengeRenderer
  onRetry?: () => void
  onBackToSettings?: () => void
  // describeFailure turns a stream error into the detail the failure card
  // shows. It defaults to safeTerminalFailureDetail.
  describeFailure?: (rawError?: string) => string
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
  session,
  connectTerminal,
  renderTrustChallenge,
  onRetry,
  onBackToSettings,
  describeFailure = safeTerminalFailureDetail,
}: TerminalPaneProps) {
  // Keep the terminal host, current stream and visible connection state together.
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

  // Stop the current terminal before requesting a fresh connection.
  const handleLocalRetry = useCallback(async () => {
    await stopTerminalRef.current?.()
    retryConnection()
  }, [])

  // Replace a connection only after the preceding terminal has stopped.
  useEffect(() => {
    // Retain cancellation and cleanup for this connection attempt.
    let cancelled = false
    let stopAttempt = async () => {}
    const previousStopped = stopTerminalRef.current?.() ?? Promise.resolve()

    // Start this attempt once the previous terminal's cleanup completes.
    void previousStopped.then(() => {
      // Require the active host and connector before opening another terminal.
      if (cancelled) return
      const host = terminalHostRef.current
      if (!host) return
      setStatus(connectingStatus)
      setTrustChallenge(null)
      if (!connectTerminal) return

      // Allocate cancellation, frame delivery and the terminal renderer for this attempt.
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

      // Resize locally and report the terminal grid through its frame queue.
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

      // Forward terminal input through the bounded frame queue.
      const disposeInput = term.onData((chunk) => {
        if (!chunk) return
        queue.push({
          kind: TerminalFrameKind.INPUT,
          data: terminalEncoder.encode(chunk),
        })
      })

      // Report grid changes when the terminal host is resized.
      const resizeObserver =
        typeof ResizeObserver === 'undefined'
          ? null
          : new ResizeObserver(() => fitAndReportSize())
      resizeObserver?.observe(host)

      // Render the connected frame stream and update its visible connection state.
      const terminalDone = readTerminalFrames(
        connectTerminal(queue.stream(), rpcAbort.signal),
        term,
        renderAbort.signal,
        setTrustChallenge,
        describeFailure,
        {
          onOutput: () => setStatus({ kind: 'ready' }),
          onFailure: (detail) => setStatus({ kind: 'failed', detail }),
          onClosed: () => setStatus({ kind: 'closed' }),
        },
      )

      // Share one close handshake across every caller that stops this terminal.
      let stopping: Promise<void> | null = null
      stopAttempt = () => {
        if (stopping) return stopping
        stopping = (async () => {
          // Finish rendering and deliver close before aborting and disposing the transport.
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
  }, [connectTerminal, connectionAttempt, describeFailure])

  // Send the person's host-key trust response through the active terminal queue.
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
      <div sx={[stylexStyles2.rule1]} data-terminal-state={status.kind}>
        <div ref={terminalHostRef} sx={stylexStyles2.rule2} />
        {status.kind !== 'ready' ? (
          <TerminalPaneStatusLayer
            status={status}
            session={session}
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
  session,
  onRetry,
  onBackToSettings,
}: {
  status: TerminalStatus
  session: TerminalSessionKind
  onRetry?: () => void
  onBackToSettings?: () => void
}) {
  return (
    <div
      sx={stylexStyles2.rule3}
      data-terminal-status-layer={status.kind}
      role={status.kind === 'failed' ? 'alert' : 'status'}
      aria-live="polite"
    >
      <LoadingCard
        view={terminalStatusToLoadingView(status, {
          session,
          onRetry,
          onBackToSettings,
        })}
        sx={stylexStyles.rule1}
      />
    </div>
  )
}

async function readTerminalFrames(
  frames: MessageStream<TerminalFrame>,
  term: XTerm,
  signal: AbortSignal,
  onSshTrustChallenge: (frame: TerminalFrame | null) => void,
  describeFailure: (rawError?: string) => string,
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
          // xterm decodes the bytes itself, so a character split across
          // frames is kept whole.
          if (!signal.aborted && frame.data) {
            term.write(frame.data)
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
            handlers.onFailure(describeFailure(frame.error))
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
      handlers.onFailure(describeFailure())
    }
  } catch (error) {
    if (!signal.aborted) {
      handlers.onFailure(describeFailure(errorMessage(error)))
    }
  }
}

function errorMessage(error: unknown): string | undefined {
  return error instanceof Error ? error.message : undefined
}

export function safeTerminalFailureDetail(rawError?: string): string {
  // Map known terminal failures to their existing recovery guidance.
  const normalized = rawError?.toLowerCase() ?? ''
  if (normalized.includes('cli-session-failed')) {
    return 'The Spacewave CLI connection ended. Retry to start a new CLI session.'
  }
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

  // Distinguish rejected SSH connections from unreachable hosts.
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

  // Keep host identity and key failures on their specific recovery paths.
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
  // Keep queued frames, reader notifications and close acknowledgement together.
  const frames: TerminalFrame[] = []
  const waiters: Array<() => void> = []
  const closeWaiters: Array<() => void> = []
  const closed = { value: false }
  const started = { value: false }

  // Wake waiting readers and acknowledge delivery of the terminal close frame.
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

  // Expose the bounded writer, close handshake and asynchronous frame reader.
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
            // Deliver queued frames in order and acknowledge the terminal close frame.
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
