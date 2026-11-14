import type { ReactNode } from 'react'
import { LuTerminal } from 'react-icons/lu'

import {
  TerminalSessionState,
  TerminalTargetKind,
  type Terminal,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import {
  ViewerStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'

// TerminalViewState is what the Terminal viewer draws, derived from its
// sources as TerminalViewer.tla models them: the handle's status in place of
// the screen, or the screen with the session state once it arrives and at
// most one notice for a failed state watch.
export type TerminalViewState =
  | { body: 'status'; status: ViewerStatusProps }
  | { body: 'screen'; session?: Terminal; notice?: ViewerStatusProps }

export interface TerminalViewProps {
  // view is the derived state to draw.
  view: TerminalViewState
  // detailsOpen opens the Details popover on mount, for a fixture.
  detailsOpen?: boolean
  // children is the terminal screen.
  children: ReactNode
}

// TerminalView shows a terminal session: one bar with its name, state and
// target, its facts behind Details, and the screen filling the pane under it.
// The bar names a target only once the session state says what it is.
export function TerminalView({
  view,
  detailsOpen,
  children,
}: TerminalViewProps) {
  // The session and its target, once the state watch has said them.
  const session = view.body === 'screen' ? view.session : undefined
  const target = session ? terminalTarget(session) : null
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuTerminal />}
        title={session?.name || 'Terminal'}
        state={
          session ? (
            <span className="text-foreground-alt text-xs">
              {formatTerminalState(session.state)}
            </span>
          ) : undefined
        }
        description={
          target ? (
            <>
              on <span>{target.label}</span>{' '}
              <span className="font-mono">{target.key}</span>
            </>
          ) : undefined
        }
        details={
          session
            ? {
                label: 'Session',
                facts: <TerminalFacts state={session} target={target} />,
                defaultOpen: detailsOpen,
              }
            : undefined
        }
      />
      {view.body === 'status' ? (
        <ViewerStatus {...view.status} />
      ) : (
        <>
          {view.notice ? <ViewerStatus {...view.notice} /> : null}
          {children}
        </>
      )}
    </div>
  )
}

// TerminalTarget names the Device or SSH host a session runs on.
interface TerminalTarget {
  label: 'device' | 'ssh host'
  key: string
}

// terminalTarget reads the Device or SSH host a session names, or null
// when it names none.
function terminalTarget(state: Terminal): TerminalTarget | null {
  const ssh = state.targetKind === TerminalTargetKind.SSH_HOST
  const key = ssh ? state.sshHostObjectKey : state.deviceObjectKey
  if (!key) return null
  return { label: ssh ? 'ssh host' : 'device', key }
}

// TerminalFacts lists a session's facts for the Details popover.
function TerminalFacts({
  state,
  target,
}: {
  state: Terminal
  target: TerminalTarget | null
}) {
  return (
    <>
      <FactRow
        label="State"
        value={formatTerminalState(state.state)}
        kind="text"
        control={null}
      />
      {target ? (
        <FactRow
          label={target.label === 'device' ? 'Device' : 'SSH host'}
          value={target.key}
        />
      ) : null}
      {state.devicePeerId ? (
        <FactRow label="Peer ID" value={state.devicePeerId} />
      ) : null}
      {state.command ? (
        <FactRow label="Command" value={state.command} kind="text" />
      ) : null}
      {state.cols && state.rows ? (
        <FactRow
          label="Size"
          value={`${state.cols} × ${state.rows}`}
          kind="text"
          control={null}
        />
      ) : null}
      {state.createdAt ? (
        <FactRow
          label="Started"
          value={state.createdAt.toLocaleString(undefined, {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
          })}
          kind="text"
          control={null}
        />
      ) : null}
    </>
  )
}

// formatTerminalState names a session state as the bar shows it.
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
