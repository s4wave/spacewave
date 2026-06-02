import { useCallback, useMemo, type ReactNode } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuArrowLeft,
  LuDownload,
  LuCircle,
  LuRefreshCw,
  LuSettings,
  LuTerminal,
  LuUsers,
} from 'react-icons/lu'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Root } from '@s4wave/sdk/root'
import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  type DesktopCLIInstallActionItem,
  type DesktopCLIEntrypointIdentity,
  type DesktopCLIInstallState,
  type WatchCLIInstallStateResponse,
} from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime.pb.js'
import { DesktopCLIInstallResourceServiceClient } from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime_srpc.pb.js'

import { useRuntimeHandoff } from '@s4wave/app/listener/RuntimeHandoffContext.js'
import { useListenerStatus } from '@s4wave/app/hooks/useListenerStatus.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
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
  const rootResource = useRootResource()
  const cliInstall = useDesktopCLIInstallState(rootResource)

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  const handleCLIInstallAction = useCallback(
    async (action: DesktopCLIInstallActionItem) => {
      const root = rootResource.value
      if (!root) return
      const service = new DesktopCLIInstallResourceServiceClient(root.client)
      await service.InvokeCLIInstallAction({
        actionId: action.id,
        generation: action.generation,
      })
    },
    [rootResource.value],
  )

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
          <LuArrowLeft className="size-4" />
          Back to dashboard
        </button>

        <div className="mb-6 flex items-start gap-3">
          <div className="bg-brand/10 flex size-9 shrink-0 items-center justify-center rounded-md">
            <LuTerminal className="text-brand size-4" />
          </div>
          <div>
            <h1 className="text-foreground text-lg font-semibold tracking-wide">
              Command Line
            </h1>
            <p className="text-foreground-alt mt-1 text-sm">
              Session {sessionIdx}
            </p>
          </div>
        </div>

        <div className="space-y-4">
          <DesktopCLIInstallCard
            state={cliInstall.value?.state}
            loading={cliInstall.loading}
            error={cliInstall.error}
            onInvokeAction={handleCLIInstallAction}
          />
          <ListenerStatusChip />
          <WalkthroughSection opts={opts} />
          <InstallGuidanceSection />
          <MoreCommandsSection opts={opts} />
        </div>
      </div>
    </div>
  )
}

function useDesktopCLIInstallState(
  rootResource: ReturnType<typeof useRootResource>,
) {
  return useStreamingResource(
    rootResource,
    (root: Root, signal: AbortSignal) =>
      watchDesktopCLIInstallState(root, signal),
    [],
  )
}

async function* watchDesktopCLIInstallState(
  root: Root,
  signal: AbortSignal,
): AsyncIterable<WatchCLIInstallStateResponse> {
  if (!isDesktop) return
  const service = new DesktopCLIInstallResourceServiceClient(root.client)
  for await (const item of service.WatchCLIInstallState({}, signal)) {
    yield item
  }
}

export function DesktopCLIInstallCard({
  state,
  loading,
  error,
  onInvokeAction,
}: {
  state?: DesktopCLIInstallState
  loading?: boolean
  error?: Error | null
  onInvokeAction?: (action: DesktopCLIInstallActionItem) => void | Promise<void>
}) {
  const presentation = desktopCLIInstallPresentation(state, loading, error)
  const selectedTarget = state?.targets?.find((target) => target.selected)
  const actions = state?.actions ?? []

  return (
    <section className="border-foreground/6 bg-background-card/30 rounded-lg border p-4 backdrop-blur-sm">
      <div className="mb-3 flex items-start gap-3">
        <StatusDot tone={presentation.tone} />
        <div className="min-w-0 flex-1">
          <h2 className="text-foreground text-sm font-semibold tracking-tight">
            Desktop CLI install
          </h2>
          <p className="text-foreground-alt mt-0.5 text-xs">
            {presentation.label}
          </p>
        </div>
        <ActionButtons actions={actions} onInvokeAction={onInvokeAction} />
      </div>

      {presentation.detail && (
        <p className="text-foreground-alt mb-3 text-xs">
          {presentation.detail}
        </p>
      )}

      <div className="grid gap-3 md:grid-cols-2">
        <div className="min-w-0">
          <p className="text-foreground-alt mb-1 text-[0.7rem] font-medium uppercase">
            Selected target
          </p>
          <code className="text-foreground-alt/90 bg-foreground/5 block max-w-full truncate rounded px-1.5 py-1 font-mono text-[0.7rem]">
            {selectedTarget?.path || 'Not selected'}
          </code>
          {selectedTarget && (
            <p className="text-foreground-alt mt-1 text-[0.7rem]">
              {selectedTarget.detail ||
                (selectedTarget.writable
                  ? 'Writable user target'
                  : 'Manual target review')}
            </p>
          )}
        </div>

        <div className="min-w-0">
          <p className="text-foreground-alt mb-1 text-[0.7rem] font-medium uppercase">
            Release identity
          </p>
          <p className="text-foreground text-xs">
            {formatCLIIdentity(state?.available) ||
              'Waiting for release metadata'}
          </p>
          {state?.installed?.manifestId && (
            <p className="text-foreground-alt mt-1 text-[0.7rem]">
              Installed {formatCLIIdentity(state.installed)}
            </p>
          )}
        </div>
      </div>

      {state?.conflictPath && (
        <div className="mt-3 rounded-md border border-amber-500/20 bg-amber-500/5 p-3">
          <p className="text-foreground text-xs font-medium">PATH conflict</p>
          <code className="text-foreground-alt/90 mt-1 block max-w-full truncate font-mono text-[0.7rem]">
            {state.conflictPath}
          </code>
        </div>
      )}

      {state?.errorMessage && (
        <p className="text-danger mt-3 text-xs">{state.errorMessage}</p>
      )}
    </section>
  )
}

