import { useCallback, useState, type ComponentProps } from 'react'
import {
  LuChevronDown,
  LuChevronRight,
  LuFolderOpen,
  LuFolderPlus,
  LuHardDrive,
  LuTrash2,
  LuTriangleAlert,
  LuUser,
  LuUserPlus,
} from 'react-icons/lu'
import { isDesktop } from '@aptre/bldr'

import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { useSpaceRootAliases } from '@s4wave/app/hooks/useSpaceRootAliases.js'
import { useSpaceRootRuntime } from '@s4wave/app/hooks/useSpaceRootRuntime.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { useSessionAccountStatuses } from '@s4wave/app/hooks/useSessionAccountStatuses.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useSelectAccount } from '@s4wave/app/hooks/useSelectAccount.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  SpaceRootRuntimeStatus,
  SpaceRootStatus,
  type SpaceRootRuntimeSession,
  type SpaceRootAliasRecord,
  type WatchSpaceRootRuntimeResponse,
} from '@s4wave/sdk/root/root.pb.js'

import { accountDescription, accountTitle } from './account-presentation.js'

// SessionSelector displays a full-page session picker for users with multiple sessions.
export function SessionSelector() {
  const resource = useSessionList()
  const rootAliases = useSpaceRootAliases()
  const addRootAlias = useAddSpaceRootAlias()
  const accountStatuses = useSessionAccountStatuses()
  const navigate = useNavigate()
  const sessions = resource.value?.sessions ?? []
  const rootRecords = rootAliases.value?.records ?? []
  // undefined means the user has not chosen; null means every root is closed.
  const [chosenRootAliasId, setChosenRootAliasId] = useState<
    string | null | undefined
  >(undefined)
  // Without local sessions, the first ready root opens by default.
  const selectedRootAliasId =
    chosenRootAliasId !== undefined
      ? chosenRootAliasId
      : sessions.length === 0
        ? (rootRecords.find(isRootAliasReady)?.aliasId ?? null)
        : null
  const selectedRootRuntime = useSpaceRootRuntime(selectedRootAliasId)

  const handleAddAccount = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  const handleHome = useCallback(() => {
    navigate({ path: '/landing' })
  }, [navigate])

  if (resource.loading || (sessions.length === 0 && rootAliases.loading)) {
    return (
      <SessionSelectorStatus
        view={{
          state: 'loading',
          title: 'Loading sessions',
          detail: 'Reading available sessions from the provider.',
        }}
      />
    )
  }

  if (resource.error) {
    return (
      <SessionSelectorStatus
        view={{
          state: 'error',
          title: 'Failed to load sessions',
          error: resource.error.message,
          onRetry: resource.retry,
        }}
      />
    )
  }

  if (sessions.length === 0 && rootRecords.length === 0) {
    return <NavigatePath to="/landing" replace />
  }

  return (
    <div
      className="bg-background-landing relative flex h-full w-full flex-col overflow-x-hidden overflow-y-auto"
      data-testid="session-selector"
    >
      <BackButton floating onClick={handleHome}>
        Home
      </BackButton>

      <div className="relative z-10 flex min-h-full flex-1 flex-col items-center justify-center px-4 py-12">
        <AnimatedLogo followMouse={true} containerClassName="mb-6" />

        <h1 className="text-2xl font-semibold tracking-wide">Welcome back</h1>
        <p className="text-foreground-alt/60 mb-6 text-sm">
          {sessions.length > 0
            ? 'Choose a session to continue'
            : 'Accounts found in your state roots'}
        </p>

        {sessions.length > 0 && (
          <div className="w-full max-w-md space-y-2">
            {sessions.map((session) => (
              <SessionCard
                key={session.sessionIndex}
                session={session}
                accountStatus={accountStatuses.get(session.sessionIndex ?? 0)}
              />
            ))}
          </div>
        )}

        <StateRootList
          records={rootRecords}
          showHeading={sessions.length > 0}
          selectedAliasId={selectedRootAliasId}
          selectedRuntime={selectedRootRuntime.value}
          onOpenChange={setChosenRootAliasId}
        />

        <SelectorActions
          addRootAlias={addRootAlias}
          onAddAccount={handleAddAccount}
          onRootAdded={setChosenRootAliasId}
        />
      </div>

      <div className="relative z-10 pb-3 text-center">
        <p className="text-foreground-alt/60 text-xs">
          local-first · encrypted
        </p>
      </div>
    </div>
  )
}

