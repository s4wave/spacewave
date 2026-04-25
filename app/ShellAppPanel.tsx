import { useCallback, useMemo } from 'react'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { setAppPath } from '@s4wave/web/router/app-path.js'
import { HistoryRouter } from '@s4wave/web/router/HistoryRouter.js'
import { resolvePath, type To } from '@s4wave/web/router/router.js'
import {
  StateNamespaceProvider,
  useStateAtom,
} from '@s4wave/web/state/index.js'
import {
  TabContextProvider,
  type TabContextValue,
} from '@s4wave/web/object/TabContext.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import type { AddTabRequest } from '@s4wave/sdk/layout/layout.pb.js'

import { AppRoutes } from './routes/AppRoutes.js'
import {
  addTab as addShellTab,
  ShellTabStateProvider,
  useShellTabs,
  useTabId,
} from './ShellTabContext.js'

// ShellAppPanelProps are the props for ShellAppPanel.
export interface ShellAppPanelProps {
  tabId: string
  initialPath: string
  namespace: string[]
  syncAppPath?: boolean
}

// ShellAppPanel renders the shared app surface for a shell tab or grid panel.
export function ShellAppPanel({
  tabId,
  initialPath,
  namespace,
  syncAppPath = false,
}: ShellAppPanelProps) {
  return (
    <ShellTabStateProvider tabId={tabId}>
      <StateNamespaceProvider namespace={namespace}>
        <ShellAppPanelInner
          initialPath={initialPath}
          syncAppPath={syncAppPath}
        />
      </StateNamespaceProvider>
    </ShellTabStateProvider>
  )
}

// ShellAppPanelInner provides tab context, bottom bar, and routing for a shell panel.
function ShellAppPanelInner({
  initialPath,
  syncAppPath,
}: {
  initialPath: string
  syncAppPath: boolean
}) {
  const [openMenu, setOpenMenu] = useStateAtom<string>(null, 'openMenu', '')
  const tabId = useTabId()
  const { tabs, setTabs, setActiveTabId, updateTabPath } = useShellTabs()

  const addTab = useCallback(
    (request: AddTabRequest) => {
      const tab = request.tab
      if (!tab) return Promise.resolve({ tabId: '' })

      let path = '/'
      if (tab.data && tab.data.length > 0) {
        const layoutTab = ObjectLayoutTab.fromBinary(tab.data)
        path = layoutTab.path || '/'
      }

      const result = addShellTab(tabs, path, tabId ?? undefined)
      setTabs(result.tabs)
      if (request.select) {
        setActiveTabId(result.newTab.id)
      }
      return Promise.resolve({ tabId: result.newTab.id })
    },
    [tabs, tabId, setTabs, setActiveTabId],
  )

  const navigateTab = useCallback(
    (path: string) => {
      if (tabId) updateTabPath(tabId, path)
      return Promise.resolve({})
    },
    [tabId, updateTabPath],
  )

  const tabContext = useMemo<TabContextValue>(
    () => ({ tabId: tabId ?? '', addTab, navigateTab }),
    [tabId, addTab, navigateTab],
  )

  const path = useMemo(() => {
    if (!tabId) return initialPath
    const tab = tabs.find((t) => t.id === tabId)
    return tab?.path ?? initialPath
  }, [tabs, tabId, initialPath])

  const handleNavigate = useCallback(
    (to: To) => {
      if (!tabId) return
      const newPath = resolvePath(path, to)
      updateTabPath(tabId, newPath)
      if (syncAppPath) setAppPath(newPath)
    },
    [tabId, path, updateTabPath, syncAppPath],
  )

  return (
    <TabContextProvider value={tabContext}>
      <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
        <div className="flex h-full flex-1 flex-col overflow-hidden">
          <HistoryRouter path={path} onNavigate={handleNavigate}>
            <AppRoutes />
          </HistoryRouter>
        </div>
      </BottomBarRoot>
    </TabContextProvider>
  )
}
