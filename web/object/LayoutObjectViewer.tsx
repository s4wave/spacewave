import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent as ReactMouseEvent,
} from 'react'
import {
  Actions,
  DockLocation,
  type Action,
  type BorderNode,
  type ITabRenderValues,
  type Model,
  type TabNode,
  type TabSetNode,
} from '@aptre/flex-layout'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuCircleX, LuCopy, LuPencil, LuX } from 'react-icons/lu'

import { ILocalState, cloneLocalState } from '@s4wave/sdk/layout/layout.js'
import { LayoutHostHandle } from '@s4wave/sdk/layout/layout-host.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import {
  BaseLayout,
  ITabComponentProps,
} from '@s4wave/web/layout/BaseLayout.js'
import {
  FlexTabContextMenu,
  type FlexTabContextMenuEntry,
  type FlexTabContextMenuState,
} from '@s4wave/web/layout/FlexTabContextMenu.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state'
import { cn } from '@s4wave/web/style/utils.js'
import { DocumentTitleFocusContext } from '@s4wave/web/title/DocumentTitleFocusContext.js'

import type { ObjectViewerComponentProps } from './object.js'
import { getObjectKey } from './object.js'
import { TabContentContainer } from './TabContent.js'
import { buildObjectLayoutExternalDrag } from './layout-object-app-drag.js'

// ObjectLayoutTypeID is the type identifier for ObjectLayout objects.
export const ObjectLayoutTypeID = 'alpha/object-layout'

interface ObjectLayoutTabContextMenuState extends FlexTabContextMenuState {
  model: Model
}

interface ObjectLayoutTabLabelProps {
  tabId: string
  name: string
  renaming: boolean
  onRename: (name: string) => void
  onRenameHandled: () => void
}

function countObjectLayoutTabs(model: Model | null): number {
  let count = 0
  model?.visitNodes((node) => {
    if (node.getType() === 'tab') {
      count += 1
    }
  })
  return count
}

// ObjectLayoutTabLabel renders an object-layout tab name with inline rename.
function ObjectLayoutTabLabel({
  tabId,
  name,
  renaming,
  onRename,
  onRenameHandled,
}: ObjectLayoutTabLabelProps) {
  const [editing, setEditing] = useState(false)
  const [editValue, setEditValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!renaming) return
    setEditValue(name)
    setEditing(true)
    onRenameHandled()
  }, [renaming, name, onRenameHandled])

  useEffect(() => {
    if (!editing || !inputRef.current) return
    inputRef.current.focus()
    inputRef.current.select()
  }, [editing])

  const handleDoubleClick = useCallback(
    (event: ReactMouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
      setEditValue(name)
      setEditing(true)
    },
    [name],
  )

  const handleSave = useCallback(() => {
    onRename(editValue.trim() || name)
    setEditing(false)
  }, [editValue, name, onRename])

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      event.stopPropagation()
      if (event.key === 'Enter') {
        event.preventDefault()
        handleSave()
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        setEditing(false)
      }
    },
    [handleSave],
  )

  if (editing) {
    return (
      <input
        ref={inputRef}
        aria-label={`Rename ${tabId}`}
        className={cn(
          'bg-background-secondary text-foreground rounded-menu-button',
          'border-none outline-none',
          'text-[0.6875rem] leading-5 font-medium tracking-[-0.01em]',
          'w-full max-w-64 min-w-12 px-1 py-0',
        )}
        value={editValue}
        onChange={(event) => setEditValue(event.target.value)}
        onBlur={handleSave}
        onKeyDown={handleKeyDown}
        onMouseDown={(event) => event.stopPropagation()}
        onClick={(event) => event.stopPropagation()}
      />
    )
  }

  return (
    <span className="truncate" onDoubleClick={handleDoubleClick}>
      {name}
    </span>
  )
}

