import { useCallback, useMemo, type ReactNode } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuArrowLeft, LuCircle, LuTerminal, LuUsers } from 'react-icons/lu'

import { useRuntimeHandoff } from '@s4wave/app/listener/RuntimeHandoffContext.js'
import { useListenerStatus } from '@s4wave/app/hooks/useListenerStatus.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { cn } from '@s4wave/web/style/utils.js'

import {
  buildSpacewaveCommand,
  type CommandOptions,
} from './command-line-commands.js'

// CommandLineSetupPage renders the session-local /settings/cli page.
// It walks the user through connecting the spacewave CLI to the
// current desktop session.
export function CommandLineSetupPage() {
  const navigate = useNavigate()
  const sessionIdx = useSessionIndex()
  const status = useListenerStatus()

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  const opts: CommandOptions = useMemo(
    () => ({
      sessionIndex: sessionIdx,
      socketPath: status?.socketPath || '',
    }),
    [sessionIdx, status?.socketPath],
  )

  return (
    <div className="bg-background-landing flex flex-1 flex-col overflow-y-auto p-6 md:p-10">
      <div className="mx-auto w-full max-w-2xl">
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-foreground mb-6 flex items-center gap-1.5 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back to dashboard
        </button>

        <div className="mb-6 flex items-start gap-3">
          <div className="bg-brand/10 flex h-9 w-9 shrink-0 items-center justify-center rounded-md">
            <LuTerminal className="text-brand h-4 w-4" />
          </div>
          <div>
            <h1 className="text-foreground text-lg font-bold tracking-wide">
              Command Line
            </h1>
            <p className="text-foreground-alt mt-1 text-sm">
              Session {sessionIdx}
            </p>
          </div>
        </div>

        <div className="space-y-4">
          <ListenerStatusChip />
          <WalkthroughSection opts={opts} />
          <InstallGuidanceSection />
          <MoreCommandsSection opts={opts} />
        </div>
      </div>
    </div>
  )
}

// WalkthroughSection renders the three-step CLI walkthrough bound to
// the active session (status, whoami, space list).
function WalkthroughSection({ opts }: { opts: CommandOptions }) {
  return (
    <section className="border-foreground/6 bg-background-card/30 rounded-lg border p-4 backdrop-blur-sm">
      <h2 className="text-foreground mb-3 text-sm font-semibold tracking-tight">
        Try it out
      </h2>
      <p className="text-foreground-alt mb-4 text-xs">
        Run these three commands in a terminal to confirm the CLI is talking to
        this session.
      </p>
      <ol className="space-y-3">
        <CommandStep
          index={1}
          command={buildSpacewaveCommand('status', opts)}
          explanation="Proves the CLI reached the desktop app session and reports socket, lock, and space count."
        />
        <CommandStep
          index={2}
          command={buildSpacewaveCommand('whoami', opts)}
          explanation="Confirms the session identity the CLI is acting as."
        />
        <CommandStep
          index={3}
          command={buildSpacewaveCommand('space list', opts)}
          explanation="Lists the spaces visible to this session."
        />
      </ol>
    </section>
  )
}

// InstallGuidanceSection renders the install-guidance block shown
// above the walkthrough's "More commands" panel. Links out to
// /download#cli for the packaged binary and to the user-facing install
// and quickstart guide.
function InstallGuidanceSection() {
  const cliDownloadHref = useStaticHref('/download#cli')
  const cliInstallHref = useStaticHref('/docs/users/cli/install')

  return (
    <section className="border-foreground/6 bg-background-card/30 rounded-lg border p-4 backdrop-blur-sm">
      <h2 className="text-foreground mb-2 text-sm font-semibold tracking-tight">
        Install the CLI
      </h2>
      <p className="text-foreground-alt mb-3 text-xs">
        Grab a packaged build for your platform. The CLI connects to this
        session out of the box when the desktop app is running.
      </p>
      <ul className="space-y-1.5 text-xs">
        <li>
          <a
            href={cliDownloadHref}
            className="text-brand hover:text-brand/80 transition-colors"
          >
            Download the spacewave CLI
          </a>
          <span className="text-foreground-alt"> for your platform</span>
        </li>
        <li>
          <a
            href={cliInstallHref}
            className="text-brand hover:text-brand/80 transition-colors"
          >
            Install and quickstart guide
          </a>
        </li>
      </ul>
    </section>
  )
}

// MoreCommandsSection renders a collapsed panel that links to the
// next set of useful CLI commands without pulling them into the
// walkthrough. Covers the web listener, file listing, git helpers, and
// the developer docs index.
function MoreCommandsSection({ opts }: { opts: CommandOptions }) {
  const ns = useStateNamespace(['cli-setup'])
  const [open, setOpen] = useStateAtom<boolean>(ns, 'more-open', false)
  return (
    <CollapsibleSection
      title="More commands"
      open={open}
      onOpenChange={setOpen}
    >
      <div className="space-y-3">
        <ul className="space-y-2">
          <MoreCommandRow
            command={buildSpacewaveCommand('web --bg', opts)}
            explanation="Open a local web listener that stays running in the background."
          />
          <MoreCommandRow
            command={buildSpacewaveCommand('fs ls', opts)}
            explanation="List files in the current space."
          />
          <MoreCommandRow
            command={buildSpacewaveCommand('git', opts)}
            explanation="Drive git helpers against a selected space."
          />
        </ul>
        <a
          href="/docs/developers/cli/installation-and-commands"
          className="text-brand hover:text-brand/80 inline-flex items-center gap-1.5 text-xs transition-colors"
        >
          Developer CLI reference
        </a>
      </div>
    </CollapsibleSection>
  )
}

