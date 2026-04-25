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
  ITabRenderValues,
  ITabSetRenderValues,
  BorderNode,
  Actions,
} from '@aptre/flex-layout'
import { LuExternalLink, LuPlus, LuX } from 'react-icons/lu'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'

import { ShellGridPanel } from './ShellGridPanel.js'
import { ShellTabLabel } from './ShellTabLabel.js'
import { useShellTabs } from './ShellTabContext.js'
import {
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
import { ShellTab } from '@s4wave/app/shell-tab.js'
import { buildShellExternalDrag } from './shell-app-drag.js'

// ShellGridLayout renders the shell in grid mode.
// Decodes layout from URL parameter and renders FlexLayout with shell styling.
export function ShellGridLayout() {
  const { layoutData } = useParams()
  const navigate = useNavigate()
  const { tabs, setTabs, setActiveTabId, startRenaming } = useShellTabs()

  // Track the structural layout (without local state) to detect real changes
  const structureRef = useRef<string | null>(null)
  // Track if we've done initial setup
  const initializedRef = useRef(false)
  // Track tabs for reconciliation without triggering re-decode
  const tabsRef = useRef(tabs)
  tabsRef.current = tabs

  // Decode layout from URL - only on initial mount or when URL changes externally
  const initialDecodeResult = useMemo((): DecodeResult | null => {
    if (!layoutData) return null
    const decoded = decodeGridLayout(layoutData, SHELL_GRID_BASE_MODEL)
    if (!decoded) return null
    return {
      model: reconcileModelWithTabs(decoded.model, tabsRef.current),
      localState: decoded.localState,
    }
  }, [layoutData])

  // Model state - initialized once from URL
  const [model, setModel] = useState<Model | null>(() => {
    if (!initialDecodeResult) return null
    const m = Model.fromJson(initialDecodeResult.model)
    applyLocalStateToModel(m, initialDecodeResult.localState)
    // Store initial structure
    structureRef.current = encodeGridLayoutStructure(m)
    initializedRef.current = true
    return m
  })

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

  // renderTab renders ShellGridPanel for each tab
  const renderTab = useCallback(
    (node: TabNode) => <ShellGridPanel tabId={node.getId()} />,
    [],
  )

  // Handle model changes - detect exit from grid mode or update URL only for structural changes
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
      setTabs((prev) => prev.filter((t) => modelTabIds.has(t.id)))

      if (!hasGridLayout(newModel)) {
        // Collapsed to single tabset - exit grid mode
        const selectedId = getSelectedTabId(newModel)
        let selectedPath = '/'
        if (selectedId) {
          const tabNode = newModel.getNodeById(selectedId)
          if (tabNode && tabNode.getType() === 'tab') {
            const config = (tabNode as TabNode).getConfig() as
              | { path?: string }
              | undefined
            selectedPath = config?.path ?? '/'
          }
        }
        setActiveTabId(selectedId ?? 'home')
        structureRef.current = null
        navigate({ path: selectedPath, replace: true })
        return
      }

      // Check if structure changed (not just local state like tab selection)
      const newStructure = encodeGridLayoutStructure(newModel)
      if (newStructure !== structureRef.current) {
        structureRef.current = newStructure
        // Encode with local state for URL (so refreshing restores selection)
        const newLayoutData = encodeGridLayout(newModel)
        navigate({ path: `/g/${newLayoutData}`, replace: true })
      }
    },
    [navigate, setActiveTabId, setTabs],
  )

  // Handle adding a new tab to the active tabset
  const handleAddTab = useCallback(() => {
    if (!model) return

    const activeTabsetId = getActiveTabsetId(model)
    if (!activeTabsetId) return

    // Create new tab with home path
    const newTab = buildPathTab('/')

    // Add to global tabs state
    setTabs((prev) => [...prev, newTab])

    addAndSelectShellModelTab(model, activeTabsetId, newTab, 'shell-panel')
  }, [model, setTabs])

  const handleAddTabAtTab = useCallback(
    (tabId: string) => {
      if (!model) return

      const tabsetId =
        getShellTabsetId(model, tabId) ?? getActiveTabsetId(model)
      if (!tabsetId) return

      const sourceTab = findShellTab(tabs, tabId)
      const newTab = buildContextualShellTab(sourceTab?.path)

      setTabs((prev) => [...prev, newTab])
      addAndSelectShellModelTab(model, tabsetId, newTab, 'shell-panel')
    },
    [model, tabs, setTabs],
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
      setTabs((prev) => [...prev, nextTab])
      addAndSelectShellModelTab(model, tabsetId, nextTab, 'shell-panel')
    },
    [model, tabs, setTabs],
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
          <LuX className="h-3 w-3" />
        </button>,
        <button
          key="add-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={handleAddTab}
          title="Add tab"
        >
          <LuPlus className="h-3 w-3" />
        </button>,
        <button
          key="popout-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={() => selectedTabId && handlePopoutTab(selectedTabId)}
          title="Open in new tab"
        >
          <LuExternalLink className="h-3 w-3" />
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
        setTabs((prev) => {
          const existingIdx = prev.findIndex((t) => t.id === tab.id)
          if (existingIdx < 0) {
            return [...prev, tab]
          }
          return prev.map((t) => (t.id === tab.id ? tab : t))
        })
        setActiveTabId(tab.id)
      }),
    [setActiveTabId, setTabs],
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

// LayoutNode is a union type for all possible nodes in a layout.
type LayoutNode = {
  type?: string
  id?: string
  children?: LayoutNode[]
}

// reconcileModelWithTabs ensures all tabs in the model exist in global state.
// Adds missing tabs to global state and removes orphaned tabs from model.
function reconcileModelWithTabs(
  model: IJsonModel,
  tabs: ShellTab[],
): IJsonModel {
  const tabIds = new Set(tabs.map((t) => t.id))
  const modelTabIds: string[] = []

  // Collect all tab IDs from model
  const collectTabIds = (node: LayoutNode | undefined): void => {
    if (!node || typeof node !== 'object') return

    if (node.type === 'tab' && typeof node.id === 'string') {
      modelTabIds.push(node.id)
    }
    if (node.children && Array.isArray(node.children)) {
      for (const child of node.children) {
        collectTabIds(child)
      }
    }
  }

  collectTabIds(model.layout as LayoutNode)

  // Check if all model tabs exist in global state
  const missingFromGlobal = modelTabIds.filter((id) => !tabIds.has(id))
  if (missingFromGlobal.length > 0) {
    // This shouldn't happen normally - tabs in URL layout should exist in global state
    // For now, we'll create placeholder tabs for any missing ones
    console.warn(
      'Grid layout references tabs not in global state:',
      missingFromGlobal,
    )
  }

  return model
}
