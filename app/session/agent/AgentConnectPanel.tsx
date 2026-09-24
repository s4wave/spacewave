import { useEffect, useState } from 'react'
import {
  LuBot,
  LuCircleAlert,
  LuCircleCheck,
  LuCopy,
  LuRefreshCw,
  LuX,
} from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'

import { SessionsSection } from '../dashboard/SessionsSection.js'
import { PairingVerificationStep } from '../setup/PairingVerificationStep.js'

// AgentPairing is the state of the panel's pairing attempt.
export interface AgentPairing {
  // code is the pairing code, empty while it is being created.
  code: string
  // expiresAt is the code expiry in epoch milliseconds.
  expiresAt: number
  // remotePeerId is the agent's peer once its CLI connects.
  remotePeerId: string
  // error describes a failed attempt.
  error: string
}

// CopyState is the state of the latest prompt copy.
export type CopyState = 'idle' | 'copying' | 'copied' | 'failed'

export interface AgentConnectPanelProps {
  session: Session | null | undefined
  socketPath: string
  connectedClients: number
  pairing: AgentPairing | null
  copyState: CopyState
  onCopy: () => void
  onNewCode: () => void
  onClose: () => void
}

// AgentConnectPanel shows how the copied prompt connects an agent: the code
// and its countdown, the agent's approval, and the account's Sessions.
export function AgentConnectPanel({
  session,
  socketPath,
  connectedClients,
  pairing,
  copyState,
  onCopy,
  onNewCode,
  onClose,
}: AgentConnectPanelProps) {
  const { providerId, accountId } = useSessionInfo(session)
  const account = useMountAccount(providerId, accountId)
  const isLocal = providerId === 'local'
  const [sessionsOpen, setSessionsOpen] = useState(false)

  return (
    <div
      className="max-h-128 space-y-3 overflow-y-auto p-3"
      data-testid="agent-connect-panel"
    >
      <div className="flex items-start gap-3">
        <div className="bg-brand/10 text-brand flex size-8 shrink-0 items-center justify-center rounded-md">
          <LuBot className="size-4" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold tracking-tight">
            Connect an agent
          </div>
          <CopyStatus copyState={copyState} />
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          className="text-foreground-alt/60 hover:text-foreground rounded p-1"
        >
          <LuX className="size-3.5" />
        </button>
      </div>

      {socketPath ? (
        <SocketBody
          socketPath={socketPath}
          connectedClients={connectedClients}
          onCopy={onCopy}
        />
      ) : (
        <PairingBody
          session={session}
          pairing={pairing}
          onCopy={onCopy}
          onNewCode={onNewCode}
        />
      )}

      <div className="border-foreground/8 border-t pt-2">
        <SessionsSection
          account={account}
          isLocal={isLocal}
          retainStepUp={!isLocal}
          open={sessionsOpen}
          onOpenChange={setSessionsOpen}
        />
      </div>
    </div>
  )
}

const COPY_STATUS: Record<CopyState, string> = {
  idle: 'Copy a prompt and paste it into your agent.',
  copying: 'Copying the prompt…',
  copied: 'Prompt copied. Paste it into your agent.',
  failed: 'Could not copy. Use Copy prompt below.',
}

function CopyStatus({ copyState }: { copyState: CopyState }) {
  return (
    <div
      className={cn(
        'mt-0.5 text-xs',
        copyState === 'failed' ? 'text-destructive' : 'text-foreground-alt/60',
      )}
    >
      {COPY_STATUS[copyState]}
    </div>
  )
}

function SocketBody({
  socketPath,
  connectedClients,
  onCopy,
}: {
  socketPath: string
  connectedClients: number
  onCopy: () => void
}) {
  return (
    <div className="space-y-3">
      <p className="text-foreground-alt text-xs leading-relaxed">
        Your agent connects through this app's command-line socket. No approval
        is needed.
      </p>
      <div className="bg-foreground/5 text-foreground-alt rounded-md px-2 py-1.5 font-mono text-xs break-all">
        {socketPath}
      </div>
      <div className="text-foreground-alt/60 text-xs">
        {connectedClients === 1
          ? '1 command-line client connected'
          : `${connectedClients} command-line clients connected`}
      </div>
      <PanelButton icon={<LuCopy />} label="Copy prompt" onClick={onCopy} />
    </div>
  )
}

