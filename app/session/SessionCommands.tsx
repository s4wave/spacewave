import { useCallback } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'

// SessionCommands registers session-scoped commands that need access
// to the Session context (space list, etc). Returns null (no UI).
export function SessionCommands() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigateSession = useSessionNavigate()
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const isTabActive = useIsTabActive()

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

  const subItems: SubItemsCallback = useCallback(
    (query: string) => {
      const spaces = resourcesList?.spacesList ?? []
      const q = query.toLowerCase()
      return Promise.resolve(
        spaces
          .filter((entry) => !!entry.entry?.ref?.providerResourceRef?.id)
          .map((entry) => ({
            id: entry.entry!.ref!.providerResourceRef!.id!,
            label: entry.spaceMeta?.name ?? 'Untitled',
          }))
          .filter((item) => !q || item.label.toLowerCase().includes(q)),
      )
    },
    [resourcesList],
  )

  useCommand({
    commandId: 'spacewave.session.lock',
    label: 'Lock Session',
    description: 'Lock the current session and return to session list',
    menuPath: 'File/Lock Session',
    menuGroup: 80,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      navigate({ path: '/sessions' })
    }, [navigate]),
  })

  useCommand({
    commandId: 'spacewave.session.switch',
    label: 'Switch Account',
    description: 'Switch to a different account',
    menuPath: 'File/Switch Account',
    menuGroup: 80,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      navigate({ path: '/sessions' })
    }, [navigate]),
  })

  useCommand({
    commandId: 'spacewave.session.settings',
    label: 'Account Settings',
    description: 'Open the account settings panel',
    menuPath: 'File/Account Settings',
    menuGroup: 80,
    menuOrder: 3,
    active: isTabActive,
    handler: useCallback(() => {
      setOpenMenu?.('account')
    }, [setOpenMenu]),
  })

  useCommand({
    commandId: 'spacewave.session.join-space',
    label: 'Join Space',
    description: 'Join a shared space via invite code or link',
    menuPath: 'File/Join Space',
    menuGroup: 70,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      navigateSession({ path: 'join' })
    }, [navigateSession]),
  })

  useCommand({
    commandId: 'spacewave.session.cli-setup',
    label: 'Command Line Setup',
    description: 'Connect the spacewave CLI to this session',
    menuPath: 'File/Command Line Setup',
    menuGroup: 80,
    menuOrder: 4,
    active: isTabActive,
    handler: useCallback(() => {
      navigateSession({ path: 'settings/cli' })
    }, [navigateSession]),
  })

  useCommand({
    commandId: 'spacewave.nav.home',
    label: 'Go Home',
    description: 'Navigate to the session dashboard',
    menuPath: 'View/Go Home',
    menuGroup: 30,
    menuOrder: 4,
    active: isTabActive,
    handler: useCallback(() => {
      navigateSession({ path: '' })
    }, [navigateSession]),
  })

  useCommand({
    commandId: 'spacewave.nav.go-to-space',
    label: 'Go to Space',
    description: 'Navigate to a space in the current session',
    menuPath: 'View/Go to Space',
    menuGroup: 30,
    menuOrder: 5,
    active: isTabActive,
    hasSubItems: true,
    subItems,
    handler: useCallback(
      (args: Record<string, string>) => {
        const spaceId = args.subItemId
        if (spaceId) {
          navigateSession({ path: `so/${spaceId}` })
          return
        }
        navigateSession({ path: '' })
      },
      [navigateSession],
    ),
  })
  return null
}
