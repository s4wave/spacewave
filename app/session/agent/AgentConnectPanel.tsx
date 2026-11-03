import { useEffect, useState } from 'react'
import {
  LuBot,
  LuCircleAlert,
  LuCircleCheck,
  LuCopy,
  LuRefreshCw,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'

import { SessionsSection } from '../dashboard/SessionsSection.js'
import { PairingVerificationStep } from '../setup/PairingVerificationStep.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule5: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    padding: {
      default: null,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule6: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: { color: themeColors['--color-destructive'] },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule13: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule14: { overflowWrap: 'break-word' },
  rule15: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule16: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
  },
  rule17: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    textDecorationLine: 'line-through',
  },
  rule18: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule19: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

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
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'max-h-128 space-y-3 overflow-y-auto p-3'
      }
      data-testid="agent-connect-panel"
    >
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <LuBot sx={stylexStyles.rule1} aria-hidden="true" />
        </div>
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>Connect an agent</div>
          <CopyStatus copyState={copyState} />
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          sx={stylexStyles2.rule5}
        >
          <LuX sx={stylexStyles.rule2} />
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

      <div sx={stylexStyles2.rule6}>
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
      sx={[
        stylexStyles2.rule7,
        copyState === 'failed' ? stylexStyles2.rule8 : stylexStyles2.rule9,
      ]}
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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <p sx={stylexStyles2.rule10}>
        Your agent connects through this app's command-line socket. No approval
        is needed.
      </p>
      <div sx={stylexStyles2.rule11}>{socketPath}</div>
      <div sx={stylexStyles2.rule12}>
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
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
        }
      >
        <p sx={stylexStyles2.rule10}>
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
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
        }
      >
        <div sx={stylexStyles2.rule13}>
          <LuCircleAlert sx={stylexStyles.rule3} />
          <span sx={stylexStyles2.rule14}>{pairing.error}</span>
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
    <div>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule15]}>
        <div
          sx={[stylexStyles2.rule16, !!expired && stylexStyles2.rule17]}
          aria-label="Pairing code"
        >
          {code ? formatted : <Spinner size="sm" />}
        </div>
        <div sx={stylexStyles2.rule12} aria-live="polite">
          {!code
            ? 'Creating a pairing code…'
            : expired
              ? 'This code expired. Copy a new prompt.'
              : `Waiting for your agent · expires in ${countdown}`}
        </div>
      </div>
      <p sx={[nativeChildSpacing.s3, stylexStyles2.rule10]}>
        Your agent reads the instructions, runs the Spacewave CLI, and asks you
        to approve it here.
      </p>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule18]}>
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
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
        }
      >
        <div sx={stylexStyles2.rule13}>
          <LuCircleAlert sx={stylexStyles.rule3} />
          <span sx={stylexStyles2.rule14}>{error}</span>
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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <div sx={stylexStyles2.rule19}>
        {done ? (
          <LuCircleCheck sx={stylexStyles.rule4} />
        ) : (
          <Spinner size="sm" />
        )}
        <span>{done ? 'Agent connected' : 'Finishing connection…'}</span>
      </div>
      {done && (
        <p sx={stylexStyles2.rule10}>
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
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: [&_svg]:size-3.5 */ cn(
          'flex h-11 flex-1 items-center justify-center gap-1.5 rounded-md border px-2 text-xs sm:fine-pointer:h-8 [&_svg]:size-3.5',
          primary
            ? 'border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground'
            : 'border-foreground/15 hover:border-foreground/30 text-foreground-alt',
        )
      }
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
    let cancelled = false
    queueMicrotask(() => {
      if (!cancelled) setNow(Date.now())
    })
    const interval = window.setInterval(() => {
      const current = Date.now()
      setNow(current)
      if (current >= deadline) {
        window.clearInterval(interval)
      }
    }, 1000)
    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [deadline])
  return Math.max(0, Math.ceil((deadline - now) / 1000))
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
