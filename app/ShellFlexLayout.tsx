/* eslint-disable react-doctor/no-giant-component */
import {
  type DragEvent as ReactDragEvent,
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
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
  ShellTab,
  getTabDisplayName,
  getTabNameFromPath,
} from '@s4wave/app/shell-tab.js'
import { useStateAtom } from '@s4wave/web/state/index.js'
import {
  TabContextProvider,
  type TabContextValue,
} from '@s4wave/web/object/TabContext.js'
import {
  ShellTabStateProvider,
  ShellTabsProvider,
  useShellTabs,
  type OpenShellTabOptions,
} from './ShellTabContext.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'
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
import { reconcileModelWithTabs } from './ShellGridLayout.js'
import { ShellTabLabel } from './ShellTabLabel.js'
import {
  encodeGridLayout,
  encodeGridLayoutStructure,
  getActiveTabsetId,
  getSelectedTabId,
  getTabIdsFromModel,
  hasGridLayout,
  decodeGridLayout,
  applyLocalStateToModel,
  SHELL_GRID_BASE_MODEL,
} from './shell-grid-utils.js'
import { buildShellExternalDrag } from './shell-app-drag.js'
import { openShellTabInNewTab } from './shell-popout.js'

function isTabNode(node: { getType(): string } | undefined): node is TabNode {
  return node?.getType() === 'tab'
}

// MENU_COLLAPSE_WIDTH is the top-left tabset width below which the menu bar
// collapses to the logo. Mirrors the page-width `narrow` breakpoint (640px),
// re-based onto the overlay's container instead of the viewport.
const MENU_COLLAPSE_WIDTH = 640

// findTopLeftStrip returns the top-left tab strip element in the shell layout,
// which is the container the menu-bar overlay sits over. Nested FlexLayouts
// inside tab content are excluded. Returns null when no strip is present yet.
function findTopLeftStrip(container: HTMLElement): HTMLElement | null {
  const strips = Array.from(
    container.querySelectorAll<HTMLElement>(
      '.flexlayout__tabset_tabbar_outer_top',
    ),
  ).filter((el) => !el.closest('.flexlayout__tab'))
  if (strips.length === 0) return null
  return strips.reduce((best, el) => {
    const a = el.getBoundingClientRect()
    const b = best.getBoundingClientRect()
    if (a.top < b.top - 2) return el
    if (a.top <= b.top + 2 && a.left < b.left) return el
    return best
  })
}

// noop stubs for TabContextValue in the shell overlay scope.
const noopAddTab = () => Promise.resolve({ tabId: '' })
const noopNavigateTab = () => Promise.resolve({})

// SHELL_TABS_STORAGE_KEY is the sessionStorage key for shell layout state.
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
      tabSetEnableDeleteWhenEmpty: false,
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

