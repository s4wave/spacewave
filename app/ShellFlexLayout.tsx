import {
  type DragEvent as ReactDragEvent,
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react'
import {
  OptimizedLayout,
  Actions,
  BorderNode,
  ITabRenderValues,
  ITabSetRenderValues,
  IJsonModel,
  Model,
  TabNode,
  TabSetNode,
} from '@aptre/flex-layout'
import { LuExternalLink, LuPlus, LuX } from 'react-icons/lu'

import { BASE_MODEL } from '@s4wave/web/layout/layout.js'
import { getAppPath, setAppPath } from '@s4wave/web/router/app-path.js'
import {
  DEFAULT_HOME_TAB,
  ShellTab,
  generateTabId,
  getTabDisplayName,
  getTabNameFromPath,
} from '@s4wave/app/shell-tab.js'
import { useStateAtom } from '@s4wave/web/state/index.js'
import {
  TabContextProvider,
  type TabContextValue,
} from '@s4wave/web/object/TabContext.js'
import {
  cleanupOrphanedTabStorage,
  ShellTabStateProvider,
  ShellTabsProvider,
  useShellTabs,
} from './ShellTabContext.js'
import {
  addAndSelectShellModelTab,
  addShellModelTab,
  buildContextualShellTab,
  buildPathTab,
  cloneShellTab,
  countShellModelTabs,
  findShellTab,
  getShellTabsetId,
} from './shell-layout-tab-utils.js'
import {
  ShellTabContextMenu,
  type ShellTabContextMenuState,
} from './ShellTabContextMenu.js'
import { ShellTabContent } from './ShellTabContent.js'
import { ShellTabLabel } from './ShellTabLabel.js'
import { hasGridLayout, encodeGridLayout } from './shell-grid-utils.js'
import { buildShellExternalDrag } from './shell-app-drag.js'
import { openShellTabInNewTab } from './shell-popout.js'

// noop stubs for TabContextValue in the shell overlay scope.
const noopAddTab = () => Promise.resolve({ tabId: '' })
const noopNavigateTab = () => Promise.resolve({})

// SHELL_TABS_STORAGE_KEY is the localStorage key for shell layout state.
const SHELL_TABS_STORAGE_KEY = 'shell-tabs-layout'
const SHELL_TABS_NONCE = 4

// buildDefaultModel creates a FlexLayout model for the shell tabs.
// Note: Tab paths are NOT stored in the model config - they come from tabs state only.
function buildDefaultModel(tabs: ShellTab[], activeTabId: string): IJsonModel {
  return {
    ...BASE_MODEL,
    global: {
      ...BASE_MODEL.global,
      tabEnableClose: false,
      tabSetEnableMaximize: false,
      tabSetEnableDivide: true,
      tabSetEnableDeleteWhenEmpty: true,
      splitterSize: 4,
      splitterExtra: 4,
      enableEdgeDock: true,
    },
    layout: {
      type: 'row',
      weight: 100,
      children: [
        {
          type: 'tabset',
          id: 'shell-tabset',
          weight: 100,
          selected: Math.max(
            0,
            tabs.findIndex((t) => t.id === activeTabId),
          ),
          children: tabs.map((tab) => ({
            type: 'tab',
            id: tab.id,
            name: getTabDisplayName(tab),
            component: 'shell-content',
          })),
        },
      ],
    },
  }
}

// loadModelFromStorage loads the FlexLayout model from localStorage.
// If the stored model is a grid layout (multiple tabsets), ignore it and rebuild.
function loadModelFromStorage(
  tabs: ShellTab[],
  activeTabId: string,
): IJsonModel {
  try {
    const stored = localStorage.getItem(SHELL_TABS_STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as unknown
      if (typeof parsed === 'object' && parsed !== null) {
        const parsedObj = parsed as Record<string, unknown>
        if (Number(parsedObj.nonce) === SHELL_TABS_NONCE) {
          // Check if stored model has grid layout - if so, don't use it
          // ShellTabStrip is for single-tabset mode only
          const model = parsedObj.model as IJsonModel | undefined
          if (model) {
            let tabsetCount = 0
            const countTabsets = (node: {
              type?: string
              children?: unknown[]
            }) => {
              if (node.type === 'tabset') tabsetCount++
              if (node.children && Array.isArray(node.children)) {
                for (const child of node.children) {
                  countTabsets(child as { type?: string; children?: unknown[] })
                }
              }
            }
            if (model.layout) {
              countTabsets(
                model.layout as { type?: string; children?: unknown[] },
              )
            }
            if (tabsetCount <= 1) {
              return model
            }
          }
        }
      }
    }
  } catch {
    // Ignore parse errors
  }
  return buildDefaultModel(tabs, activeTabId)
}

// saveModelToStorage saves the FlexLayout model to localStorage.
function saveModelToStorage(model: IJsonModel): void {
  try {
    localStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({ nonce: SHELL_TABS_NONCE, model }),
    )
  } catch {
    // Ignore storage errors
  }
}

