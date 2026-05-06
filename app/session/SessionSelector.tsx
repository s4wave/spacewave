import { useCallback, useState } from 'react'
import {
  LuChevronRight,
  LuFolderOpen,
  LuLoaderCircle,
  LuPlug,
  LuTrash2,
  LuTriangleAlert,
  LuUser,
} from 'react-icons/lu'
import { isDesktop } from '@aptre/bldr'

import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { useSpaceRootAliases } from '@s4wave/app/hooks/useSpaceRootAliases.js'
import { useSpaceRootRuntime } from '@s4wave/app/hooks/useSpaceRootRuntime.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { useSessionAccountStatuses } from '@s4wave/app/hooks/useSessionAccountStatuses.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
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

// SessionSelector displays a full-page session picker for users with multiple sessions.
export function SessionSelector() {
  const resource = useSessionList()
  const rootAliases = useSpaceRootAliases()
  const addRootAlias = useAddSpaceRootAlias()
  const accountStatuses = useSessionAccountStatuses()
  const navigate = useNavigate()
  const sessions = resource.value?.sessions ?? []
  const [selectedRootAliasId, setSelectedRootAliasId] = useState<string | null>(
    null,
  )
  const selectedRootRuntime = useSpaceRootRuntime(selectedRootAliasId)

  const handleAddAccount = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  const handleHome = useCallback(() => {
    navigate({ path: '/landing' })
  }, [navigate])

  if (resource.loading) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Loading sessions',
              detail: 'Reading available sessions from the provider.',
            }}
          />
        </div>
      </div>
    )
  }

  if (resource.error) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'error',
              title: 'Failed to load sessions',
              error: resource.error.message,
              onRetry: resource.retry,
            }}
          />
        </div>
      </div>
    )
  }

  if (sessions.length === 0) {
    return <NavigatePath to="/landing" replace />
  }

  return (
    <div className="bg-background-landing relative flex h-full w-full flex-col overflow-x-hidden overflow-y-auto">
      <BackButton floating onClick={handleHome}>
        Home
      </BackButton>

      <div className="relative z-10 flex min-h-full flex-1 flex-col items-center justify-center px-4 py-12">
        <AnimatedLogo followMouse={true} containerClassName="mb-6" />

        <h1 className="text-2xl font-bold tracking-wide">Welcome back</h1>
        <p className="text-foreground-alt/60 mb-6 text-sm">
          Choose a session to continue
        </p>

        <div className="w-full max-w-md space-y-2">
          {sessions.map((session) => (
            <SessionCard
              key={session.sessionIndex}
              session={session}
              accountStatus={accountStatuses.get(session.sessionIndex ?? 0)}
            />
          ))}
        </div>

        <div className="mt-4 w-full max-w-md space-y-2">
          {(rootAliases.value?.records ?? []).map((record) => (
            <SpaceRootAliasCard
              key={record.aliasId}
              record={record}
              selected={record.aliasId === selectedRootAliasId}
              onSelect={setSelectedRootAliasId}
              onRemoveSelected={setSelectedRootAliasId}
            />
          ))}
        </div>

        {selectedRootAliasId && (
          <SpaceRootRuntimePanel runtime={selectedRootRuntime.value} />
        )}

        <div className="mt-6 flex items-center justify-center gap-3">
          <Button variant="outline" onClick={handleAddAccount}>
            Add account
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              void addRootAlias.add()
            }}
            disabled={!addRootAlias.canAdd}
          >
            <LuFolderOpen className="h-4 w-4" />
            {addRootAlias.adding ? 'Adding root' : 'Add state root'}
          </Button>
        </div>
        {!isDesktop && (
          <p className="text-foreground-alt/50 mt-2 text-xs">
            State root loading is available in the desktop app.
          </p>
        )}
        {isDesktop && (
          <p className="text-foreground-alt/50 mt-2 text-xs">
            Select an existing .spacewave state directory. .s4wave files are
            deferred.
          </p>
        )}
      </div>

      <div className="relative z-10 pb-3 text-center">
        <p className="text-foreground-alt/60 text-xs">
          local-first · encrypted
        </p>
      </div>
    </div>
  )
}

