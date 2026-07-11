import {
  type DragEvent as ReactDragEvent,
  useCallback,
  useMemo,
  useState,
  useEffect,
  useRef,
} from 'react'
import {
  OptimizedLayout,
  Model,
  TabNode,
  TabSetNode,
  IJsonModel,
  IJsonBorderNode,
  IJsonRowNode,
  IJsonTabNode,
  IJsonTabSetNode,
  ITabRenderValues,
  ITabSetRenderValues,
  BorderNode,
  Actions,
} from '@aptre/flex-layout'
import { LuExternalLink, LuPlus, LuX } from 'react-icons/lu'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'

import { ShellGridPanel } from './ShellGridPanel.js'
import { ShellTabLabel } from './ShellTabLabel.js'
import { useShellTabs, type OpenShellTabOptions } from './ShellTabContext.js'
import {
  addShellModelTab,
  addAndSelectShellModelTab,
  buildContextualShellTab,
  buildPathTab,
  cloneShellTab,
  countShellModelTabs,
  findShellTab,
  getShellTabsetId,
} from './shell-layout-tab-utils.js'
import { openShellTabInNewTab } from './shell-popout.js'
import {
  ShellTabContextMenu,
  type ShellTabContextMenuState,
} from './ShellTabContextMenu.js'
import {
  decodeGridLayout,
  encodeGridLayout,
  encodeGridLayoutStructure,
  hasGridLayout,
  getSelectedTabId,
  getActiveTabsetId,
  applyLocalStateToModel,
  SHELL_GRID_BASE_MODEL,
  type DecodeResult,
} from './shell-grid-utils.js'
import { getTabDisplayName, type ShellTab } from '@s4wave/app/shell-tab.js'
import { buildShellExternalDrag } from './shell-app-drag.js'