// syncTabsStateToModel keeps the single-tabset FlexLayout model aligned with
// the shell tab state, including state-only tab additions and selections.
function syncTabsStateToModel(
  model: Model,
  tabs: ShellTab[],
  activeTabId: string,
): void {
  const modelTabIds = new Set<string>()
  let selectedTabId: string | null = null

  model.visitNodes((node) => {
    if (node.getType() === 'tab') {
      modelTabIds.add(node.getId())
    }
    if (node.getType() === 'tabset') {
      const tabset = node as TabSetNode
      const selected = tabset.getSelectedNode()
      if (selected) {
        selectedTabId = selected.getId()
      }
    }
  })

  const tabIds = new Set(tabs.map((t) => t.id))

  for (const tab of tabs) {
    if (!modelTabIds.has(tab.id)) {
      addShellModelTab(model, 'shell-tabset', tab, 'shell-content')
    }
    const node = model.getNodeById(tab.id)
    if (node && node.getType() === 'tab') {
      const tabNode = node as TabNode
      const displayName = getTabDisplayName(tab)
      if (tabNode.getName() !== displayName) {
        model.doAction(
          Actions.updateNodeAttributes(tab.id, { name: displayName }),
        )
      }
    }
  }

  for (const tabId of modelTabIds) {
    if (!tabIds.has(tabId)) {
      model.doAction(Actions.deleteTab(tabId))
    }
  }

  if (activeTabId !== selectedTabId && model.getNodeById(activeTabId)) {
    model.doAction(Actions.selectTab(activeTabId))
  }
}

// ShellTabStripProps are the props for ShellTabStrip.
export interface ShellTabStripProps {
  children?: React.ReactNode
}

// ShellTabStrip provides draggable tabs using FlexLayout.
// The FlexLayout spans the entire content area, enabling drag-to-split anywhere.
// When tabs are dragged to create splits, it transitions to grid mode via URL.
export function ShellTabStrip({ children }: ShellTabStripProps) {
  return (
    <ShellTabsProvider>
      <ShellTabStripInner>{children}</ShellTabStripInner>
    </ShellTabsProvider>
  )
}