// LayoutObjectViewer renders an ObjectLayout world object using BaseLayout.
export function LayoutObjectViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  // Create namespace for persisting local state
  const namespace = useStateNamespace(['layout', objectKey])

  // Persist local layout state (selected tabs, active tabset, maximized tab)
  const [localState, setLocalState] = useStateAtom<ILocalState>(
    namespace,
    'localState',
    { tabSetSelected: {} },
  )

  // Access the typed object resource for this layout
  const typedObjectResource = useAccessTypedHandle(
    worldState,
    objectKey,
    LayoutHostHandle,
    ObjectLayoutTypeID,
  )

  const layoutHost = useResourceValue(typedObjectResource)

  // Handle local state changes
  const handleLocalStateChange = useCallback(
    (nextState: ILocalState) => {
      setLocalState(cloneLocalState(nextState))
    },
    [setLocalState],
  )

  // Memoize local state to avoid unnecessary re-renders
  const memoizedLocalState = useMemo(
    () => cloneLocalState(localState),
    [localState],
  )

  // Render tab content
  const renderTab = useCallback(
    ({ tabID, tabData, navigate, addTab, replaceTab }: ITabComponentProps) => {
      return (
        <TabContentContainer
          tabID={tabID}
          tabData={tabData}
          navigate={navigate}
          addTab={addTab}
          replaceTab={replaceTab}
        />
      )
    },
    [],
  )

  const handleExternalDrag = useCallback(
    (event: Parameters<typeof buildObjectLayoutExternalDrag>[0]) =>
      buildObjectLayoutExternalDrag(event),
    [],
  )

  const latestModelRef = useRef<Model | null>(null)
  const [contextMenu, setContextMenu] =
    useState<ObjectLayoutTabContextMenuState | null>(null)
  const [renamingTabId, setRenamingTabId] = useState<string | null>(null)
  const selectedTabIds = Object.values(localState.tabSetSelected)
  const focusedTabId = localState.activeTabSet
    ? (localState.tabSetSelected[localState.activeTabSet] ?? null)
    : selectedTabIds.length === 1
      ? selectedTabIds[0]
      : null

  const handleRenameHandled = useCallback(() => {
    setRenamingTabId(null)
  }, [])

  const handleRenderTab = useCallback(
    (node: TabNode, renderValues: ITabRenderValues) => {
      const model = node.getModel()
      const tabId = node.getId()
      latestModelRef.current = model
      renderValues.content = (
        <ObjectLayoutTabLabel
          tabId={tabId}
          name={node.getName()}
          renaming={renamingTabId === tabId}
          onRename={(name) => model.doAction(Actions.renameTab(tabId, name))}
          onRenameHandled={handleRenameHandled}
        />
      )
      if (node.isEnableClose()) return

      const canCloseTabs = countObjectLayoutTabs(model) > 1
      renderValues.buttons.push(
        <button
          key="object-layout-close-tab"
          type="button"
          className="flexlayout__tab_button_trailing"
          title="Close"
          aria-label={`Close ${node.getName()}`}
          disabled={!canCloseTabs}
          onPointerDown={(event) => {
            event.preventDefault()
            event.stopPropagation()
          }}
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            if (countObjectLayoutTabs(model) <= 1) return
            model.doAction(Actions.deleteTab(tabId))
          }}
        >
          <LuX className="size-2.5" />
        </button>,
      )
    },
    [handleRenameHandled, renamingTabId],
  )

  const handleContextMenu = useCallback(
    (node: TabNode | TabSetNode | BorderNode, event: ReactMouseEvent) => {
      if (node.getType() !== 'tab') return
      event.preventDefault()
      const model = node.getModel()
      latestModelRef.current = model
      setContextMenu({
        tabId: node.getId(),
        x: event.clientX,
        y: event.clientY,
        model,
      })
    },
    [],
  )

  const handleLayoutAction = useCallback((action: Action) => {
    if (
      action.type === Actions.DELETE_TAB &&
      countObjectLayoutTabs(latestModelRef.current) <= 1
    ) {
      return undefined
    }
    return action
  }, [])

  const handleDuplicateTab = useCallback(
    (tabId: string) => {
      const model = contextMenu?.model
      const node = model?.getNodeById(tabId)
      if (!model || !node || node.getType() !== 'tab') return
      const tabNode = node as TabNode
      const parent = tabNode.getParent()
      if (!parent || parent.getType() !== 'tabset') return
      const json = tabNode.toJson()
      if (!json) return
      const children = parent.getChildren()
      const index = children.findIndex((child) => child.getId() === tabId)
      model.doAction(
        Actions.addNode(
          {
            ...json,
            id: `tab-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
            name: tabNode.getName(),
            enableClose: true,
          },
          parent.getId(),
          DockLocation.CENTER,
          index < 0 ? -1 : index + 1,
          true,
        ),
      )
    },
    [contextMenu],
  )

  const handleCloseOtherTabs = useCallback(
    (keepTabId: string) => {
      const model = contextMenu?.model
      if (!model || countObjectLayoutTabs(model) <= 1) return
      const tabIds: string[] = []
      model.visitNodes((node) => {
        if (node.getType() === 'tab' && node.getId() !== keepTabId) {
          tabIds.push(node.getId())
        }
      })
      for (const tabId of tabIds) {
        model.doAction(Actions.deleteTab(tabId))
      }
    },
    [contextMenu],
  )

  const handleCloseTab = useCallback(
    (tabId: string) => {
      const model = contextMenu?.model
      if (!model || countObjectLayoutTabs(model) <= 1) return
      model.doAction(Actions.deleteTab(tabId))
    },
    [contextMenu],
  )

  const canCloseContextTabs = contextMenu
    ? countObjectLayoutTabs(contextMenu.model) > 1
    : false
  const contextMenuItems = useMemo<FlexTabContextMenuEntry[]>(
    () => [
      {
        id: 'rename-tab',
        label: 'Rename Tab',
        icon: <LuPencil className="size-4" />,
        onSelect: setRenamingTabId,
      },
      {
        id: 'duplicate-tab',
        label: 'Duplicate Tab',
        icon: <LuCopy className="size-4" />,
        onSelect: handleDuplicateTab,
      },
      { id: 'close-separator', type: 'separator' },
      {
        id: 'close-other-tabs',
        label: 'Close Other Tabs',
        icon: <LuCircleX className="size-4" />,
        disabled: !canCloseContextTabs,
        onSelect: handleCloseOtherTabs,
      },
      {
        id: 'close-tab',
        label: 'Close Tab',
        icon: <LuX className="size-4" />,
        disabled: !canCloseContextTabs,
        variant: 'destructive',
        onSelect: handleCloseTab,
      },
    ],
    [
      canCloseContextTabs,
      handleCloseOtherTabs,
      handleCloseTab,
      handleDuplicateTab,
    ],
  )

  const icons = useMemo(
    () => ({
      close: <LuX className="size-2.5" />,
    }),
    [],
  )

  const flexLayoutProps = useMemo(
    () => ({
      onExternalDrag: handleExternalDrag,
      onContextMenu: handleContextMenu,
      onRenderTab: handleRenderTab,
      onAction: handleLayoutAction,
      icons,
    }),
    [
      handleContextMenu,
      handleExternalDrag,
      handleLayoutAction,
      handleRenderTab,
      icons,
    ],
  )

  if (layoutHost === null) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <LoadingCard
          view={{ state: 'loading', title: 'Loading layout' }}
          className="w-full max-w-sm"
        />
      </div>
    )
  }

  if (layoutHost === undefined) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center">
        Failed to load layout
      </div>
    )
  }

  return (
    <div className="space-flexlayout bg-foreground/6 relative flex h-full w-full flex-col gap-1 overflow-hidden text-xs">
      <div className="relative flex h-full w-full flex-1 flex-col">
        <DocumentTitleFocusContext.Provider value={focusedTabId}>
          <BaseLayout
            layoutHost={layoutHost}
            renderTab={renderTab}
            flexLayoutProps={flexLayoutProps}
            localState={memoizedLocalState}
            onLocalStateChange={handleLocalStateChange}
          />
        </DocumentTitleFocusContext.Provider>
        <FlexTabContextMenu
          state={contextMenu}
          items={contextMenuItems}
          onClose={() => setContextMenu(null)}
        />
      </div>
    </div>
  )
}