// ShellGridLayout renders the shell in grid mode.
// Decodes layout from URL parameter and renders FlexLayout with shell styling.
export function ShellGridLayout() {
  const { layoutData } = useParams()
  const navigate = useNavigate()
  const {
    tabs,
    activeTabId,
    addShellTab,
    selectShellTab,
    retainShellTabs,
    startRenaming,
    registerActiveTabsetPathOpener,
  } = useShellTabs()

  // Decode layout from URL - only on initial mount or when URL changes externally
  const initialDecodeResult = useMemo((): DecodeResult | null => {
    if (!layoutData) return null
    const decoded = decodeGridLayout(layoutData, SHELL_GRID_BASE_MODEL)
    if (!decoded) return null
    return {
      model: reconcileModelWithTabs(decoded.model, tabs),
      localState: decoded.localState,
    }
  }, [layoutData, tabs])

  const initialModelResult = useMemo(() => {
    if (!initialDecodeResult) return null
    const m = Model.fromJson(initialDecodeResult.model)
    applyLocalStateToModel(m, initialDecodeResult.localState)
    return {
      model: m,
      structure: encodeGridLayoutStructure(m),
    }
  }, [initialDecodeResult])

  // Track the structural layout (without local state) to detect real changes
  const structureRef = useRef<string | null>(
    initialModelResult?.structure ?? null,
  )
  // Track if we've done initial setup
  const initializedRef = useRef(initialModelResult !== null)

  // Model state - initialized once from URL
  const [model, setModel] = useState<Model | null>(
    initialModelResult?.model ?? null,
  )

  // Handle URL changes from external sources (back/forward navigation)
  useEffect(() => {
    if (!initializedRef.current || !layoutData || !initialDecodeResult) return

    // Decode the new URL's structure to compare
    const newModel = Model.fromJson(initialDecodeResult.model)
    const newStructure = encodeGridLayoutStructure(newModel)

    // Only update model if structure actually changed (external navigation)
    if (newStructure !== structureRef.current) {
      applyLocalStateToModel(newModel, initialDecodeResult.localState)
      structureRef.current = newStructure
      setModel(newModel)
    }
  }, [layoutData, initialDecodeResult])

  // Handle invalid layout - redirect to home
  useEffect(() => {
    if (!layoutData || !model) {
      queueMicrotask(() => navigate({ path: '/', replace: true }))
    }
  }, [layoutData, model, navigate])

  useEffect(() => {
    if (!model || !hasGridLayout(model)) return

    const activeTab = findShellTab(tabs, activeTabId)
    if (!activeTab) return

    if (model.getNodeById(activeTab.id)) {
      if (getSelectedTabId(model) !== activeTab.id) {
        model.doAction(Actions.selectTab(activeTab.id))
      }
      return
    }

    const activeTabsetId = getActiveTabsetId(model)
    if (!activeTabsetId) return

    addAndSelectShellModelTab(model, activeTabsetId, activeTab, 'shell-panel')
    const newStructure = encodeGridLayoutStructure(model)
    if (newStructure !== structureRef.current) {
      structureRef.current = newStructure
      navigate({ path: `/g/${encodeGridLayout(model)}`, replace: true })
    }
  }, [activeTabId, model, navigate, tabs])

  // renderTab renders ShellGridPanel for each tab
  const renderTab = useCallback(
    (node: TabNode) => <ShellGridPanel tabId={node.getId()} />,
    [],
  )

  // Handle model changes without replacing the grid layout owner.
  const handleModelChange = useCallback(
    (newModel: Model) => {
      setModel(newModel)

      // Sync tabs state with model - remove tabs that no longer exist in model
      const modelTabIds = new Set<string>()
      newModel.visitNodes((node) => {
        if (node.getType() === 'tab') {
          modelTabIds.add(node.getId())
        }
      })
      retainShellTabs(modelTabIds, getSelectedTabId(newModel) ?? undefined)

      // Check if structure changed (not just local state like tab selection)
      const newStructure = encodeGridLayoutStructure(newModel)
      if (newStructure !== structureRef.current) {
        structureRef.current = newStructure
        // Encode with local state for URL (so refreshing restores selection)
        const newLayoutData = encodeGridLayout(newModel)
        navigate({ path: `/g/${newLayoutData}`, replace: true })
      }
    },
    [navigate, retainShellTabs],
  )

  // Handle adding a new tab to the active tabset
  const handleAddTab = useCallback(() => {
    if (!model) return

    const activeTabsetId = getActiveTabsetId(model)
    if (!activeTabsetId) return

    // Create new tab with home path
    const newTab = buildPathTab('/')

    addShellTab(newTab, { select: true })

    addAndSelectShellModelTab(model, activeTabsetId, newTab, 'shell-panel')
  }, [model, addShellTab])

  const handleAddTabAtTab = useCallback(
    (tabId: string) => {
      if (!model) return

      const tabsetId =
        getShellTabsetId(model, tabId) ?? getActiveTabsetId(model)
      if (!tabsetId) return

      const sourceTab = findShellTab(tabs, tabId)
      const newTab = buildContextualShellTab(sourceTab?.path)

      addShellTab(newTab, { select: true })
      addAndSelectShellModelTab(model, tabsetId, newTab, 'shell-panel')
    },
    [model, tabs, addShellTab],
  )

  const openPathInGridTabset = useCallback(
    (path: string, options: OpenShellTabOptions = {}) => {
      if (!model) return null
      const select = options.select ?? true
      const existingTab = options.focusExisting
        ? tabs.find((tab) => tab.path === path && model.getNodeById(tab.id))
        : undefined

      if (existingTab) {
        if (select) {
          selectShellTab(existingTab.id)
          model.doAction(Actions.selectTab(existingTab.id))
        }
        return existingTab.id
      }

      const activeTabsetId = getActiveTabsetId(model)
      if (!activeTabsetId) return null

      const newTab = buildPathTab(path)
      addShellTab(newTab, {
        afterTabId: options.afterTabId,
        select,
      })
      if (select) {
        addAndSelectShellModelTab(model, activeTabsetId, newTab, 'shell-panel')
      } else {
        addShellModelTab(model, activeTabsetId, newTab, 'shell-panel')
      }
      return newTab.id
    },
    [addShellTab, model, selectShellTab, tabs],
  )

  useEffect(
    () => registerActiveTabsetPathOpener(openPathInGridTabset),
    [openPathInGridTabset, registerActiveTabsetPathOpener],
  )

  // Handle closing a tab
  const handleCloseTab = useCallback(
    (tabId: string) => {
      if (!model) return

      // Don't allow closing the last tab
      let tabCount = 0
      model.visitNodes((node) => {
        if (node.getType() === 'tab') tabCount++
      })
      if (tabCount <= 1) return

      // Remove from FlexLayout - tabs state sync happens in handleModelChange
      model.doAction(Actions.deleteTab(tabId))
    },
    [model],
  )

  // Handle popping out current tab to a new browser tab
  const handlePopoutTab = useCallback(
    (tabId: string) => {
      const tab = findShellTab(tabs, tabId)
      if (!tab) return
      openShellTabInNewTab(tab.path)
    },
    [tabs],
  )

  const handleDuplicateTab = useCallback(
    (tabId: string) => {
      if (!model) return

      const tab = findShellTab(tabs, tabId)
      if (!tab) return

      const tabsetId =
        getShellTabsetId(model, tabId) ?? getActiveTabsetId(model)
      if (!tabsetId) return

      const nextTab = cloneShellTab(tab)
      addShellTab(nextTab, { select: true })
      addAndSelectShellModelTab(model, tabsetId, nextTab, 'shell-panel')
    },
    [model, tabs, addShellTab],
  )

  const handleCloseOtherTabs = useCallback(
    (keepTabId: string) => {
      if (!model) return

      tabs.forEach((tab) => {
        if (tab.id !== keepTabId) {
          model.doAction(Actions.deleteTab(tab.id))
        }
      })
    },
    [model, tabs],
  )

  // Render tab with name from global state, supporting inline rename.
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

  // Render tabset toolbar with add/close buttons
  const onRenderTabSet = useCallback(
    (node: TabSetNode | BorderNode, renderValues: ITabSetRenderValues) => {
      if (node.getType() !== 'tabset') return

      const tabset = node as TabSetNode
      const selectedNode = tabset.getSelectedNode()
      const selectedTabId = selectedNode?.getId()

      // Count total tabs in model
      let tabCount = 0
      model?.visitNodes((n) => {
        if (n.getType() === 'tab') tabCount++
      })

      renderValues.stickyButtons.push(
        <button
          key="close-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={() => selectedTabId && handleCloseTab(selectedTabId)}
          title="Close tab"
          disabled={tabCount <= 1}
        >
          <LuX className="size-3" />
        </button>,
        <button
          key="add-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handleAddTab}
          title="Add tab"
        >
          <LuPlus className="size-3" />
        </button>,
        <button
          key="popout-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={() => selectedTabId && handlePopoutTab(selectedTabId)}
          title="Open in new tab"
        >
          <LuExternalLink className="size-3" />
        </button>,
      )
    },
    [model, handleAddTab, handleCloseTab, handlePopoutTab],
  )

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

  const handleExternalAppDrag = useCallback(
    (event: ReactDragEvent<HTMLElement>) =>
      buildShellExternalDrag(event, (tab) => {
        addShellTab(tab, { select: true })
      }),
    [addShellTab],
  )

  if (!model) {
    return null
  }

  return (
    <div className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1">
      <OptimizedLayout
        model={model}
        renderTab={renderTab}
        onModelChange={handleModelChange}
        onContextMenu={handleContextMenu}
        onExternalDrag={handleExternalAppDrag}
        onRenderTab={onRenderTab}
        onRenderTabSet={onRenderTabSet}
      />
      <ShellTabContextMenu
        state={contextMenu}
        canCloseTabs={countShellModelTabs(model) > 1}
        onClose={() => setContextMenu(null)}
        onNewTab={handleAddTabAtTab}
        onRenameTab={startRenaming}
        onDuplicateTab={handleDuplicateTab}
        onPopoutTab={handlePopoutTab}
        onCloseOtherTabs={handleCloseOtherTabs}
        onCloseTab={handleCloseTab}
      />
    </div>
  )
}