// MoreCommandRow is a compact command listing row without a copy
// button. The walkthrough step handles copy; this section is for
// orientation only.
function MoreCommandRow({
  command,
  explanation,
}: {
  command: string
  explanation: string
}) {
  return (
    <li className="flex flex-col gap-0.5">
      <code className="text-foreground bg-foreground/5 w-fit max-w-full truncate rounded px-1.5 py-0.5 font-mono text-xs">
        {command}
      </code>
      <p className="text-foreground-alt text-xs">{explanation}</p>
    </li>
  )
}

// CommandStep renders a single walkthrough row with a copy button.
function CommandStep({
  index,
  command,
  explanation,
}: {
  index: number
  command: string
  explanation: string
}) {
  return (
    <li className="flex items-start gap-3">
      <div className="bg-brand/10 text-brand flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold">
        {index}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="border-foreground/10 bg-background/40 flex items-center gap-2 rounded-md border px-2.5 py-1.5">
          <code className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
            {command}
          </code>
          <CopyButton text={command} label="Copy command" />
        </div>
        <p className="text-foreground-alt text-xs">{explanation}</p>
      </div>
    </li>
  )
}

// ListenerStatusChip renders a compact row showing the desktop
// resource listener's live status: socket path, Ready / Not listening
// state, and connected-client count. On non-desktop builds (WASM in the
// browser, where the listener is a no-op), renders a note that the CLI
// runs against the desktop app instead of a misleading "Not listening"
// state. While a remote runtime has taken the socket, shows "Not
// listening" with a note pointing at the reclaim affordance.
function ListenerStatusChip() {
  const status = useListenerStatus()
  const handoff = useRuntimeHandoff()

  if (!isDesktop) {
    return (
      <div className="border-foreground/10 bg-background-card/30 flex items-center gap-3 rounded-md border p-3 backdrop-blur-sm">
        <StatusDot tone="muted" />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-foreground text-xs font-medium">
            CLI runs against the desktop app
          </span>
          <span className="text-foreground-alt text-xs">
            Install the Spacewave desktop app to expose a local CLI socket.
          </span>
        </div>
      </div>
    )
  }

  const { tone, label } = listenerToneLabel(status, handoff.active)
  const listening = !!status?.listening && !handoff.active

  let socketRow: ReactNode
  if (status?.socketPath) {
    socketRow = (
      <code className="text-foreground-alt/80 bg-foreground/5 w-fit max-w-full truncate rounded px-1.5 py-0.5 font-mono text-[0.65rem]">
        {status.socketPath}
      </code>
    )
  } else {
    socketRow = (
      <span className="text-foreground-alt text-[0.7rem]">
        Socket path not yet resolved.
      </span>
    )
  }

  return (
    <div
      className={cn(
        'border-foreground/10 bg-background-card/30 flex flex-wrap items-center gap-3 rounded-md border p-3 backdrop-blur-sm',
      )}
    >
      <StatusDot tone={tone} />
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-xs font-semibold tracking-tight">
            {label}
          </span>
          {listening && (
            <span className="text-foreground-alt inline-flex items-center gap-1 text-[0.65rem]">
              <LuUsers className="h-3 w-3" />
              {status?.connectedClients ?? 0} connected
            </span>
          )}
        </div>
        {socketRow}
        {handoff.active && (
          <span className="text-foreground-alt text-[0.7rem]">
            Runtime is handed off to{' '}
            {handoff.requesterName || 'spacewave serve'}. Reclaim it from the
            banner above to resume listening.
          </span>
        )}
      </div>
    </div>
  )
}

// listenerToneLabel returns the desktop listener chip tone and label
// for a given status/handoff pair. Caller must have already gated on
// isDesktop; this helper is only valid for the desktop path.
function listenerToneLabel(
  status: ReturnType<typeof useListenerStatus>,
  handoffActive: boolean,
): { tone: 'ready' | 'warning' | 'muted'; label: string } {
  if (handoffActive) return { tone: 'warning', label: 'Not listening' }
  if (status == null) return { tone: 'muted', label: 'Checking...' }
  if (status.listening) return { tone: 'ready', label: 'Ready' }
  return { tone: 'muted', label: 'Not listening' }
}

// StatusDot renders a small filled circle whose color matches the
// listener state tone.
function StatusDot({ tone }: { tone: 'ready' | 'warning' | 'muted' }) {
  const cls =
    tone === 'ready' ? 'text-emerald-500'
    : tone === 'warning' ? 'text-amber-500'
    : 'text-foreground-alt/40'
  return (
    <LuCircle
      className={cn('h-2.5 w-2.5 shrink-0 fill-current', cls)}
      aria-hidden="true"
    />
  )
}
