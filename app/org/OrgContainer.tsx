import { createContext, useCallback, useContext, useMemo } from 'react'
import { LuArrowUp, LuBuilding2 } from 'react-icons/lu'
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
import { cn } from '@s4wave/web/style/utils.js'

import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { SessionSharedObjectContainer } from '@s4wave/app/session/SessionSharedObjectContainer.js'

import { ORG_ROLE_OWNER } from './org-constants.js'
import { OrganizationDashboard } from './OrganizationDashboard.js'
import { OrganizationDetails } from './OrganizationDetails.js'

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
  const ctx = useContext(OrgContainerContext)
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
      (resourcesList?.spacesList ?? [])
        .map((entry) => {
          const id = entry.entry?.ref?.providerResourceRef?.id
          if (!id) {
            return null
          }
          return [id, entry.spaceMeta?.name ?? id] as const
        })
        .filter((entry): entry is readonly [string, string] => !!entry),
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
  const roleBadgeClass =
    isOwner ?
      'bg-brand/15 text-brand'
    : 'bg-foreground/10 text-foreground-alt/70'

  const orgButton = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        className={className}
        aria-label={
          selected ? 'Close organization menu' : 'Open organization menu'
        }
      >
        {selected ?
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        : <LuBuilding2 {...bottomBarIconProps} aria-hidden="true" />}
        <div className="max-w-36 truncate">{orgName}</div>
        {info && (
          <span
            className={cn(
              'ml-1.5 rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase',
              roleBadgeClass,
            )}
          >
            {roleLabel}
          </span>
        )}
      </BottomBarItem>
    ),
    [orgName, roleLabel, roleBadgeClass, info],
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
    [orgId, orgState, isOwner, handleCloseDetails],
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

  const buttonKey = `${orgName}|${roleLabel}`

  return (
    <OrgContainerContext.Provider value={containerState}>
      <BottomBarLevel
        id="organization"
        button={orgButton}
        overlay={orgOverlay}
        buttonKey={buttonKey}
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