// SessionSelectorStatus renders a centered loading or error card.
function SessionSelectorStatus(props: {
  view: ComponentProps<typeof LoadingCard>['view']
}) {
  return (
    <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard view={props.view} />
      </div>
    </div>
  )
}

// StateRootList renders the configured state roots, opening the selected one.
function StateRootList(props: {
  records: SpaceRootAliasRecord[]
  showHeading: boolean
  selectedAliasId: string | null
  selectedRuntime?: WatchSpaceRootRuntimeResponse | null
  onOpenChange: (aliasId: string | null) => void
}) {
  const { records, selectedAliasId } = props
  if (records.length === 0) return null

  return (
    <div className="w-full max-w-md">
      {props.showHeading && (
        <h2 className="text-foreground-alt/60 mt-6 mb-2 px-1 text-xs font-medium">
          State roots
        </h2>
      )}
      <div className="space-y-2">
        {records.map((record) => (
          <SpaceRootCard
            key={record.aliasId}
            record={record}
            open={record.aliasId === selectedAliasId}
            runtime={
              record.aliasId === selectedAliasId ? props.selectedRuntime : null
            }
            onOpenChange={props.onOpenChange}
          />
        ))}
      </div>
    </div>
  )
}

// SelectorActions renders the add-account and add-state-root controls.
// onRootAdded receives the alias of a newly added state root.
function SelectorActions(props: {
  addRootAlias: ReturnType<typeof useAddSpaceRootAlias>
  onAddAccount: () => void
  onRootAdded: (aliasId: string) => void
}) {
  const { addRootAlias, onRootAdded } = props

  return (
    <>
      <div className="mt-6 flex items-center justify-center gap-3">
        <Button variant="outline" onClick={props.onAddAccount}>
          <LuUserPlus className="size-4" />
          Add account
        </Button>
        <Button
          variant="outline"
          onClick={() => {
            void addRootAlias.add().then((aliasId) => {
              if (aliasId) onRootAdded(aliasId)
            })
          }}
          disabled={!addRootAlias.canAdd}
        >
          <LuFolderPlus className="size-4" />
          {addRootAlias.adding ? 'Adding state root' : 'Add state root'}
        </Button>
      </div>
      {!isDesktop && (
        <p className="text-foreground-alt/50 mt-2 text-xs">
          State roots can be added in the desktop app.
        </p>
      )}
    </>
  )
}