function normalizeSelectedIndex(
  selected: number | undefined,
  originalChildren: IJsonTabNode[],
  children: IJsonTabNode[],
  emptySelected: number | undefined,
): number | undefined {
  if (children.length === 0 || typeof selected !== 'number') {
    return children.length === 0 ? emptySelected : selected
  }
  if (selected < 0) return selected
  const selectedChildId = originalChildren[selected]?.id
  if (!selectedChildId) return 0
  const nextSelected = children.findIndex(
    (child) => child.id === selectedChildId,
  )
  return nextSelected >= 0 ? nextSelected : 0
}

// reconcileModelWithTabs ensures all tabs in the model exist in global state.
// Shell tab paths and display names are owned by ShellTabsContext, so decoded
// grid URL tabs with no global state entry cannot be rendered correctly.
function reconcileModelWithTabs(
  model: IJsonModel,
  tabs: ShellTab[],
): IJsonModel {
  const tabsById = new Map(tabs.map((tab) => [tab.id, tab]))

  const reconcileTabNode = (node: IJsonTabNode): IJsonTabNode | null => {
    const tab = node.id ? tabsById.get(node.id) : null
    if (!tab) return null
    return {
      ...node,
      name: getTabDisplayName(tab),
    }
  }

  const reconcileTabChildren = (children: IJsonTabNode[]): IJsonTabNode[] =>
    children.flatMap((child) => {
      const next = reconcileTabNode(child)
      return next ? [next] : []
    })

  const reconcileTabsetNode = (
    node: IJsonTabSetNode,
    keepEmpty: boolean,
  ): IJsonTabSetNode | null => {
    const children = reconcileTabChildren(node.children)
    if (children.length === 0 && !keepEmpty) return null
    return {
      ...node,
      children,
      selected: normalizeSelectedIndex(
        node.selected,
        node.children,
        children,
        0,
      ),
    }
  }

  const reconcileRowNode = (
    node: IJsonRowNode,
    keepEmpty: boolean,
  ): IJsonRowNode | null => {
    const children = node.children.flatMap((child) => {
      const next =
        child.type === 'row'
          ? reconcileRowNode(child, false)
          : reconcileTabsetNode(child, false)
      return next ? [next] : []
    })
    if (children.length === 0 && !keepEmpty) return null
    return {
      ...node,
      children,
    }
  }

  const reconcileBorderNode = (node: IJsonBorderNode): IJsonBorderNode => {
    const children = reconcileTabChildren(node.children)
    return {
      ...node,
      children,
      selected: normalizeSelectedIndex(
        node.selected,
        node.children,
        children,
        -1,
      ),
    }
  }

  return {
    ...model,
    layout: reconcileRowNode(model.layout, true) ?? {
      type: 'row',
      weight: 100,
      children: [],
    },
    borders: model.borders?.map(reconcileBorderNode),
  }
}