function ActionButtons({
  actions,
  onInvokeAction,
}: {
  actions: DesktopCLIInstallActionItem[]
  onInvokeAction?: (action: DesktopCLIInstallActionItem) => void | Promise<void>
}) {
  const visibleActions = actions.filter((action) => {
    switch (action.kind) {
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK:
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_OPEN_SETTINGS:
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_INSTALL:
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_UPDATE:
        return true
      default:
        return false
    }
  })
  if (visibleActions.length === 0) return null

  return (
    <div className="flex shrink-0 items-center gap-1">
      {visibleActions.map((action) => {
        const Icon =
          action.kind ===
          DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK
            ? LuRefreshCw
            : action.kind ===
                  DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_INSTALL ||
                action.kind ===
                  DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_UPDATE
              ? LuDownload
              : LuSettings
        return (
          <button
            key={action.id}
            type="button"
            disabled={!(action.enabled ?? false)}
            title={action.label || action.id}
            aria-label={action.label || action.id}
            onClick={() => void onInvokeAction?.(action)}
            className="border-foreground/10 text-foreground-alt hover:text-foreground hover:bg-foreground/5 disabled:text-foreground-alt/30 flex size-7 items-center justify-center rounded-md border transition-colors disabled:cursor-not-allowed"
          >
            <Icon className="size-3.5" />
          </button>
        )
      })}
    </div>
  )
}

function desktopCLIInstallPresentation(
  state: DesktopCLIInstallState | undefined,
  loading: boolean | undefined,
  error: Error | null | undefined,
): {
  tone: 'ready' | 'warning' | 'muted' | 'active' | 'error'
  label: string
  detail: string
} {
  if (error) {
    return {
      tone: 'error',
      label: 'Install state unavailable',
      detail: error.message,
    }
  }
  if (loading || !state) {
    return {
      tone: 'muted',
      label: 'Checking command line tool',
      detail: '',
    }
  }
  switch (state.status) {
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED:
      return {
        tone: 'ready',
        label: state.label || 'Command line tool installed',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE:
      return {
        tone: 'warning',
        label: state.label || 'Command line tool update available',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT:
      return {
        tone: 'warning',
        label: state.label || 'Command line tool conflict',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_ERROR:
      return {
        tone: 'error',
        label: state.label || 'Command line tool check failed',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLING:
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATING:
      return {
        tone: 'active',
        label: state.label || 'Command line tool update running',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING:
      return {
        tone: 'muted',
        label: state.label || 'Command line tool not installed',
        detail: state.detail ?? '',
      }
    default:
      return {
        tone: 'muted',
        label: state.label || 'Checking command line tool',
        detail: state.detail ?? '',
      }
  }
}

function formatCLIIdentity(
  identity: DesktopCLIEntrypointIdentity | undefined,
): string {
  if (!identity?.manifestId) return ''
  const rev = identity.manifestRev ? ` rev ${identity.manifestRev}` : ''
  const platform = identity.platformId ? ` (${identity.platformId})` : ''
  return `${identity.manifestId}${rev}${platform}`
}

// WalkthroughSection renders the three-step CLI walkthrough bound to
// the active session (status, whoami, space list).
export function WalkthroughSection({ opts }: { opts: CommandOptions }) {
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
// /download/cli for the packaged binary and to the user-facing install
// and quickstart guide.
function InstallGuidanceSection() {
  const cliDownloadHref = useStaticHref('/download/cli')
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
  const developerCliHref = useStaticHref(
    '/docs/developers/cli/installation-and-commands',
  )

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
          href={developerCliHref}
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
      <div className="bg-brand/10 text-brand flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold">
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
              <LuUsers className="size-3" />
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
function StatusDot({
  tone,
}: {
  tone: 'ready' | 'warning' | 'muted' | 'active' | 'error'
}) {
  const cls =
    tone === 'ready'
      ? 'text-emerald-500'
      : tone === 'active'
        ? 'text-brand'
        : tone === 'error'
          ? 'text-danger'
          : tone === 'warning'
            ? 'text-amber-500'
            : 'text-foreground-alt/40'
  return (
    <LuCircle
      className={cn('h-2.5 w-2.5 shrink-0 fill-current', cls)}
      aria-hidden="true"
    />
  )
}