// SpaceRootCard renders a configured state root and, when open, its accounts.
function SpaceRootCard(props: {
  record: SpaceRootAliasRecord
  open: boolean
  runtime?: WatchSpaceRootRuntimeResponse | null
  onOpenChange: (aliasId: string | null) => void
}) {
  const { record, open, runtime, onOpenChange } = props
  const rootResource = useRootResource()
  const root = rootResource.value
  const [removing, setRemoving] = useState(false)
  const ready = isRootAliasReady(record)
  const aliasId = record.aliasId ?? ''

  const handleRemove = useCallback(async () => {
    if (!root || !aliasId || removing) return
    setRemoving(true)
    try {
      await root.removeSpaceRootAlias(aliasId)
      if (open) onOpenChange(null)
    } catch (err) {
      toast.error('Could not remove state root', { description: String(err) })
    } finally {
      setRemoving(false)
    }
  }, [aliasId, onOpenChange, open, removing, root])

  const handleToggle = useCallback(() => {
    if (!ready || !aliasId) return
    onOpenChange(open ? null : aliasId)
  }, [aliasId, onOpenChange, open, ready])

  return (
    <div
      className="border-foreground/10 rounded-lg border"
      data-testid="space-root-card"
    >
      <div className="flex items-center gap-1 pr-2">
        <button
          type="button"
          onClick={handleToggle}
          disabled={!ready}
          aria-expanded={open}
          className="hover:bg-foreground/5 flex min-w-0 flex-1 items-center gap-3 rounded-lg px-4 py-3 text-left transition-colors disabled:cursor-default disabled:hover:bg-transparent"
        >
          <div className="bg-foreground/5 flex size-9 shrink-0 items-center justify-center rounded-lg">
            {ready ? (
              <LuHardDrive className="size-4" />
            ) : (
              <LuTriangleAlert className="text-warning size-4" />
            )}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-foreground truncate text-sm font-medium">
              {record.displayName || aliasId}
            </div>
            <div
              className={cn(
                'truncate text-xs',
                ready ? 'text-foreground-alt/60' : 'text-warning',
              )}
            >
              {record.statusMessage || record.native?.path}
            </div>
          </div>
          {ready && (
            <LuChevronDown
              className={cn(
                'text-foreground-alt/40 size-4 shrink-0 transition-transform',
                !open && '-rotate-90',
              )}
            />
          )}
        </button>
        <Button
          onClick={() => {
            void handleRemove()
          }}
          disabled={!root || removing}
          aria-label="Remove state root"
          variant="muted"
          size="iconSm"
        >
          <LuTrash2 className="size-4" />
        </Button>
      </div>
      {open && <SpaceRootAccounts runtime={runtime} />}
    </div>
  )
}

// isRootAliasReady reports whether a configured root can be opened.
function isRootAliasReady(record: SpaceRootAliasRecord): boolean {
  return (
    record.status === SpaceRootStatus.SpaceRootStatus_READY ||
    record.status === SpaceRootStatus.SpaceRootStatus_UNKNOWN
  )
}

// SpaceRootAccounts renders the accounts served by an open state root.
function SpaceRootAccounts(props: {
  runtime?: WatchSpaceRootRuntimeResponse | null
}) {
  const runtime = props.runtime
  const status = runtime?.status

  if (status === SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_ERROR) {
    return (
      <div className="border-foreground/10 text-warning flex items-start gap-2 border-t px-4 py-3 text-xs">
        <LuTriangleAlert className="mt-px size-3.5 shrink-0" />
        <span>{runtime?.error || 'State root unavailable'}</span>
      </div>
    )
  }
  if (status !== SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_READY) {
    return (
      <div className="border-foreground/10 text-foreground-alt/60 flex items-center gap-2 border-t px-4 py-3 text-xs">
        <Spinner size="sm" />
        {status === SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_STARTING
          ? 'Starting the state root daemon'
          : 'Connecting to the state root'}
      </div>
    )
  }

  const runtimeSessions = runtime?.runtimeSessions?.length
    ? runtime.runtimeSessions
    : (runtime?.sessions ?? []).map((session): SpaceRootRuntimeSession => ({
        session,
      }))
  if (runtimeSessions.length === 0) {
    return (
      <div className="border-foreground/10 text-foreground-alt/60 border-t px-4 py-3 text-xs">
        No accounts in this state root.
      </div>
    )
  }

  return (
    <div className="border-foreground/10 divide-foreground/10 divide-y border-t">
      {runtimeSessions.map((runtimeSession) => (
        <SpaceRootAccount
          key={runtimeSession.session?.sessionIndex}
          runtimeSession={runtimeSession}
        />
      ))}
    </div>
  )
}

