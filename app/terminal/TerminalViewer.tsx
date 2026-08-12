import { useCallback, useMemo } from 'react'
import { LuTerminal } from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import {
  TerminalSessionState,
  TerminalTargetKind,
  type Terminal,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import {
  TerminalHandle,
  TerminalTypeID,
} from '@s4wave/sdk/terminal/terminal.js'
import {
  TerminalPane,
  safeTerminalFailureDetail,
  type TerminalPaneConnector,
  type TerminalPaneTrustChallengeRenderer,
} from './TerminalPane.js'
import { TerminalSshTrustPanel } from './TerminalSshTrustPanel.js'

export { TerminalTypeID }

export function TerminalViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    TerminalHandle,
    TerminalTypeID,
  )
  const terminalHandle = handle.value

  const streamFactory = useCallback(
    (h: TerminalHandle, signal: AbortSignal) => h.watchTerminalState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Terminal | undefined = stateResource.value ?? undefined

  const connectTerminal = useMemo<TerminalPaneConnector | undefined>(() => {
    if (!terminalHandle) return undefined
    return (frames, signal) => terminalHandle.connectTerminal(frames, signal)
  }, [terminalHandle])

  const renderTrustChallenge = useCallback<TerminalPaneTrustChallengeRenderer>(
    (challenge, onRespond) => {
      return (
        <TerminalSshTrustPanel challenge={challenge} onRespond={onRespond} />
      )
    },
    [],
  )

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
      <div className="border-foreground/8 flex h-8 shrink-0 items-center gap-4 border-b px-4 text-xs">
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
      {state?.error && !connectTerminal && (
        <div className="border-destructive/30 bg-destructive/10 text-destructive border-b px-4 py-2 text-sm">
          {safeTerminalFailureDetail(state.error)}
        </div>
      )}
      <TerminalPane
        connectTerminal={connectTerminal}
        renderTrustChallenge={renderTrustChallenge}
      />
    </div>
  )
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
