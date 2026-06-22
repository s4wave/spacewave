import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { useCallback, useEffect, useRef, useState } from 'react'
import { LuTerminal } from 'react-icons/lu'
import type { MessageStream } from 'starpc'

import {
  TerminalFrameKind,
  TerminalSessionState,
  TerminalTargetKind,
  type Terminal,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import {
  TerminalHandle,
  TerminalTypeID,
} from '@s4wave/sdk/terminal/terminal.js'
import { TerminalSshTrustPanel } from './TerminalSshTrustPanel.js'

export { TerminalTypeID }

const terminalEncoder = new TextEncoder()
const terminalDecoder = new TextDecoder()

type TerminalFrameQueue = ReturnType<typeof createTerminalFrameQueue>

export function TerminalViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const terminalHostRef = useRef<HTMLDivElement | null>(null)
  const terminalQueueRef = useRef<TerminalFrameQueue | null>(null)
  const [trustChallenge, setTrustChallenge] = useState<TerminalFrame | null>(
    null,
  )

  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    TerminalHandle,
    TerminalTypeID,
  )

  const streamFactory = useCallback(
    (h: TerminalHandle, signal: AbortSignal) => h.watchTerminalState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Terminal | undefined = stateResource.value ?? undefined

  useEffect(() => {
    const terminalHandle = handle.value
    const host = terminalHostRef.current
    if (!terminalHandle || !host) return

    const rpcAbort = new AbortController()
    const renderAbort = new AbortController()
    const queue = createTerminalFrameQueue()
    terminalQueueRef.current = queue
    const term = new XTerm({
      cursorBlink: true,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      fontSize: 13,
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
      terminalHandle.connectTerminal(queue.stream(), rpcAbort.signal),
      term,
      renderAbort.signal,
      setTrustChallenge,
    )

    return () => {
      renderAbort.abort()
      if (terminalQueueRef.current === queue) {
        terminalQueueRef.current = null
      }
      void queue
        .close()
        .then(() => terminalDone)
        .finally(() => rpcAbort.abort())
      window.removeEventListener('resize', handleResize)
      disposeInput.dispose()
      term.dispose()
    }
  }, [handle.value])

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
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <span className="text-foreground flex min-w-0 items-center gap-2 text-sm font-semibold select-none">
          <LuTerminal className="size-4 shrink-0" />
          {state?.name || 'Terminal'}
        </span>
        <span className="text-muted-foreground text-xs">
          {formatTerminalState(state?.state)}
        </span>
      </div>
      <div className="border-foreground/8 flex h-8 shrink-0 items-center gap-4 border-b px-4 text-[11px]">
        <span className="text-muted-foreground min-w-0 truncate">
          {formatTerminalTargetLabel(state)}{' '}
          <span className="text-foreground font-mono">
            {formatTerminalTargetObjectKey(state, objectKey)}
          </span>
        </span>
        {state?.command && (
          <span className="text-muted-foreground min-w-0 truncate">
            command <span className="text-foreground">{state.command}</span>
          </span>
        )}
      </div>
      {stateResource.loading && !state && (
        <div className="p-4">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading terminal',
              detail: 'Reading terminal state.',
            }}
          />
        </div>
      )}
      {state?.error && (
        <div className="border-destructive/30 bg-destructive/10 text-destructive border-b px-4 py-2 text-sm">
          {state.error}
        </div>
      )}
      {trustChallenge && (
        <TerminalSshTrustPanel
          challenge={trustChallenge}
          onRespond={respondToSshTrust}
        />
      )}
      <div
        ref={terminalHostRef}
        className="min-h-0 flex-1 overflow-hidden bg-zinc-950"
      />
    </div>
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

function createTerminalFrameQueue(): {
  push: (frame: TerminalFrame) => void
  close: () => Promise<void>
  stream: () => MessageStream<TerminalFrame>
} {
  const frames: TerminalFrame[] = []
  const waiters: Array<() => void> = []
  const closeWaiters: Array<() => void> = []
  const closed = { value: false }
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
    push(frame) {
      if (closed.value) return
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
    async *stream() {
      for (;;) {
        const frame = frames.shift()
        if (frame) {
          yield frame
          if (frame.kind === TerminalFrameKind.CLOSE) {
            resolveClose()
            return
          }
          continue
        }
        if (closed.value) return
        await new Promise<void>((resolve) => {
          waiters.push(resolve)
        })
      }
    },
  }
}

function formatTerminalTargetLabel(state?: Terminal): string {
  return state?.targetKind === TerminalTargetKind.SSH_HOST
    ? 'ssh host'
    : 'device'
}

function formatTerminalTargetObjectKey(
  state: Terminal | undefined,
  fallback: string,
): string {
  if (state?.targetKind === TerminalTargetKind.SSH_HOST) {
    return state.sshHostObjectKey || fallback
  }
  return state?.deviceObjectKey || fallback
}

function formatTerminalState(state?: TerminalSessionState): string {
  switch (state) {
    case TerminalSessionState.CONNECTING:
      return 'Connecting'
    case TerminalSessionState.ACTIVE:
      return 'Active'
    case TerminalSessionState.FAILED:
      return 'Failed'
    case TerminalSessionState.CLOSED:
      return 'Closed'
    case TerminalSessionState.DISCONNECTED:
      return 'Disconnected'
    default:
      return 'Idle'
  }
}