// SpaceRootAccount renders one account and its spaces in a state root.
function SpaceRootAccount(props: { runtimeSession: SpaceRootRuntimeSession }) {
  const { runtimeSession } = props
  const meta = runtimeSession.metadata
  const provider =
    meta?.providerDisplayName ||
    (meta?.providerId === 'spacewave' ? 'Cloud' : 'Local')
  const title =
    meta?.displayName || meta?.cloudEntityId || `${provider} account`
  const accountId = meta?.providerAccountId
  const spaces = runtimeSession.spaces ?? []

  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-3">
        <div className="bg-foreground/5 flex size-9 shrink-0 items-center justify-center rounded-lg">
          <LuUser className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-foreground truncate text-sm font-medium">
            {title}
          </div>
          <div className="text-foreground-alt/60 flex min-w-0 gap-1.5 text-xs">
            <span className="shrink-0">
              {spaces.length === 1 ? '1 space' : `${spaces.length} spaces`}
            </span>
            {accountId && (
              <span className="truncate font-mono" title={accountId}>
                · {accountId}
              </span>
            )}
          </div>
        </div>
      </div>
      {spaces.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5 pl-12">
          {spaces.map((space) => (
            <span
              key={
                space.entry?.ref?.providerResourceRef?.id ??
                [space.entry?.source, space.spaceMeta?.name].join(':')
              }
              className="bg-foreground/5 text-foreground-alt/80 inline-flex max-w-full items-center gap-1.5 rounded-md px-2 py-1 text-xs"
            >
              <LuFolderOpen className="size-3.5 shrink-0" />
              <span className="truncate">
                {space.spaceMeta?.name || 'Untitled space'}
              </span>
            </span>
          ))}
        </div>
      )}
      {runtimeSession.error && (
        <div className="text-warning mt-2 pl-12 text-xs">
          {runtimeSession.error}
        </div>
      )}
    </div>
  )
}

// SessionCard renders a single session entry in the selector list.
function SessionCard(props: {
  session: SessionListEntry
  accountStatus?: ProviderAccountStatus
}) {
  const selectAccount = useSelectAccount()
  const meta = useSessionMetadata(props.session.sessionIndex ?? null)
  const isCloudProvider = meta?.providerId === 'spacewave'
  const isLinked = meta?.providerId === 'local' && !!meta?.cloudAccountId
  const isInactive =
    props.accountStatus === ProviderAccountStatus.ProviderAccountStatus_DORMANT
  const sessionIndex = props.session.sessionIndex ?? 0
  const title = accountTitle(meta, sessionIndex)
  const subtitle = accountDescription(meta, sessionIndex)

  const handleSessionSelect = useCallback(() => {
    if (sessionIndex) selectAccount(sessionIndex)
  }, [selectAccount, sessionIndex])

  const handleSessionSelectKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      handleSessionSelect()
    },
    [handleSessionSelect],
  )

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="session-card"
      data-session-index={props.session.sessionIndex ?? ''}
      onClick={handleSessionSelect}
      onKeyDown={handleSessionSelectKeyDown}
      className={cn(
        'border-foreground/10 hover:bg-foreground/5 flex cursor-pointer items-center gap-3 rounded-lg border px-4 py-3 transition-colors',
        isLinked && 'opacity-50',
      )}
    >
      <div className="bg-foreground/5 flex size-9 items-center justify-center rounded-lg">
        <LuUser className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-sm font-medium">{title}</span>
          {isCloudProvider && (
            <span className="bg-brand/15 text-brand micro-nine rounded-full px-1.5 py-0.5 font-semibold tracking-wider uppercase">
              Cloud
            </span>
          )}
          {isLinked && (
            <span className="text-foreground-alt/80 micro-ten rounded-full px-1.5 py-0.5 font-medium">
              (linked)
            </span>
          )}
          {isInactive && (
            <span className="bg-foreground/6 text-foreground-alt/75 micro-ten rounded-full px-1.5 py-0.5 font-medium">
              (Inactive)
            </span>
          )}
        </div>
        <div className="text-foreground-alt/60 truncate text-xs">
          {subtitle}
        </div>
      </div>
      <LuChevronRight className="text-foreground-alt/40 size-4" />
    </div>
  )
}