// ShellTabStripInner is the inner component that uses the tabs context.
function ShellTabStripInner({ children }: ShellTabStripProps) {
  const { tabs, setTabs, activeTabId, setActiveTabId, startRenaming } =
    useShellTabs()

  const [, setHasEngaged] = useStateAtom<boolean>(null, 'hasEngaged', false)

  // Ref to access latest tabs without causing re-renders.
  // Assigned directly (not in useEffect) to avoid one-frame stale reads.
  const tabsRef = useRef(tabs)
  // eslint-disable-next-line react-hooks/refs
  tabsRef.current = tabs

  // Track previous tab IDs for cleanup
  const prevTabIdsRef = useRef<Set<string>>(new Set([DEFAULT_HOME_TAB.id]))

  // Cleanup orphaned tab storage when tabs are removed
  useEffect(() => {
    const currentIds = new Set(tabs.map((t) => t.id))
    const prevIds = prevTabIdsRef.current

    // Check if any tabs were removed
    const removed = [...prevIds].filter((id) => !currentIds.has(id))
    if (removed.length > 0) {
      cleanupOrphanedTabStorage([...currentIds])
    }

    prevTabIdsRef.current = currentIds
  }, [tabs])

  // Check if we're currently in grid mode (URL starts with /g/)
  const isGridMode = useCallback(() => {
    return getAppPath().startsWith('/g/')
  }, [])

  // Initialize model from storage or default, and perform URL sync during initialization
  // This avoids calling setState in the sync effect
  const [model, setModel] = useState<Model>(() => {
    const jsonModel = loadModelFromStorage(tabs, activeTabId)
    const m = Model.fromJson(jsonModel)
    // Disable tabset dragging in non-grid mode, and tab dragging initially
    m.doAction(
      Actions.updateModelAttributes({
        tabSetEnableDrag: false,
        tabEnableDrag: false,
      }),
    )
    return m
  })

  // Track if initial URL sync has happened - use ref instead of state
  // to avoid setState in effect body
  const initializedRef = useRef(false)

  // Only enable tab dragging when there are at least 2 tabs (can't create splits with 1 tab)
  const canDrag = tabs.length >= 2
  useEffect(() => {
    model.doAction(Actions.updateModelAttributes({ tabEnableDrag: canDrag }))
  }, [model, canDrag])

  // Keep the single-tabset model aligned with tab state, including state-only
  // tab additions from command handlers and cross-window storage hydration.
  useEffect(() => {
    syncTabsStateToModel(model, tabs, activeTabId)
  }, [model, tabs, activeTabId])

  // Sync with URL hash on mount
  useEffect(() => {
    if (initializedRef.current) return
    initializedRef.current = true

    // Don't sync in grid mode - grid mode handles its own routing
    if (isGridMode()) {
      return
    }

    const currentPath = getAppPath()

    const existingTab = tabs.find((t) => t.path === currentPath)
    if (existingTab) {
      setActiveTabId(existingTab.id)
      model.doAction(Actions.selectTab(existingTab.id))
    } else if (currentPath !== '/') {
      // Create new tab for non-home paths
      const newTab = buildPathTab(currentPath)
      setTabs((prev) => [...prev, newTab])
      setActiveTabId(newTab.id)
      addAndSelectShellModelTab(model, 'shell-tabset', newTab, 'shell-content')
    }

    setHasEngaged(true)
  }, [model, tabs, setTabs, setActiveTabId, setHasEngaged, isGridMode])

  // Sync URL hash when active tab changes (after initialization)
  useEffect(() => {
    if (!initializedRef.current) return
    // Don't sync URL in grid mode
    if (isGridMode()) return

    const activeTab = tabs.find((t) => t.id === activeTabId)
    if (activeTab && activeTab.path !== getAppPath()) {
      setAppPath(activeTab.path)
    }
  }, [activeTabId, tabs, isGridMode])

  // Listen for hash changes (back/forward navigation)
  const handleHashChange = useEffectEvent(() => {
    // Don't handle hash changes in grid mode
    if (isGridMode()) return

    const currentPath = getAppPath()
    const activeTab = tabs.find((t) => t.id === activeTabId)
    if (!activeTab || activeTab.path === currentPath) return

    // Check if the node still exists in the model before updating
    if (!model.getNodeById(activeTabId)) return

    // Update the current tab's path in tabs state (model doesn't store paths)
    const updated = {
      ...activeTab,
      path: currentPath,
      name: getTabNameFromPath(currentPath),
    }
    setTabs((prevTabs) =>
      prevTabs.map((t) => (t.id !== activeTabId ? t : updated)),
    )
    // Update tab name in model outside the setTabs updater to avoid
    // triggering Layout.setState during an existing state transition.
    model.doAction(
      Actions.updateNodeAttributes(activeTabId, {
        name: getTabDisplayName(updated),
      }),
    )
    setHasEngaged(true)
  })

  useEffect(() => {
    if (!initializedRef.current) return

    const onHashChange = () => {
      handleHashChange()
    }
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  // onRenderTab customizes the tab button label with inline rename support.
  // Uses display name (custom or auto-derived) from tabs state.
  const onRenderTab = useCallback(
    (node: TabNode, renderValues: ITabRenderValues) => {
      const tabId = node.getId()
      const tab = findShellTab(tabsRef.current, tabId)
      if (tab) {
        renderValues.content = <ShellTabLabel tab={tab} />
      }
    },
    [],
  )

  // renderTab function - renders content for each tab
  // Path comes from tabs state via ref (single source of truth)
  // Using ref ensures stable callback identity to prevent FlexLayout re-renders
  const renderTab = useCallback((node: TabNode) => {
    const tabId = node.getId()
    const tab = findShellTab(tabsRef.current, tabId)
    const path = tab?.path ?? '/'
    return <ShellTabContent tabId={tabId} path={path} />
  }, [])

  // Handle model changes - sync tabs state, check for grid mode transition
  const handleModelChange = useCallback(
    (newModel: Model) => {
      setModel(newModel)

      // Save to localStorage
      saveModelToStorage(newModel.toJson())

      // Check for transition to grid mode (splits detected)
      if (hasGridLayout(newModel)) {
        const layoutData = encodeGridLayout(newModel)
        // Navigate to grid mode URL
        setAppPath(`/g/${layoutData}`)
        return
      }

      // Extract tab IDs and names from model (paths come from tabs state)
      const modelTabs: { id: string; name: string }[] = []
      let newActiveId: string | null = null

      newModel.visitNodes((node) => {
        if (node.getType() === 'tab') {
          const tabNode = node as TabNode
          modelTabs.push({
            id: tabNode.getId(),
            name: tabNode.getName(),
          })
        }
        if (node.getType() === 'tabset') {
          const tabset = node as TabSetNode
          const selected = tabset.getSelectedNode()
          if (selected) {
            newActiveId = selected.getId()
          }
        }
      })

      // Ensure at least one tab exists
      if (modelTabs.length === 0) {
        const homeTab = { ...DEFAULT_HOME_TAB, id: generateTabId() }
        addAndSelectShellModelTab(
          newModel,
          'shell-tabset',
          homeTab,
          'shell-content',
        )
        setTabs([homeTab])
        setActiveTabId(homeTab.id)
        setAppPath(homeTab.path)
        return
      }

      // Update tabs state: keep existing paths, remove deleted tabs, update names
      setTabs((prevTabs) => {
        const modelTabIds = new Set(modelTabs.map((t) => t.id))
        const modelTabNames = new Map(modelTabs.map((t) => [t.id, t.name]))

        // Keep tabs that still exist in model, update names
        const updatedTabs = prevTabs
          .filter((t) => modelTabIds.has(t.id))
          .map((t) => ({
            ...t,
            name: modelTabNames.get(t.id) ?? t.name,
          }))

        // Add any new tabs from model (shouldn't happen often, but handle it)
        for (const mt of modelTabs) {
          if (!prevTabs.some((t) => t.id === mt.id)) {
            updatedTabs.push({
              id: mt.id,
              name: mt.name,
              path: '/',
            })
          }
        }

        return updatedTabs
      })

      if (newActiveId && newActiveId !== activeTabId) {
        setActiveTabId(newActiveId)
        // Update URL to match selected tab (only if not in grid mode)
        if (!isGridMode()) {
          // Get path from current tabs state
          const selectedTab = tabs.find((t) => t.id === newActiveId)
          if (selectedTab) {
            setAppPath(selectedTab.path)
          }
        }
        setHasEngaged(true)
      }
    },
    [setTabs, setActiveTabId, setHasEngaged, activeTabId, tabs, isGridMode],
  )

  // Custom icons for close button
  const icons = useMemo(
    () => ({
      close: <LuX className="h-2.5 w-2.5" />,
    }),
    [],
  )

  const appendAndSelectTab = useCallback(
    (tab: ShellTab, tabsetId = 'shell-tabset') => {
      setTabs((prev) => [...prev, tab])
      addAndSelectShellModelTab(model, tabsetId, tab, 'shell-content')
    },
    [model, setTabs],
  )

  const handleNewTabAtTab = useCallback(
    (tabId: string) => {
      const sourceTab = findShellTab(tabs, tabId)
      const tabsetId = getShellTabsetId(model, tabId) ?? 'shell-tabset'
      appendAndSelectTab(buildContextualShellTab(sourceTab?.path), tabsetId)
    },
    [tabs, model, appendAndSelectTab],
  )

  // Handle creating a new blank tab.
  // If the current tab is in a session (/u/{idx}/...), opens to that session's dashboard.
  // Otherwise opens to home.
  const handleNewTab = useCallback(() => {
    handleNewTabAtTab(activeTabId)
  }, [activeTabId, handleNewTabAtTab])

  // Handle popping out current tab to a new browser tab
  const handlePopoutTab = useCallback(() => {
    const activeTab = findShellTab(tabs, activeTabId)
    if (!activeTab) return
    openShellTabInNewTab(activeTab.path)
  }, [tabs, activeTabId])

  // Handle closing the active tab
  const handleCloseTab = useCallback(() => {
    if (tabs.length <= 1) return
    model.doAction(Actions.deleteTab(activeTabId))
  }, [model, tabs.length, activeTabId])

  // Handle closing a specific tab by ID
  const handleCloseTabById = useCallback(
    (tabId: string) => {
      if (tabs.length <= 1) return
      model.doAction(Actions.deleteTab(tabId))
    },
    [model, tabs.length],
  )

  // Handle duplicating a specific tab by ID
  const handleDuplicateTab = useCallback(
    (tabId: string) => {
      const tab = findShellTab(tabs, tabId)
      if (!tab) return

      const tabsetId = getShellTabsetId(model, tabId) ?? 'shell-tabset'
      appendAndSelectTab(cloneShellTab(tab), tabsetId)
    },
    [tabs, model, appendAndSelectTab],
  )

  // Handle popping out a specific tab to a new tab
  const handlePopoutTabById = useCallback(
    (tabId: string) => {
      const tab = findShellTab(tabs, tabId)
      if (!tab) return
      openShellTabInNewTab(tab.path)
    },
    [tabs],
  )

  // Handle closing all other tabs
  const handleCloseOtherTabs = useCallback(
    (keepTabId: string) => {
      tabs.forEach((tab) => {
        if (tab.id !== keepTabId) {
          model.doAction(Actions.deleteTab(tab.id))
        }
      })
    },
    [model, tabs],
  )

  const handleExternalAppDrag = useCallback(
    (event: ReactDragEvent<HTMLElement>) =>
      buildShellExternalDrag(event, (tab) => {
        setTabs((prev) => {
          const existingIdx = prev.findIndex((t) => t.id === tab.id)
          if (existingIdx < 0) {
            return [...prev, tab]
          }
          return prev.map((t) => (t.id === tab.id ? tab : t))
        })
        setActiveTabId(tab.id)
        setAppPath(tab.path)
        setHasEngaged(true)
      }),
    [setActiveTabId, setHasEngaged, setTabs],
  )

  const [contextMenu, setContextMenu] =
    useState<ShellTabContextMenuState | null>(null)

  // Handle right-click on tab via FlexLayout's onContextMenu
  const handleContextMenu = useCallback(
    (node: TabNode | TabSetNode | BorderNode, event: React.MouseEvent) => {
      if (node.getType() !== 'tab') return
      event.preventDefault()
      setContextMenu({
        tabId: node.getId(),
        x: event.clientX,
        y: event.clientY,
      })
    },
    [],
  )

  // Render tabset toolbar with add button
  const onRenderTabSet = useCallback(
    (node: TabSetNode | BorderNode, renderValues: ITabSetRenderValues) => {
      if (node.getType() !== 'tabset') return
      renderValues.stickyButtons.push(
        <button
          key="close-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handleCloseTab}
          title="Close tab"
          disabled={tabs.length <= 1}
        >
          <LuX className="h-2.5 w-2.5" />
        </button>,
        <button
          key="add-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handleNewTab}
          title="New tab"
        >
          <LuPlus className="h-2.5 w-2.5" />
        </button>,
        <button
          key="popout-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handlePopoutTab}
          title="Open in new tab"
        >
          <LuExternalLink className="h-2.5 w-2.5" />
        </button>,
      )
    },
    [handleCloseTab, handleNewTab, handlePopoutTab, tabs.length],
  )

  // Ref for measuring menu bar width
  const menuBarRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Track menu bar width and update CSS variable
  useEffect(() => {
    const menuBar = menuBarRef.current
    const container = containerRef.current
    if (!menuBar || !container) return

    const updateWidth = () => {
      const width = menuBar.offsetWidth
      container.style.setProperty('--menu-bar-width', `${width}px`)
    }

    updateWidth()

    const observer = new ResizeObserver(updateWidth)
    observer.observe(menuBar)
    return () => observer.disconnect()
  }, [])

  // Provide TabContext for command components in the shell overlay.
  const overlayTabContext = useMemo<TabContextValue>(
    () => ({
      tabId: activeTabId,
      addTab: noopAddTab,
      navigateTab: noopNavigateTab,
    }),
    [activeTabId],
  )

  return (
    <ShellTabStateProvider tabId={activeTabId}>
      <TabContextProvider value={overlayTabContext}>
        <div
          ref={containerRef}
          className="shell-flexlayout shell-flexlayout--with-menu flex flex-1 flex-col overflow-hidden"
        >
          <div ref={menuBarRef} className="shell-menu-bar-overlay">
            {children}
          </div>
          <OptimizedLayout
            model={model}
            renderTab={renderTab}
            onModelChange={handleModelChange}
            onContextMenu={handleContextMenu}
            onExternalDrag={handleExternalAppDrag}
            onRenderTab={onRenderTab}
            onRenderTabSet={onRenderTabSet}
            icons={icons}
          />
          <ShellTabContextMenu
            state={contextMenu}
            canCloseTabs={countShellModelTabs(model) > 1}
            onClose={() => setContextMenu(null)}
            onNewTab={handleNewTabAtTab}
            onRenameTab={startRenaming}
            onDuplicateTab={handleDuplicateTab}
            onPopoutTab={handlePopoutTabById}
            onCloseOtherTabs={handleCloseOtherTabs}
            onCloseTab={handleCloseTabById}
          />
        </div>
      </TabContextProvider>
    </ShellTabStateProvider>
  )
}

// SHELL_TAB_STRIP_CONTAINER_ID is kept for backwards compatibility.
export const SHELL_TAB_STRIP_CONTAINER_ID = 'shell-tab-strip-container'
