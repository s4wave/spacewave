import {
  useCallback,
  useMemo,
  useRef,
  useState,
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
import { FlexTabNameInput } from '@s4wave/web/layout/FlexTabNameInput.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state'
import { DocumentTitleFocusContext } from '@s4wave/web/title/DocumentTitleFocusContext.js'

import type { ObjectViewerComponentProps } from './object.js'
import { getObjectKey } from './object.js'
import { TabContentContainer } from './TabContent.js'
import { buildObjectLayoutExternalDrag } from './layout-object-app-drag.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: { width: '100%', maxWidth: 'var(--container-sm)' },
})

// ObjectLayoutTypeID is the type identifier for ObjectLayout objects.
export const ObjectLayoutTypeID = 'alpha/object-layout'

interface ObjectLayoutTabContextMenuState extends FlexTabContextMenuState {
  model: Model
}

interface ObjectLayoutTabLabelProps {
  tabId: string
  name: string
  editing: boolean
  onStartRename: () => void
  onRename: (name: string) => void
  onStopRename: () => void
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
  editing,
  onStartRename,
  ...props
}: ObjectLayoutTabLabelProps) {
  if (editing) {
    return (
      <FlexTabNameInput
        name={props.name}
        label={`Rename ${props.tabId}`}
        onSave={(name) => {
          props.onRename(name || props.name)
          props.onStopRename()
        }}
        onCancel={props.onStopRename}
      />
    )
  }

  return (
    <span
      className="truncate"
      onDoubleClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        onStartRename()
      }}
    >
      {props.name}
    </span>
  )
}

// useObjectLayoutController owns persisted layout state and the tab actions
// that mutate the current FlexLayout model.
function useObjectLayoutController({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Scope persisted layout state to this shared object's identity.
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

  // Use the mounted layout host for subsequent layout actions.
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
    ({
      tabID,
      tabData,
      navigate,
      addTab,
      replaceTab,
      nameTab,
    }: ITabComponentProps) => {
      return (
        <TabContentContainer
          tabID={tabID}
          tabData={tabData}
          navigate={navigate}
          addTab={addTab}
          replaceTab={replaceTab}
          nameTab={nameTab}
        />
      )
    },
    [],
  )

  // Translate app drag payloads into the existing object-layout drop contract.
  const handleExternalDrag = useCallback(
    (event: Parameters<typeof buildObjectLayoutExternalDrag>[0]) =>
      buildObjectLayoutExternalDrag(event),
    [],
  )

  // Keep the active model and tab menu editing state together.
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

  // Render the object tab's label and close control from its current model.
  const handleRenderTab = useCallback(
    (node: TabNode, renderValues: ITabRenderValues) => {
      // Keep tab renaming tied to the model currently being rendered.
      const model = node.getModel()
      const tabId = node.getId()
      latestModelRef.current = model
      renderValues.content = (
        <ObjectLayoutTabLabel
          tabId={tabId}
          name={node.getName()}
          editing={renamingTabId === tabId}
          onStartRename={() => setRenamingTabId(tabId)}
          onRename={(name) => model.doAction(Actions.renameTab(tabId, name))}
          onStopRename={() => setRenamingTabId(null)}
        />
      )
      if (node.isEnableClose()) return

      // Allow closing a tab only while another object tab remains.
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
    [renamingTabId],
  )

  // Open a menu for the tab under the pointer.
  const handleContextMenu = useCallback(
    (node: TabNode | TabSetNode | BorderNode, event: ReactMouseEvent) => {
      // Capture the selected tab, model and pointer position for its menu.
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

  // Preserve the last tab when the layout requests a close.
  const handleLayoutAction = useCallback((action: Action) => {
    if (
      action.type === Actions.DELETE_TAB &&
      countObjectLayoutTabs(latestModelRef.current) <= 1
    ) {
      return undefined
    }
    return action
  }, [])

  // Duplicate a tab beside its source in the same tabset.
  const handleDuplicateTab = useCallback(
    (tabId: string) => {
      // Require the source tab and its current tabset before copying it.
      const model = contextMenu?.model
      const node = model?.getNodeById(tabId)
      if (!model || !node || node.getType() !== 'tab') return
      const tabNode = node as TabNode
      const parent = tabNode.getParent()
      if (!parent || parent.getType() !== 'tabset') return

      // Copy the source configuration into a newly selected sibling tab.
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

  // Close the other tabs while preserving the chosen tab.
  const handleCloseOtherTabs = useCallback(
    (keepTabId: string) => {
      // Collect the other tab identities before deleting them from the mutable model.
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

  // Close the chosen tab only while another tab remains.
  const handleCloseTab = useCallback(
    (tabId: string) => {
      const model = contextMenu?.model
      if (!model || countObjectLayoutTabs(model) <= 1) return
      model.doAction(Actions.deleteTab(tabId))
    },
    [contextMenu],
  )

  // Build tab menu actions from the current model's close eligibility.
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

  // Keep the layout library's close icon stable between renders.
  const icons = useMemo(
    () => ({
      close: <LuX className="size-2.5" />,
    }),
    [],
  )

  // Keep layout callbacks and icons stable while their dependencies stay unchanged.
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

  return {
    contextMenu,
    contextMenuItems,
    flexLayoutProps,
    focusedTabId,
    handleLocalStateChange,
    layoutHost,
    memoizedLocalState,
    renderTab,
    setContextMenu,
  }
}

// LayoutObjectViewer renders an ObjectLayout world object using BaseLayout.
export function LayoutObjectViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const {
    contextMenu,
    contextMenuItems,
    flexLayoutProps,
    focusedTabId,
    handleLocalStateChange,
    layoutHost,
    memoizedLocalState,
    renderTab,
    setContextMenu,
  } = useObjectLayoutController({ objectInfo, worldState })

  if (layoutHost === null) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <LoadingCard
          view={{ state: 'loading', title: 'Loading layout' }}
          sx={stylexStyles.rule1}
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
