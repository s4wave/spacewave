import { useCallback, useMemo, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { toast } from '@s4wave/web/ui/toaster.js'

import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import {
  getQuickstartOption,
  isQuickstartId,
} from '@s4wave/app/quickstart/options.js'
import { SelfEnrollmentGateState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import {
  type DashboardSpace,
  type DashboardOrg,
  SessionDashboard,
} from './dashboard/SessionDashboard.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { SessionFrame } from './SessionFrame.js'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { LuTriangleAlert } from 'react-icons/lu'

// CDN_SPACE_DISPLAY_NAME is the label shown for the process-scoped CDN Space
// on the session dashboard. The CDN SharedObject has no user-facing name;
// dashboards render a fixed label.
const CDN_SPACE_DISPLAY_NAME = 'Spacewave CDN'

// SessionDashboardContainer displays the session dashboard.
export function SessionDashboardContainer() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)

  const { providerId, accountId, isCloud } = useSessionInfo(session)

  const resourcesList = useWatchStateRpc(
    useCallback(
      (req: WatchResourcesListRequest, signal: AbortSignal) =>
        session?.watchResourcesList(req, signal) ?? null,
      [session],
    ),
    {},

    WatchResourcesListRequest.equals,

    WatchResourcesListResponse.equals,
  )

  const orgListCtx = SpacewaveOrgListContext.useContextSafe()
  const orgList = orgListCtx?.organizations

  const spaceToOrg = useMemo(() => {
    const map = new Map<string, string>()
    if (!orgList) return map
    for (const org of orgList) {
      if (!org.spaceIds || !org.id) continue
      for (const sid of org.spaceIds) {
        map.set(sid, org.id)
      }
    }
    return map
  }, [orgList])

  const dashboardOrgs: DashboardOrg[] | undefined = useMemo(
    () =>
      orgList
        ?.filter((o): o is typeof o & { id: string } => !!o.id)
        .map((o) => ({
          id: o.id,
          displayName: o.displayName ?? '',
        })),
    [orgList],
  )

  const accountResource = useMountAccount(providerId, accountId)
  const onboarding = SpacewaveOnboardingContext.useContextSafe()
  const rootResource = useRootResource()

  // cdnSpaceIdResource resolves the process-scoped default CDN Space ULID.
  // The Cdn handle is released on unmount via the cleanup register; the
  // underlying CdnInstance is owned by the server-side process registry and
  // outlives the handle. Returning just the ULID keeps the dashboard layer
  // free of CDN resource plumbing.
  const cdnSpaceIdResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      if (!root) return null
      const { cdn, cdnSpaceId } = await root.getCdn('', signal)
      cleanup(cdn)
      return cdnSpaceId || null
    },
    [],
  )
  const cdnSpaceId = useResourceValue(cdnSpaceIdResource) ?? null

  const dashboardSpaces: DashboardSpace[] | undefined = useMemo(() => {
    if (!resourcesList?.spacesList) return undefined
    const spaces: DashboardSpace[] = resourcesList.spacesList
      .filter((entry) => !!entry.entry?.ref?.providerResourceRef?.id)
      .map((entry) => {
        const id = entry.entry!.ref!.providerResourceRef!.id!
        return {
          id,
          name: entry.spaceMeta?.name ?? 'Untitled',
          orgId: spaceToOrg.get(id),
        }
      })
    // Inject the CDN Space under the org that claims it (typically the
    // platform/staging org). If no org lists the CDN ULID in space_ids,
    // skip injection so the CDN does not leak into the personal lane.
    if (cdnSpaceId) {
      const cdnOrgId = spaceToOrg.get(cdnSpaceId)
      if (cdnOrgId) {
        spaces.push({
          id: cdnSpaceId,
          name: CDN_SPACE_DISPLAY_NAME,
          orgId: cdnOrgId,
        })
      }
    }
    return spaces
  }, [resourcesList, spaceToOrg, cdnSpaceId])

  const sessionIdx = useSessionIndex()
  const deleteAt = onboarding?.onboarding?.deleteAt
  const isPendingDelete = onboarding?.isPendingDelete ?? false
  const isReadOnly = isPendingDelete || (onboarding?.isReadOnlyGrace ?? false)
  const selfEnrollmentStatus = useMemo(() => {
    switch (onboarding?.onboarding?.selfEnrollmentGateState) {
      case SelfEnrollmentGateState.UNKNOWN:
      case SelfEnrollmentGateState.CHECKING:
        return 'Checking connected spaces'
      case SelfEnrollmentGateState.AUTO_CONNECTING:
        return 'Connecting spaces'
      default:
        return undefined
    }
  }, [onboarding?.onboarding?.selfEnrollmentGateState])

  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()

  const handleSpaceClick = useCallback(
    (space: DashboardSpace) => {
      navigateSession({
        path:
          space.orgId ? `org/${space.orgId}/so/${space.id}` : `so/${space.id}`,
      })
    },
    [navigateSession],
  )

  const handleQuickstartClick = useCallback(
    (quickstartId: string) => {
      if (isReadOnly) {
        toast.error('This cloud account is read-only')
        return
      }
      // Options with a custom path are navigation actions, not space creators.
      // Pair navigates within the session context, others use their absolute path.
      if (!isQuickstartId(quickstartId)) return
      const opt = getQuickstartOption(quickstartId)
      if (opt.path) {
        if (quickstartId === 'pair') {
          navigateSession({ path: 'pair' })
        } else {
          navigate({ path: opt.path })
        }
        return
      }
      if (
        quickstartId === 'account' ||
        quickstartId === 'pair' ||
        quickstartId === 'local'
      ) {
        return
      }
      navigateSession({ path: `new/${quickstartId}` })
    },
    [isReadOnly, navigate, navigateSession],
  )

  const handleUndoDelete = useCallback(async () => {
    if (!session) return
    await session.spacewave.undoDeleteNow()
    toast.success(
      'Deletion canceled. The account stays read-only until you resubscribe.',
    )
  }, [session])

  const handleLogout = useCallback(async () => {
    if (!accountResource.value || !session || !sessionIdx) return
    const peerId = (await session.getSessionInfo()).peerId
    if (!peerId) return
    await accountResource.value.selfRevokeSession(peerId).catch(() => {})
    const root = rootResource.value
    if (root) {
      await root.deleteSession(sessionIdx).catch(() => {})
    }
    navigate({ path: '/sessions', replace: true })
  }, [accountResource.value, navigate, rootResource.value, session, sessionIdx])

  return (
    <SessionFrame>
      {isPendingDelete && (
        <PendingDeleteNotice
          deleteAt={deleteAt}
          onUndo={handleUndoDelete}
          onLogout={handleLogout}
        />
      )}
      <SessionDashboard
        spaces={dashboardSpaces}
        orgs={dashboardOrgs}
        onSpaceClick={handleSpaceClick}
        onQuickstartClick={handleQuickstartClick}
        isCloud={isCloud}
        accountResource={accountResource}
        session={session ?? undefined}
        readOnly={isReadOnly}
        topStatus={selfEnrollmentStatus}
      />
    </SessionFrame>
  )
}