function PairingBody({
  session,
  pairing,
  onCopy,
  onNewCode,
}: {
  session: Session | null | undefined
  pairing: AgentPairing | null
  onCopy: () => void
  onNewCode: () => void
}) {
  const [approved, setApproved] = useState('')

  if (!pairing) {
    return (
      <div className="space-y-3">
        <p className="text-foreground-alt text-xs leading-relaxed">
          The prompt carries a pairing code. Your agent runs the Spacewave CLI
          and asks you to approve it here.
        </p>
        <PanelButton
          icon={<LuCopy />}
          label="Copy prompt"
          onClick={onCopy}
          primary
        />
      </div>
    )
  }

  if (pairing.error) {
    return (
      <div className="space-y-3">
        <div className="text-destructive flex items-start gap-2 text-xs">
          <LuCircleAlert className="mt-0.5 size-3.5 shrink-0" />
          <span className="break-words">{pairing.error}</span>
        </div>
        <PanelButton
          icon={<LuRefreshCw />}
          label="New code"
          onClick={onNewCode}
        />
      </div>
    )
  }

  const remotePeerId = pairing.remotePeerId
  if (remotePeerId && approved === remotePeerId) {
    return (
      <AgentConnected
        session={session}
        remotePeerId={remotePeerId}
        onNewCode={onNewCode}
      />
    )
  }
  if (remotePeerId) {
    return (
      <PairingVerificationStep
        key={remotePeerId}
        session={session}
        onContinue={() => setApproved(remotePeerId)}
        onAbort={onNewCode}
      />
    )
  }

  return (
    <CodeWaiting
      code={pairing.code}
      expiresAt={pairing.expiresAt}
      onCopy={onCopy}
      onNewCode={onNewCode}
    />
  )
}

function CodeWaiting({
  code,
  expiresAt,
  onCopy,
  onNewCode,
}: {
  code: string
  expiresAt: number
  onCopy: () => void
  onNewCode: () => void
}) {
  const secondsLeft = useSecondsUntil(expiresAt)
  const expired = !!code && secondsLeft === 0
  const formatted = code ? `${code.slice(0, 4)} ${code.slice(4)}` : ''
  const countdown = `${Math.floor(secondsLeft / 60)}:${(secondsLeft % 60)
    .toString()
    .padStart(2, '0')}`

  return (
    <div className="space-y-3">
      <div className="flex flex-col items-center gap-1.5">
        <div
          className={cn(
            'border-foreground/15 bg-foreground/5 flex h-12 w-full items-center justify-center rounded-md border font-mono text-xl font-bold tracking-widest',
            expired && 'text-foreground-alt/40 line-through',
          )}
          aria-label="Pairing code"
        >
          {code ? formatted : <Spinner size="sm" />}
        </div>
        <div className="text-foreground-alt/60 text-xs" aria-live="polite">
          {!code
            ? 'Creating a pairing code…'
            : expired
              ? 'This code expired. Copy a new prompt.'
              : `Waiting for your agent · expires in ${countdown}`}
        </div>
      </div>
      <p className="text-foreground-alt text-xs leading-relaxed">
        Your agent reads the instructions, runs the Spacewave CLI, and asks you
        to approve it here.
      </p>
      <div className="flex gap-2">
        <PanelButton
          icon={<LuCopy />}
          label={expired ? 'Copy new prompt' : 'Copy prompt'}
          onClick={onCopy}
          primary
        />
        <PanelButton
          icon={<LuRefreshCw />}
          label="New code"
          onClick={onNewCode}
        />
      </div>
    </div>
  )
}

function AgentConnected({
  session,
  remotePeerId,
  onNewCode,
}: {
  session: Session | null | undefined
  remotePeerId: string
  onNewCode: () => void
}) {
  const completion = useResource(
    async (signal) =>
      session ? session.confirmPairing(remotePeerId, '', signal) : null,
    [session, remotePeerId],
  )
  const error = completion.error?.message

  if (error) {
    return (
      <div className="space-y-3">
        <div className="text-destructive flex items-start gap-2 text-xs">
          <LuCircleAlert className="mt-0.5 size-3.5 shrink-0" />
          <span className="break-words">{error}</span>
        </div>
        <PanelButton
          icon={<LuRefreshCw />}
          label="New code"
          onClick={onNewCode}
        />
      </div>
    )
  }

  const done = !completion.loading && !!completion.value
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-sm">
        {done ? (
          <LuCircleCheck className="text-brand size-4" />
        ) : (
          <Spinner size="sm" />
        )}
        <span>{done ? 'Agent connected' : 'Finishing connection…'}</span>
      </div>
      {done && (
        <p className="text-foreground-alt text-xs leading-relaxed">
          The agent can now use your account from its CLI. Remove it under
          Sessions to end its access.
        </p>
      )}
      <PanelButton
        icon={<LuBot />}
        label="Connect another agent"
        onClick={onNewCode}
      />
    </div>
  )
}

function PanelButton({
  icon,
  label,
  onClick,
  primary,
}: {
  icon: React.ReactNode
  label: string
  onClick: () => void
  primary?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex h-8 flex-1 items-center justify-center gap-1.5 rounded-md border px-2 text-xs [&_svg]:size-3.5',
        primary
          ? 'border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground'
          : 'border-foreground/15 hover:border-foreground/30 text-foreground-alt',
      )}
    >
      {icon}
      {label}
    </button>
  )
}

// useSecondsUntil counts down to a deadline once per second.
function useSecondsUntil(deadline: number): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!deadline) return
    queueMicrotask(() => setNow(Date.now()))
    const interval = window.setInterval(() => {
      const current = Date.now()
      setNow(current)
      if (current >= deadline) {
        window.clearInterval(interval)
      }
    }, 1000)
    return () => window.clearInterval(interval)
  }, [deadline])
  return Math.max(0, Math.ceil((deadline - now) / 1000))
}