// SpaceRootAliasCard renders a configured local state root entry.
function SpaceRootAliasCard(props: {
  record: SpaceRootAliasRecord
  selected: boolean
  onSelect: (aliasId: string) => void
  onRemoveSelected: (aliasId: string | null) => void
}) {
  const { record, selected, onSelect, onRemoveSelected } = props
  const rootResource = useRootResource()
  const root = rootResource.value
  const [removing, setRemoving] = useState(false)
  const ready =
    record.status === SpaceRootStatus.SpaceRootStatus_READY ||
    record.status === SpaceRootStatus.SpaceRootStatus_UNKNOWN
  const path = record.native?.path ?? ''

  const handleRemove = useCallback(async () => {
    if (!root || !record.aliasId || removing) return
    setRemoving(true)
    try {
      await root.removeSpaceRootAlias(record.aliasId)
      if (selected) onRemoveSelected(null)
    } catch (err) {
      toast.error('Could not remove state root', { description: String(err) })
    } finally {
      setRemoving(false)
    }
  }, [onRemoveSelected, record.aliasId, selected, removing, root])

  const handleSelect = useCallback(() => {
    if (!ready || !record.aliasId) return
    onSelect(record.aliasId)
  }, [onSelect, record.aliasId, ready])

  return (
    <div className="border-foreground/10 flex items-center gap-3 rounded-lg border px-4 py-3">
      <div className="bg-brand/10 flex h-9 w-9 items-center justify-center rounded-lg">
        {ready ?
          <LuFolderOpen className="h-4 w-4" />
        : <LuTriangleAlert className="text-warning h-4 w-4" />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-foreground truncate text-sm font-medium">
            {record.displayName || record.aliasId}
          </span>
          <span className="bg-foreground/6 text-foreground-alt/75 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
            State root
          </span>
        </div>
        <div className="text-foreground-alt/60 truncate text-xs">
          {record.statusMessage || path}
        </div>
      </div>
      <Button
        variant={selected ? 'secondary' : 'outline'}
        onClick={handleSelect}
        disabled={!ready}
        className="h-8 px-2 text-xs"
      >
        <LuPlug className="h-3.5 w-3.5" />
        {selected ? 'Using' : 'Use'}
      </Button>
      <Button
        variant="ghost"
        size="icon"
        onClick={() => {
          void handleRemove()
        }}
        disabled={!root || removing}
        aria-label="Remove state root"
        className="text-foreground-alt/60 hover:text-foreground h-8 w-8"
      >
        <LuTrash2 className="h-4 w-4" />
      </Button>
    </div>
  )
}

// SpaceRootRuntimePanel renders the selected root daemon status and sessions.
function SpaceRootRuntimePanel(props: {
  runtime?: WatchSpaceRootRuntimeResponse | null
}) {
  const runtime = props.runtime
  const sessions = runtime?.sessions ?? []
  const runtimeSessions =
    runtime?.runtimeSessions?.length ?
      runtime.runtimeSessions
    : sessions.map((session): SpaceRootRuntimeSession => ({ session }))
  const statusLabel = runtimeStatusLabel(runtime?.status)
  const loading =
    !runtime ||
    runtime.status ===
      SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_CONNECTING ||
    runtime.status === SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_STARTING

  return (
    <div className="border-foreground/10 mt-4 w-full max-w-md rounded-lg border px-4 py-3">
      <div className="flex items-center gap-2">
        {loading ?
          <LuLoaderCircle className="h-4 w-4 animate-spin" />
        : (
          runtime?.status ===
          SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_ERROR
        ) ?
          <LuTriangleAlert className="text-warning h-4 w-4" />
        : <LuPlug className="h-4 w-4" />}
        <span className="text-foreground text-sm font-medium">
          {statusLabel}
        </span>
      </div>
      {runtime?.error && (
        <div className="text-warning mt-2 text-xs">{runtime.error}</div>
      )}
      {runtime?.statePath && (
        <div className="text-foreground-alt/60 mt-1 truncate text-xs">
          {runtime.statePath}
        </div>
      )}
      {runtimeSessions.length > 0 && (
        <div className="mt-3 space-y-2">
          {runtimeSessions.map((runtimeSession) => (
            <div
              key={runtimeSession.session?.sessionIndex}
              className="bg-foreground/5 rounded-md px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <LuUser className="h-4 w-4" />
                <div className="min-w-0">
                  <div className="text-foreground truncate text-sm">
                    {runtimeSessionTitle(runtimeSession)}
                  </div>
                  <div className="text-foreground-alt/60 truncate text-xs">
                    {runtimeSessionSubtitle(runtimeSession)}
                  </div>
                </div>
              </div>
              {runtimeSession.spaces && runtimeSession.spaces.length > 0 && (
                <div className="mt-2 space-y-1 pl-6">
                  {runtimeSession.spaces.map((space, index) => (
                    <div
                      key={
                        space.entry?.ref?.providerResourceRef?.id ??
                        `${runtimeSession.session?.sessionIndex}-space-${index}`
                      }
                      className="text-foreground-alt/80 flex items-center gap-2 text-xs"
                    >
                      <LuFolderOpen className="h-3.5 w-3.5" />
                      <span className="truncate">
                        {space.spaceMeta?.name || 'Untitled space'}
                      </span>
                    </div>
                  ))}
                </div>
              )}
              {runtimeSession.error && (
                <div className="text-warning mt-2 pl-6 text-xs">
                  {runtimeSession.error}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      {runtime?.status ===
        SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_READY &&
        sessions.length === 0 && (
          <div className="text-foreground-alt/60 mt-2 text-xs">
            No sessions in this state root.
          </div>
        )}
    </div>
  )
}

function runtimeSessionTitle(session: SpaceRootRuntimeSession): string {
  return (
    session.metadata?.displayName ||
    session.metadata?.cloudEntityId ||
    session.metadata?.providerAccountId ||
    `Session ${session.session?.sessionIndex ?? ''}`
  )
}

function runtimeSessionSubtitle(session: SpaceRootRuntimeSession): string {
  const provider =
    session.metadata?.providerDisplayName || session.metadata?.providerId
  const count = session.spaces?.length ?? 0
  const spaces = count === 1 ? '1 space' : `${count} spaces`
  if (provider) {
    return `${provider} - ${spaces}`
  }
  return spaces
}

function runtimeStatusLabel(status?: SpaceRootRuntimeStatus): string {
  switch (status) {
    case SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_CONNECTING:
      return 'Connecting to state root'
    case SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_STARTING:
      return 'Starting state root daemon'
    case SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_READY:
      return 'State root ready'
    case SpaceRootRuntimeStatus.SpaceRootRuntimeStatus_ERROR:
      return 'State root unavailable'
    default:
      return 'Preparing state root'
  }
}

// SessionCard renders a single session entry in the selector list.
function SessionCard(props: {
  session: SessionListEntry
  accountStatus?: ProviderAccountStatus
}) {
  const navigate = useNavigate()
  const meta = useSessionMetadata(props.session.sessionIndex ?? null)
  const isCloudProvider = meta?.providerId === 'spacewave'
  const isLinked = meta?.providerId === 'local' && !!meta?.cloudAccountId
  const isInactive =
    props.accountStatus === ProviderAccountStatus.ProviderAccountStatus_DORMANT
  const title =
    meta?.displayName ||
    meta?.cloudEntityId ||
    `Session ${props.session.sessionIndex}`
  const providerLabel =
    meta?.providerDisplayName || (isCloudProvider ? 'Cloud' : 'Local')
  const subtitle =
    isCloudProvider && meta?.cloudEntityId && meta.cloudEntityId !== title ?
      `${providerLabel} · ${meta.cloudEntityId}`
    : !isCloudProvider && !meta?.displayName ?
      `${providerLabel} · Session ${props.session.sessionIndex}`
    : providerLabel

  const handleClick = useCallback(() => {
    navigate({ path: '/u/' + props.session.sessionIndex + '/' })
  }, [navigate, props.session.sessionIndex])

  return (
    <div
      onClick={handleClick}
      className={cn(
        'border-foreground/10 hover:bg-foreground/5 flex cursor-pointer items-center gap-3 rounded-lg border px-4 py-3 transition-colors',
        isLinked && 'opacity-50',
      )}
    >
      <div className="bg-foreground/5 flex h-9 w-9 items-center justify-center rounded-lg">
        <LuUser className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-sm font-medium">{title}</span>
          {isCloudProvider && (
            <span className="bg-brand/15 text-brand rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase">
              Cloud
            </span>
          )}
          {isLinked && (
            <span className="text-foreground-alt/80 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
              (linked)
            </span>
          )}
          {isInactive && (
            <span className="bg-foreground/6 text-foreground-alt/75 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
              (Inactive)
            </span>
          )}
        </div>
        <div className="text-foreground-alt/60 truncate text-xs">
          {subtitle}
        </div>
      </div>
      <LuChevronRight className="text-foreground-alt/40 h-4 w-4" />
    </div>
  )
}