function PendingDeleteNotice(props: {
  deleteAt?: bigint
  onUndo: () => Promise<void>
  onLogout: () => Promise<void>
}) {
  const [undoing, setUndoing] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)

  const handleUndoClick = useCallback(async () => {
    setUndoing(true)
    try {
      await props.onUndo()
    } finally {
      setUndoing(false)
    }
  }, [props])

  const handleLogoutClick = useCallback(async () => {
    setLoggingOut(true)
    try {
      await props.onLogout()
    } finally {
      setLoggingOut(false)
    }
  }, [props])

  return (
    <div className="border-warning/20 bg-warning/5 mx-auto mt-3 w-[calc(100%-1.5rem)] max-w-3xl rounded-lg border px-4 py-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="flex items-start gap-3">
          <LuTriangleAlert className="text-warning mt-0.5 h-4 w-4 shrink-0" />
          <div className="space-y-1">
            <p className="text-sm font-medium">
              Delete-now is active. This cloud account is read-only.
            </p>
            <p className="text-foreground-alt text-xs leading-relaxed">
              Export what you need from the dashboard. Undo stops deletion, but
              the account remains lapsed and read-only until you start a fresh
              subscription.
            </p>
            {props.deleteAt && (
              <p className="text-foreground-alt text-xs">
                Scheduled deletion:{' '}
                {new Date(Number(props.deleteAt)).toLocaleString()}
              </p>
            )}
          </div>
        </div>
        <div className="flex flex-col gap-2 md:w-56">
          <button
            onClick={() => void handleUndoClick()}
            disabled={undoing}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 rounded-md border px-3 py-2 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            {undoing ? 'Canceling deletion...' : 'Undo Deletion'}
          </button>
          <button
            onClick={() => void handleLogoutClick()}
            disabled={loggingOut}
            className="border-warning/20 bg-warning/10 hover:bg-warning/20 rounded-md border px-3 py-2 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            {loggingOut ? 'Logging out...' : 'Log Out'}
          </button>
        </div>
      </div>
    </div>
  )
}