// loadModelFromStorage loads the FlexLayout model from sessionStorage.
// If the stored model is a grid layout (multiple tabsets), ignore it and rebuild.
function loadModelFromStorage(
  tabs: ShellTab[],
  activeTabId: string,
): IJsonModel {
  try {
    const stored = sessionStorage.getItem(SHELL_TABS_STORAGE_KEY)
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
              countTabsets(model.layout)
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

// saveModelToStorage saves the FlexLayout model to sessionStorage.
function saveModelToStorage(model: IJsonModel): void {
  try {
    sessionStorage.setItem(
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

export interface ShellTabStripProps {
  children?: React.ReactNode
  entry?: ShellDocumentEntry
}

// ShellTabStrip provides draggable tabs using FlexLayout.
// The FlexLayout spans the entire content area, enabling drag-to-split anywhere.
// When tabs are dragged to create splits, it transitions to grid mode via URL.
export function ShellTabStrip({ children, entry }: ShellTabStripProps) {
  return (
    <ShellTabsProvider entry={entry}>
      <ShellTabStripInner>{children}</ShellTabStripInner>
    </ShellTabsProvider>
  )
}

function ShellTabStripInner({
  children,
}: Pick<ShellTabStripProps, 'children'>) {
  const {
    tabs,
    activeTabId,
    addShellTab,
    selectShellTab,
    retainShellTabs,
    closeShellTab,
    updateTabPath,
    startRenaming,
    registerActiveTabsetPathOpener,
  } = useShellTabs()

  const [, setHasEngaged] = useStateAtom<boolean>(null, 'hasEngaged', false)
  const hasMarkedEngagedRef = useRef(false)
  const markShellEngaged = useCallback(() => {
    if (hasMarkedEngagedRef.current) return
    hasMarkedEngagedRef.current = true
    setHasEngaged(true)
  }, [setHasEngaged])

  // Ref to access latest tabs without causing re-renders.
  // Assigned directly (not in useEffect) to avoid one-frame stale reads.
  const tabsRef = useRef(tabs)
  // eslint-disable-next-line react-hooks/refs
  tabsRef.current = tabs

  // Check if we're currently in grid mode (URL starts with /g/)
  const isGridMode = useCallback(() => {
    return getAppPath().startsWith('/g/')
  }, [])
  const routePath = useSyncExternalStore(
    (onChange) => {
      window.addEventListener('hashchange', onChange)
      return () => window.removeEventListener('hashchange', onChange)
    },
    getAppPath,
    getAppPath,
  )

  // Initialize model from storage or default, and perform URL sync during
  // initialization. This avoids calling setState in the sync effect. A grid
  // deep link is decoded here so the first paint already shows the split, and
  // the decoded path travels with it: the hash effect must not decode the same
  // URL a second time and throw this model away.
  const [initial] = useState(() => {
    const path = getAppPath()
    const gridData = path.startsWith('/g/') ? path.slice(3) : ''
    const decoded = gridData
      ? decodeGridLayout(gridData, SHELL_GRID_BASE_MODEL)
      : null
    const jsonModel = decoded
      ? reconcileModelWithTabs(decoded.model, tabs)
      : loadModelFromStorage(tabs, activeTabId)
    const m = Model.fromJson(jsonModel)
    if (decoded) {
      applyLocalStateToModel(m, decoded.localState)
      // Grid mode keeps FlexLayout's own tabset behavior: panes are draggable
      // and an emptied one disappears. Only normal mode pins the layout to a
      // single fixed tabset.
      return {
        model: m,
        gridPath: path,
        structure: encodeGridLayoutStructure(m),
      }
    }
    m.doAction(
      Actions.updateModelAttributes({
        tabSetEnableDeleteWhenEmpty: false,
        tabSetEnableDrag: false,
        tabEnableDrag: false,
      }),
    )
    return { model: m, gridPath: null, structure: null }
  })
  const [model, setModel] = useState<Model>(initial.model)

  const gridPathRef = useRef<string | null>(initial.gridPath)
  const gridStructureRef = useRef<string | null>(initial.structure)
  const initializedRef = useRef(false)

  // A deep link or back/forward navigation can supply a new grid structure
  // without remounting this owner. Replace only the model; tab nodes retain
  // their stable IDs and OptimizedLayout remains mounted.
  useEffect(() => {
    const path = getAppPath()
    if (!path.startsWith('/g/')) {
      if (gridPathRef.current !== null) {
        const next = Model.fromJson(
          buildDefaultModel(tabsRef.current, activeTabId),
        )
        // Normal mode configures the layout as one fixed tabset. The default
        // model does not carry those attributes, so a rebuild that skipped
        // them would leave FlexLayout's draggable tabsets on in a mode that
        // has nowhere to drag them to.
        next.doAction(
          Actions.updateModelAttributes({
            tabSetEnableDeleteWhenEmpty: false,
            tabSetEnableDrag: false,
            tabEnableDrag: tabsRef.current.length >= 2,
          }),
        )
        gridPathRef.current = null
        gridStructureRef.current = null
        setModel(next)
      }
      return
    }
    if (gridPathRef.current === path) return
    const decoded = decodeGridLayout(path.slice(3), SHELL_GRID_BASE_MODEL)
    if (!decoded) {
      // A grid deep link that does not decode has no layout to show. Return
      // home rather than leaving the shell on a URL it cannot render.
      queueMicrotask(() => setAppPath('/'))
      return
    }
    const next = Model.fromJson(
      reconcileModelWithTabs(decoded.model, tabsRef.current),
    )
    applyLocalStateToModel(next, decoded.localState)
    gridPathRef.current = path
    gridStructureRef.current = encodeGridLayoutStructure(next)
    setModel(next)
  }, [activeTabId, routePath, tabs])
  const didSyncEntryRef = useRef(false)
  const lastSyncedActiveTabIdRef = useRef(activeTabId)
  const suppressedHashPathRef = useRef<string | null>(null)
  const pendingLocalHashIntentRef = useRef<{
    tabId: string
    path: string
  } | null>(null)
  const projectingTabsToModelRef = useRef(false)

  // openPathInFlexTabset opens a path as a shell tab for command handlers such
  // as the terminal opener. In a split the tab belongs to whichever tabset the
  // user is working in, and the route stays on the grid URL: writing the tab's
  // own path there would collapse the split the command was invoked from.
  const openPathInFlexTabset = useCallback(
    (path: string, options: OpenShellTabOptions = {}) => {
      const select = options.select ?? true
      const gridMode = isGridMode()
      const tabsetId = gridMode
        ? (getActiveTabsetId(model) ?? 'shell-tabset')
        : 'shell-tabset'
      const existingTab = options.focusExisting
        ? tabsRef.current.find(
            (tab) => tab.path === path && model.getNodeById(tab.id),
          )
        : undefined

      if (existingTab) {
        if (select) {
          selectShellTab(existingTab.id)
          model.doAction(Actions.selectTab(existingTab.id))
          if (!gridMode && existingTab.path !== getAppPath()) {
            setAppPath(existingTab.path)
          }
        }
        return existingTab.id
      }

      const newTab = buildPathTab(path)
      addShellTab(newTab, {
        afterTabId: options.afterTabId,
        select,
        onCommitted: () => {
          if (select) {
            addAndSelectShellModelTab(model, tabsetId, newTab, 'shell-content')
            if (!gridMode) setAppPath(path)
          } else {
            addShellModelTab(model, tabsetId, newTab, 'shell-content')
          }
        },
      })
      return newTab.id
    },
    [addShellTab, isGridMode, model, selectShellTab],
  )

  useEffect(
    () => registerActiveTabsetPathOpener(openPathInFlexTabset),
    [openPathInFlexTabset, registerActiveTabsetPathOpener],
  )

  // Keep the single-tabset model aligned with tab state, including state-only
  // tab additions from command handlers and cross-window storage hydration.
  useEffect(() => {
    projectingTabsToModelRef.current = true
    try {
      syncTabsStateToModel(model, tabs, activeTabId)
    } finally {
      projectingTabsToModelRef.current = false
    }
  }, [model, tabs, activeTabId])

  // Only enable tab dragging when there are at least 2 tabs (can't create splits with 1 tab)
  const canDrag = tabs.length >= 2
  useEffect(() => {
    if (model.toJson().global?.tabEnableDrag === canDrag) return
    model.doAction(Actions.updateModelAttributes({ tabEnableDrag: canDrag }))
  }, [model, canDrag])

  // Apply the provider's committed active record to the FlexLayout model.
  useEffect(() => {
    if (didSyncEntryRef.current || isGridMode() || tabs.length === 0) return
    didSyncEntryRef.current = true
    if (activeTabId) {
      model.doAction(Actions.selectTab(activeTabId))
    }
    initializedRef.current = true
    markShellEngaged()
    handleHashChange()
  }, [activeTabId, isGridMode, markShellEngaged, model, tabs])

  // Sync URL hash when active tab selection changes (after initialization).
  // Tab path changes are owned by the route/hash listeners and should not
  // drive the URL back to a stale tab snapshot during navigation.
  useEffect(() => {
    if (!initializedRef.current) return
    const pendingLocalPath = pendingLocalHashIntentRef.current
    if (pendingLocalPath?.tabId !== activeTabId) {
      pendingLocalHashIntentRef.current = null
    } else if (pendingLocalPath.path === getAppPath()) {
      return
    }
    // Don't sync URL in grid mode
    if (isGridMode()) return
    if (lastSyncedActiveTabIdRef.current === activeTabId) return
    lastSyncedActiveTabIdRef.current = activeTabId

    const activeTab = findShellTab(tabsRef.current, activeTabId)
    if (activeTab && activeTab.path !== getAppPath()) {
      setAppPath(activeTab.path)
    }
  }, [activeTabId, isGridMode])

  // A remote shared path update follows the active record into a normal Flex
  // URL. The hash listener ignores this feedback and does not write it back.
  useEffect(() => {
    if (!initializedRef.current || isGridMode()) return
    const activeTab = tabsRef.current.find((tab) => tab.id === activeTabId)
    const currentPath = getAppPath()
    const pendingLocalPath = pendingLocalHashIntentRef.current
    if (pendingLocalPath?.tabId !== activeTabId) {
      pendingLocalHashIntentRef.current = null
    } else if (pendingLocalPath.path === currentPath) {
      if (activeTab?.path === pendingLocalPath.path) {
        pendingLocalHashIntentRef.current = null
      } else {
        return
      }
    }
    if (!activeTab || activeTab.path === currentPath) return
    suppressedHashPathRef.current = activeTab.path
    setAppPath(activeTab.path)
  }, [activeTabId, isGridMode, tabs])

  // Listen for hash changes (back/forward navigation)
  const handleHashChange = useEffectEvent(() => {
    if (!initializedRef.current) return
    // Don't handle hash changes in grid mode
    if (isGridMode()) return

    const currentPath = getAppPath()
    if (suppressedHashPathRef.current === currentPath) {
      suppressedHashPathRef.current = null
      return
    }
    const activeTab = tabs.find((t) => t.id === activeTabId)
    if (!activeTab || activeTab.path === currentPath) return

    // Check if the node still exists in the model before updating
    const tabNode = model.getNodeById(activeTabId)
    if (!isTabNode(tabNode)) return

    // Update the current tab's path in tabs state (model doesn't store paths)
    const updated = {
      ...activeTab,
      path: currentPath,
      name: getTabNameFromPath(currentPath),
    }
    pendingLocalHashIntentRef.current = {
      tabId: activeTabId,
      path: currentPath,
    }
    void updateTabPath(activeTabId, currentPath).then((committed) => {
      const pendingLocalPath = pendingLocalHashIntentRef.current
      if (
        committed ||
        pendingLocalPath?.tabId !== activeTabId ||
        pendingLocalPath.path !== currentPath
      ) {
        return
      }
      pendingLocalHashIntentRef.current = null
      const committedTab = tabsRef.current.find((tab) => tab.id === activeTabId)
      if (!committedTab || committedTab.path === getAppPath()) return
      suppressedHashPathRef.current = committedTab.path
      setAppPath(committedTab.path)
    })
    // Update tab name in model outside the setTabs updater to avoid
    // triggering Layout.setState during an existing state transition.
    const displayName = getTabDisplayName(updated)
    if (tabNode.getName() !== displayName) {
      model.doAction(
        Actions.updateNodeAttributes(activeTabId, {
          name: displayName,
        }),
      )
    }
    markShellEngaged()
  })

  useEffect(() => {
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
  // Grid mode is not passed down: the content survives the mode transition, so
  // it reads the live mode from the shell context itself.
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

      // Save to sessionStorage.
      saveModelToStorage(newModel.toJson())

      // The provider owns state-to-model projection. FlexLayout reports each
      // synchronous intermediate action, so do not reconcile partial models
      // back into provider state during that directional projection.
      if (projectingTabsToModelRef.current) return

      // A fresh entry has no active ID until its record commits. FlexLayout's
      // default first selection is not authoritative during that window.
      if (!activeTabId) return

      // A grid route is valid only after every model tab has a committed
      // Shell Tab record. External drops add the model node before their
      // serialized store mutation commits.
      if (hasGridLayout(newModel)) {
        const committedTabIds = new Set(tabsRef.current.map((tab) => tab.id))
        if (
          !getTabIdsFromModel(newModel).every((id) => committedTabIds.has(id))
        ) {
          return
        }
        // Selection follows the user across panes. The tab owner drives the
        // document title, the command session, and the toolbar actions, so a
        // selection left behind here points all three at the wrong pane.
        const selectedId = getSelectedTabId(newModel)
        retainShellTabs(
          new Set(getTabIdsFromModel(newModel)),
          selectedId ?? undefined,
        )
        if (
          selectedId &&
          selectedId !== activeTabId &&
          tabsRef.current.some((tab) => tab.id === selectedId)
        ) {
          selectShellTab(selectedId)
          markShellEngaged()
        }
        // Only a structural change earns a URL write. Selecting a tab or
        // dragging a splitter also changes the encoded model, and publishing
        // those would make Back walk through every layout interaction before
        // it reaches the route the user came from.
        const structure = encodeGridLayoutStructure(newModel)
        if (structure !== gridStructureRef.current) {
          gridStructureRef.current = structure
          const gridPath = `/g/${encodeGridLayout(newModel)}`
          // The model already holds this structure, so record the path as the
          // one in effect: the hash listener would otherwise decode our own
          // write back into a replacement model.
          gridPathRef.current = gridPath
          setAppPath(gridPath)
        }
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
      const pendingLocalPath = pendingLocalHashIntentRef.current
      const suppressPendingPathFeedback =
        pendingLocalPath?.tabId === newActiveId &&
        pendingLocalPath.path === getAppPath()
      if (pendingLocalPath && pendingLocalPath.tabId !== newActiveId) {
        pendingLocalHashIntentRef.current = null
      }

      const modelTabIds = new Set(modelTabs.map((t) => t.id))
      retainShellTabs(modelTabIds, newActiveId ?? undefined)

      if (
        !suppressPendingPathFeedback &&
        newActiveId &&
        newActiveId !== activeTabId &&
        tabs.some((tab) => tab.id === newActiveId)
      ) {
        selectShellTab(newActiveId)
        // Update URL to match selected tab (only if not in grid mode)
        if (!isGridMode()) {
          // Get path from current tabs state
          const selectedTab = tabs.find((tab) => tab.id === newActiveId)
          if (selectedTab) {
            setAppPath(selectedTab.path)
          }
        }
        markShellEngaged()
      }
    },
    [
      selectShellTab,
      retainShellTabs,
      markShellEngaged,
      activeTabId,
      tabs,
      isGridMode,
    ],
  )

  // Custom icons for close button
  const icons = useMemo(
    () => ({
      close: <LuX className="size-2.5" />,
    }),
    [],
  )

  const appendAndSelectTab = useCallback(
    (tab: ShellTab, tabsetId = 'shell-tabset') => {
      addShellTab(tab, {
        select: true,
        onCommitted: () =>
          addAndSelectShellModelTab(model, tabsetId, tab, 'shell-content'),
      })
    },
    [model, addShellTab],
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
    openShellTabInNewTab(activeTab.path, activeTab.id)
  }, [tabs, activeTabId])

  // Explicit Shell close removes the shared record; local model pruning follows.
  const handleCloseTab = useCallback(() => {
    if (tabs.length <= 1) return
    closeShellTab(activeTabId)
  }, [tabs.length, activeTabId, closeShellTab])

  const handleCloseTabById = useCallback(
    (tabId: string) => {
      if (tabs.length <= 1) return
      closeShellTab(tabId)
    },
    [tabs.length, closeShellTab],
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

  const handleCloseOtherTabs = useCallback(
    (keepTabId: string) => {
      tabs.forEach((tab) => {
        if (tab.id !== keepTabId) closeShellTab(tab.id)
      })
    },
    [tabs, closeShellTab],
  )

  const handlePopoutTabById = useCallback(
    (tabId: string) => {
      const tab = findShellTab(tabs, tabId)
      if (!tab) return
      openShellTabInNewTab(tab.path, tab.id)
    },
    [tabs],
  )

  const handleExternalAppDrag = useCallback(
    (event: ReactDragEvent<HTMLElement>) =>
      buildShellExternalDrag(event, (draggedTabs, droppedNode) => {
        const [firstTab, ...remainingTabs] = draggedTabs
        if (!firstTab) return

        const droppedTabId = droppedNode?.getId()
        const droppedTab = droppedTabId ? model.getNodeById(droppedTabId) : null
        const tabsetId =
          droppedTabId && droppedTab
            ? (getShellTabsetId(model, droppedTabId) ?? 'shell-tabset')
            : 'shell-tabset'
        const activeTab =
          droppedTabId && droppedTab
            ? { ...firstTab, id: droppedTabId }
            : firstTab

        const commitFirst = () => {
          if (!droppedTab) {
            addAndSelectShellModelTab(
              model,
              tabsetId,
              activeTab,
              'shell-content',
            )
          }
          // A drop that created a split leaves the route on the grid URL.
          // Writing the dropped tab's own path there would collapse the split
          // the drop just made.
          if (!isGridMode()) setAppPath(activeTab.path)
          model.doAction(Actions.selectTab(activeTab.id))
          markShellEngaged()
        }
        addShellTab(activeTab, { select: true, onCommitted: commitFirst })

        for (const tab of remainingTabs) {
          addShellTab(tab, {
            onCommitted: () =>
              addShellModelTab(model, tabsetId, tab, 'shell-content'),
          })
        }
      }),
    [addShellTab, isGridMode, markShellEngaged, model],
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
          <LuX className="size-2.5" />
        </button>,
        <button
          key="add-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handleNewTab}
          title="New tab"
        >
          <LuPlus className="size-2.5" />
        </button>,
        <button
          key="popout-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handlePopoutTab}
          title="Open in new tab"
        >
          <LuExternalLink className="size-2.5" />
        </button>,
      )
    },
    [handleCloseTab, handleNewTab, handlePopoutTab, tabs.length],
  )

  // Ref for measuring menu bar width
  const menuBarRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Track the menu bar width (for the top-left strip's overlay clearance) and
  // the top-left tabset width (to collapse the menu when its container, not the
  // viewport, is too narrow). Re-found per model so splits re-target the
  // top-left strip; the observer catches splitter-drag and window resizes.
  useEffect(() => {
    const menuBar = menuBarRef.current
    const container = containerRef.current
    if (!menuBar || !container) return

    const topLeftStrip = findTopLeftStrip(container)

    const update = () => {
      container.style.setProperty(
        '--menu-bar-width',
        `${menuBar.offsetWidth}px`,
      )
      const width =
        topLeftStrip?.getBoundingClientRect().width ??
        container.getBoundingClientRect().width
      menuBar.dataset.menuCollapsed = String(width < MENU_COLLAPSE_WIDTH)
    }

    update()

    const observer = new ResizeObserver(update)
    observer.observe(menuBar)
    observer.observe(container)
    if (topLeftStrip) observer.observe(topLeftStrip)
    return () => observer.disconnect()
  }, [model])

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
