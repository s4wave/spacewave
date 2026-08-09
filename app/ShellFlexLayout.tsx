import {
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
  IJsonModel,
  Model,
  TabNode,
  TabSetNode,
} from '@aptre/flex-layout'
import { LuX } from 'react-icons/lu'

import { BASE_MODEL } from '@s4wave/web/layout/layout.js'
import {
  getAppPath,
  setAppPath,
  subscribeAppPath,
} from '@s4wave/web/router/app-path.js'
import { useAppPath } from '@s4wave/web/router/useAppPath.js'
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
  buildPathTab,
  countShellModelTabs,
  findShellTab,
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
import { useShellExternalAppDrag } from './useShellExternalAppDrag.js'
import { useShellMenuMeasurement } from './useShellMenuMeasurement.js'
import { useShellTabActions } from './useShellTabActions.js'
import { useShellTabToolbar } from './useShellTabToolbar.js'

function isTabNode(node: { getType(): string } | undefined): node is TabNode {
  return node?.getType() === 'tab'
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

function useShellTabStripController(children: ShellTabStripProps['children']) {
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

  const isGridMode = useCallback(() => {
    return getAppPath().startsWith('/g/')
  }, [])
  const routePath = useAppPath()

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
  // without remounting ShellTabStripInner. Replace only the model; tab nodes
  // retain their stable IDs and OptimizedLayout remains mounted.
  useEffect(() => {
    const path = getAppPath()
    if (!path.startsWith('/g/')) {
      if (gridPathRef.current !== null) {
        const next = Model.fromJson(buildDefaultModel(tabs, activeTabId))
        // Normal mode configures the layout as one fixed tabset. The default
        // model does not carry those attributes, so a rebuild that skipped
        // them would leave FlexLayout's draggable tabsets on in a mode that
        // has nowhere to drag them to.
        next.doAction(
          Actions.updateModelAttributes({
            tabSetEnableDeleteWhenEmpty: false,
            tabSetEnableDrag: false,
            tabEnableDrag: tabs.length >= 2,
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
    const next = Model.fromJson(reconcileModelWithTabs(decoded.model, tabs))
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
        ? tabs.find((tab) => tab.path === path && model.getNodeById(tab.id))
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
    [addShellTab, isGridMode, model, selectShellTab, tabs],
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
    if (isGridMode()) return
    if (lastSyncedActiveTabIdRef.current === activeTabId) return
    lastSyncedActiveTabIdRef.current = activeTabId

    const activeTab = findShellTab(tabs, activeTabId)
    if (activeTab && activeTab.path !== getAppPath()) {
      setAppPath(activeTab.path)
    }
  }, [activeTabId, isGridMode, tabs])

  const handleHashChange = useEffectEvent(() => {
    if (!initializedRef.current) return
    if (isGridMode()) return

    const currentPath = getAppPath()
    if (suppressedHashPathRef.current === currentPath) {
      suppressedHashPathRef.current = null
      return
    }
    const activeTab = tabs.find((t) => t.id === activeTabId)
    if (!activeTab || activeTab.path === currentPath) return
    const pendingLocalPath = pendingLocalHashIntentRef.current
    if (
      pendingLocalPath?.tabId === activeTabId &&
      pendingLocalPath.path === currentPath
    ) {
      return
    }

    const tabNode = model.getNodeById(activeTabId)
    if (!isTabNode(tabNode)) return

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
      const committedTab = tabs.find((tab) => tab.id === activeTabId)
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

  useEffect(() => subscribeAppPath(handleHashChange), [])

  // onRenderTab customizes the tab button label with inline rename support.
  // Uses display name (custom or auto-derived) from tabs state.
  const onRenderTab = useCallback(
    (node: TabNode, renderValues: ITabRenderValues) => {
      const tabId = node.getId()
      const tab = findShellTab(tabs, tabId)
      if (tab) {
        renderValues.content = <ShellTabLabel tab={tab} />
      }
    },
    [tabs],
  )

  // Grid mode is not passed down: the content survives the mode transition, so
  // it reads the live mode from the shell context itself.
  const renderTab = useCallback(
    (node: TabNode) => {
      const tabId = node.getId()
      const tab = findShellTab(tabs, tabId)
      const path = tab?.path ?? '/'
      return <ShellTabContent tabId={tabId} path={path} />
    },
    [tabs],
  )

  const handleModelChange = useCallback(
    (newModel: Model) => {
      setModel(newModel)

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
        const committedTabIds = new Set(tabs.map((tab) => tab.id))
        if (
          !getTabIdsFromModel(newModel).every((id) => committedTabIds.has(id))
        ) {
          return
        }
        // Selection follows the user across panes. The selected tab drives the
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
          tabs.some((tab) => tab.id === selectedId)
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
        if (!isGridMode()) {
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

  const icons = useMemo(
    () => ({
      close: <LuX className="size-2.5" />,
    }),
    [],
  )

  const {
    closeOtherTabs: handleCloseOtherTabs,
    closeTab: handleCloseTab,
    closeTabById: handleCloseTabById,
    duplicateTab: handleDuplicateTab,
    newTab: handleNewTab,
    newTabAt: handleNewTabAtTab,
    popoutTab: handlePopoutTab,
    popoutTabById: handlePopoutTabById,
  } = useShellTabActions({
    activeTabId,
    addShellTab,
    closeShellTab,
    model,
    tabs,
  })

  const handleExternalAppDrag = useShellExternalAppDrag({
    addShellTab,
    isGridMode,
    markShellEngaged,
    model,
  })

  const [contextMenu, setContextMenu] =
    useState<ShellTabContextMenuState | null>(null)

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

  const onRenderTabSet = useShellTabToolbar({
    canClose: tabs.length > 1,
    onClose: handleCloseTab,
    onNew: handleNewTab,
    onPopout: handlePopoutTab,
  })

  const { containerRef, menuBarRef } = useShellMenuMeasurement(model)

  const overlayTabContext = useMemo<TabContextValue>(
    () => ({
      tabId: activeTabId,
      addTab: noopAddTab,
      navigateTab: noopNavigateTab,
    }),
    [activeTabId],
  )

  return {
    activeTabId,
    children,
    containerRef,
    contextMenu,
    handleCloseOtherTabs,
    handleCloseTabById,
    handleContextMenu,
    handleDuplicateTab,
    handleExternalAppDrag,
    handleModelChange,
    handleNewTabAtTab,
    handlePopoutTabById,
    icons,
    menuBarRef,
    model,
    onRenderTab,
    onRenderTabSet,
    overlayTabContext,
    renderTab,
    setContextMenu,
    startRenaming,
  }
}

function ShellTabStripInner({
  children,
}: Pick<ShellTabStripProps, 'children'>) {
  const controller = useShellTabStripController(children)

  return (
    <ShellTabStateProvider tabId={controller.activeTabId}>
      <TabContextProvider value={controller.overlayTabContext}>
        <div
          ref={controller.containerRef}
          className="shell-flexlayout shell-flexlayout--with-menu flex flex-1 flex-col overflow-hidden"
        >
          <div ref={controller.menuBarRef} className="shell-menu-bar-overlay">
            {controller.children}
          </div>
          <OptimizedLayout
            model={controller.model}
            renderTab={controller.renderTab}
            onModelChange={controller.handleModelChange}
            onContextMenu={controller.handleContextMenu}
            onExternalDrag={controller.handleExternalAppDrag}
            onRenderTab={controller.onRenderTab}
            onRenderTabSet={controller.onRenderTabSet}
            icons={controller.icons}
          />
          <ShellTabContextMenu
            state={controller.contextMenu}
            canCloseTabs={countShellModelTabs(controller.model) > 1}
            onClose={() => controller.setContextMenu(null)}
            onNewTab={controller.handleNewTabAtTab}
            onRenameTab={controller.startRenaming}
            onDuplicateTab={controller.handleDuplicateTab}
            onPopoutTab={controller.handlePopoutTabById}
            onCloseOtherTabs={controller.handleCloseOtherTabs}
            onCloseTab={controller.handleCloseTabById}
          />
        </div>
      </TabContextProvider>
    </ShellTabStateProvider>
  )
}
