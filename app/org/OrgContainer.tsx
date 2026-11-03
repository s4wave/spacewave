import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import { createContext, useCallback, use, useMemo } from 'react'
import { LuArrowUp, LuBuilding2 } from '@s4wave/web/images/icons.js'
import { useWatchStateRpc } from '@aptre/bldr-react'

import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import {
  type OrgSpaceInfo,
  WatchOrganizationStateRequest,
  WatchOrganizationStateResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import {
  Route,
  Routes,
  useNavigate,
  useParams,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { bottomBarIconProps } from '@s4wave/web/frame/bottom-icon-props.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'

import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { SessionSharedObjectContainer } from '@s4wave/app/session/SessionSharedObjectContainer.js'

import { ORG_ROLE_OWNER } from './org-constants.js'
import { OrganizationDashboard } from './OrganizationDashboard.js'
import { OrganizationDetails } from './OrganizationDetails.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 36)`,
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '9px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    marginLeft: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

// OrgContainerState holds the watched organization state provided to children.

export interface OrgContainerState {
  orgId: string
  orgState: WatchOrganizationStateResponse | null
  orgName: string
  degraded: boolean
  isOwner: boolean
  spaces: OrgSpaceInfo[]
  billingAccountId: string
}

const OrgContainerContext = createContext<OrgContainerState | null>(null)

// useOrgContainerState returns the org state from the nearest OrgContainer.
export function useOrgContainerState(): OrgContainerState {
  const ctx = use(OrgContainerContext)
  if (!ctx)
    throw new Error('useOrgContainerState must be used inside OrgContainer')
  return ctx
}

// OrgContainer wraps organization routes in a BottomBarLevel, providing the
// org button in the bottom bar and OrganizationDetails as the overlay panel.
// Mirrors the SessionContainer pattern for sessions.
export function OrgContainer() {
  const params = useParams()
  const orgId = params.orgId ?? ''
  const sessionResource = SessionContext.useContext()
  const session = sessionResource.value
  const navigate = useNavigate()
  const path = usePath()
  const parentPaths = useParentPaths()
  const currentLevelPath = parentPaths[parentPaths.length - 1] ?? path
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  const setOpenMenu = useBottomBarSetOpenMenu()

  const orgState = useWatchStateRpc(
    useCallback(
      (req: WatchOrganizationStateRequest, signal: AbortSignal) =>
        session?.spacewave.watchOrganizationState(req.orgId ?? orgId, signal) ??
        null,
      [session, orgId],
    ),
    { orgId },
    WatchOrganizationStateRequest.equals,
    WatchOrganizationStateResponse.equals,
  )
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

  const orgListInfo =
    orgListCtx?.organizations.find((org) => org.id === orgId) ?? null
  const info = orgState?.organization ?? orgListInfo ?? null
  const degraded = !orgState
  const isOwner = info?.role === ORG_ROLE_OWNER
  const orgName = info?.displayName || 'Organization'
  const billingAccountId = info?.billingAccountId ?? ''
  const spaces = useMemo<OrgSpaceInfo[]>(() => {
    if (orgState?.spaces) {
      return orgState.spaces
    }
    const orgSpaceIds = orgListInfo?.spaceIds ?? []
    if (orgSpaceIds.length === 0) {
      return []
    }
    const resourceNames = new Map(
      (resourcesList?.spacesList ?? []).flatMap((entry) => {
        const id = entry.entry?.ref?.providerResourceRef?.id
        return id ? ([[id, entry.spaceMeta?.name ?? id]] as const) : []
      }),
    )
    return orgSpaceIds.map((id) => ({
      id,
      displayName: resourceNames.get(id) ?? id,
      objectType: 'space',
    }))
  }, [orgListInfo?.spaceIds, orgState?.spaces, resourcesList?.spacesList])

  const handleCloseDetails = useCallback(() => {
    setOpenMenu?.('')
  }, [setOpenMenu])

  const handleBreadcrumbClick = useCallback(() => {
    navigate({ path: currentLevelPath })
  }, [navigate, currentLevelPath])

  const roleLabel = isOwner ? 'OWNER' : 'MEMBER'

  const orgButton = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        sx={sx}
        aria-label={
          selected ? 'Close organization menu' : 'Open organization menu'
        }
      >
        {selected ? (
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        ) : (
          <LuBuilding2 {...bottomBarIconProps} aria-hidden="true" />
        )}
        <div sx={stylexStyles.rule1}>{orgName}</div>
        {info && (
          <span
            sx={[
              stylexStyles.rule2,
              isOwner ? stylexStyles.rule3 : stylexStyles.rule4,
            ]}
          >
            {roleLabel}
          </span>
        )}
      </BottomBarItem>
    ),
    [orgName, roleLabel, isOwner, info],
  )

  const orgOverlay = useMemo(
    () => (
      <OrganizationDetails
        orgId={orgId}
        orgState={orgState}
        orgName={orgName}
        degraded={degraded}
        isOwner={isOwner}
        onCloseClick={handleCloseDetails}
      />
    ),
    [orgId, orgState, orgName, degraded, isOwner, handleCloseDetails],
  )

  const containerState = useMemo<OrgContainerState>(
    () => ({
      orgId,
      orgState,
      orgName,
      degraded,
      isOwner,
      spaces,
      billingAccountId,
    }),
    [orgId, orgState, orgName, degraded, isOwner, spaces, billingAccountId],
  )

  return (
    <OrgContainerContext.Provider value={containerState}>
      <BottomBarLevel
        id="organization"
        button={orgButton}
        overlay={orgOverlay}
        menuLabel={orgName}
        onBreadcrumbClick={handleBreadcrumbClick}
      >
        <Routes>
          <Route path="/so/:sharedObjectId/*">
            <SessionSharedObjectContainer />
          </Route>
          <Route path="*">
            <SessionFrame>
              <OrganizationDashboard />
            </SessionFrame>
          </Route>
        </Routes>
      </BottomBarLevel>
    </OrgContainerContext.Provider>
  )
}
