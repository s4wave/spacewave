import { useCallback, useMemo, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuTriangleAlert } from '@s4wave/web/images/icons.js'

import { useVisibleQuickstartOptions } from '@s4wave/app/quickstart/useQuickstartOptions.js'
import { SelfEnrollmentGateState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'
import {
  SessionContext,
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useObjectTypeMetadata } from '@s4wave/web/hooks/useObjectTypeMetadata.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import {
  type DashboardSpace,
  type DashboardOrg,
  SessionDashboard,
} from './dashboard/SessionDashboard.js'
import { LogoutConfirmDialog } from './LogoutConfirmDialog.js'
import { SessionFrame } from './SessionFrame.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginInline: 'auto',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    width: themeLayout['--width-content-inset'],
    maxWidth: 'var(--container-3xl)',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 48rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 48rem)': 'center' },
    justifyContent: {
      default: null,
      '@media (min-width: 48rem)': 'space-between',
    },
  },
  rule3: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    width: {
      default: null,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 56)`,
    },
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule9: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
})

// CDN_SPACE_DISPLAY_NAME is the label shown for the process-scoped CDN Space
// on the session dashboard. The CDN SharedObject has no user-facing name;
// dashboards render a fixed label.
const CDN_SPACE_DISPLAY_NAME = 'Spacewave CDN'

function formatScheduledDeletion(
  timestampMs: number | string | bigint,
): string {
  return new Date(Number(timestampMs)).toLocaleString()
}

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
    { includeIndexObjectTypes: true },
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
      orgList?.flatMap((o) =>
        o.id
          ? [
              {
                id: o.id,
                displayName: o.displayName ?? '',
              },
            ]
          : [],
      ),
    [orgList],
  )

  const accountResource = useMountAccount(providerId, accountId)
  const onboarding = SpacewaveOnboardingContext.useContextSafe()
  const rootResource = useRootResource()
  const objectTypeMetadataById = useObjectTypeMetadata(rootResource)

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
    const spaces: DashboardSpace[] = resourcesList.spacesList.flatMap(
      (entry) => {
        const id = entry.entry?.ref?.providerResourceRef?.id
        return id
          ? [
              {
                id,
                name: entry.spaceMeta?.name ?? 'Untitled',
                orgId: spaceToOrg.get(id),
                source: entry.entry?.source,
                objectType: entry.indexObjectType || undefined,
              },
            ]
          : []
      },
    )
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
  const quickstartOptions = useVisibleQuickstartOptions()
  const quickstartOptionById = useMemo(
    () => new Map(quickstartOptions.map((opt) => [opt.id, opt])),
    [quickstartOptions],
  )

  const handleSpaceClick = useCallback(
    (space: DashboardSpace) => {
      navigateSession({
        path: space.orgId
          ? `org/${space.orgId}/so/${space.id}`
          : `so/${space.id}`,
      })
    },
    [navigateSession],
  )

  const handleQuickstartClick = useCallback(
    (quickstartId: string, storage?: string) => {
      if (isReadOnly) {
        toast.error('This cloud account is read-only')
        return
      }
      // Options with a custom path are navigation actions, not space creators.
      // Pair navigates within the session context, others use their absolute path.
      const opt = quickstartOptionById.get(quickstartId)
      if (!opt) return
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
      navigateSession({
        path: storage
          ? `new/${quickstartId}/on/${storage}`
          : `new/${quickstartId}`,
      })
    },
    [isReadOnly, navigate, navigateSession, quickstartOptionById],
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
        objectTypeMetadataById={objectTypeMetadataById}
      />
    </SessionFrame>
  )
}

function PendingDeleteNotice({
  deleteAt,
  onUndo,
  onLogout,
}: {
  deleteAt?: bigint
  onUndo: () => Promise<void>
  onLogout: () => Promise<void>
}) {
  const [undoing, setUndoing] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutOpen, setLogoutOpen] = useState(false)

  const handleUndoClick = useCallback(async () => {
    setUndoing(true)
    try {
      await onUndo()
    } finally {
      setUndoing(false)
    }
  }, [onUndo])

  const handleLogoutClick = useCallback(async () => {
    setLoggingOut(true)
    try {
      await onLogout()
    } finally {
      setLoggingOut(false)
    }
  }, [onLogout])

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuTriangleAlert sx={stylexStyles.rule1} />
          <div>
            <p sx={[nativeChildSpacing.s1, stylexStyles2.rule4]}>
              Delete-now is active. This cloud account is read-only.
            </p>
            <p sx={[nativeChildSpacing.s1, stylexStyles2.rule5]}>
              Export what you need from the dashboard. Undo stops deletion, but
              the account remains lapsed and read-only until you start a fresh
              subscription.
            </p>
            {deleteAt && (
              <p
                suppressHydrationWarning
                sx={[nativeChildSpacing.s1, stylexStyles2.rule6]}
              >
                Scheduled deletion: {formatScheduledDeletion(deleteAt)}
              </p>
            )}
          </div>
        </div>
        <div sx={stylexStyles2.rule7}>
          <button
            type="button"
            onClick={() => void handleUndoClick()}
            disabled={undoing}
            sx={stylexStyles2.rule8}
          >
            {undoing ? 'Canceling deletion…' : 'Undo Deletion'}
          </button>
          <button
            type="button"
            onClick={() => setLogoutOpen(true)}
            disabled={loggingOut}
            sx={stylexStyles2.rule9}
          >
            {loggingOut ? 'Logging out…' : 'Log Out'}
          </button>
        </div>
      </div>
      <LogoutConfirmDialog
        open={logoutOpen}
        onOpenChange={setLogoutOpen}
        loggingOut={loggingOut}
        onConfirm={() => void handleLogoutClick()}
      />
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
