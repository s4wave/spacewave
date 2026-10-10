import { useMemo, useCallback, useState } from 'react'
import type React from 'react'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { ObjectInfo } from './object.pb.js'
import { useObjectViewerSetup } from './useObjectViewerSetup.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'
import {
  useAllViewers,
  getViewersForType,
} from '@s4wave/web/hooks/useViewerRegistry.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { useDocumentTitle } from '@s4wave/web/title/DocumentTitleContext.js'
import { useDocumentTitleFocus } from '@s4wave/web/title/DocumentTitleFocusContext.js'
import type { ObjectViewerComponent } from './object.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { ComponentSelector } from './ComponentSelector.js'
import { ObjectViewerDetails } from './ObjectViewerDetails.js'
import {
  useIsLastBottomBarItem,
  useBottomBarSetOpenMenu,
} from '@s4wave/web/frame/bottom-bar-context.js'
import type { BottomBarContextMenuItem } from '@s4wave/web/frame/bottom-bar-context.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { getObjectDisplayName } from '@s4wave/web/space/object-tree.js'
import { createSpaceObjectNavigationActions } from '@s4wave/web/space/space-object-navigation-actions.js'
import { useTabContext } from './TabContext.js'
import {
  hasObjectViewerSwitchOwner,
  switchObjectAtViewerPosition,
} from './object-viewer-space-actions.js'

// UseObjectViewerProps are the props for the useObjectViewer hook.
export interface UseObjectViewerProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  bottomBarId?: string
  stateNamespace?: string[]
  exportUrl?: string
  preferredComponentID?: string
}

// UseObjectViewerResult is the return type of the useObjectViewer hook.
export interface UseObjectViewerResult {
  objectState: Resource<IObjectState | null>
  typeID: string | undefined
  rootRef: string | undefined
  objectKey: string | undefined
  visibleComponents: ObjectViewerComponent[]
  selectedComponent: ObjectViewerComponent | undefined
  missingComponentID: string | undefined
  onSelectComponent: (c: ObjectViewerComponent) => void
  viewerContextValue: {
    visibleComponents: ObjectViewerComponent[]
    selectedComponent?: ObjectViewerComponent
    onSelectComponent: (c: ObjectViewerComponent) => void
  }
  buttonRender: (
    selected: boolean,
    onClick: () => void,
    className?: string,
  ) => React.ReactNode
  overlayContent: React.ReactNode | undefined
  buttonKeyValue: string
  overlayKeyValue: string
  contextMenuItems: readonly BottomBarContextMenuItem[] | undefined
  contextMenuKey: string
  contextMenuLabel: string
}

export interface ObjectViewerSelection {
  selectedComponent: ObjectViewerComponent | undefined
  missingComponentID: string | undefined
}

export const debugViewerComponentID = 'spacewave.debug.viewer'

export function isDebugViewerComponent(
  component: ObjectViewerComponent | undefined,
): boolean {
  return component?.componentID === debugViewerComponentID
}

export function shouldHoldDebugViewerFallback(
  typeID: string | undefined,
  selectedComponent: ObjectViewerComponent | undefined,
  selectedComponentID: string | undefined,
  preferredComponentID: string | undefined,
): boolean {
  if (!typeID) return false
  if (!isDebugViewerComponent(selectedComponent)) return false
  return (
    selectedComponentID !== debugViewerComponentID &&
    preferredComponentID !== debugViewerComponentID
  )
}

export function resolveObjectViewerSelection(
  visibleComponents: ObjectViewerComponent[],
  selectedComponentID: string | undefined,
  preferredComponentID: string | undefined,
): ObjectViewerSelection {
  if (visibleComponents.length === 0) {
    return {
      selectedComponent: undefined,
      missingComponentID: selectedComponentID ?? preferredComponentID,
    }
  }

  const requestedComponentID = selectedComponentID ?? preferredComponentID
  if (requestedComponentID) {
    const found = visibleComponents.find(
      (component) => component.componentID === requestedComponentID,
    )
    if (found) {
      return { selectedComponent: found, missingComponentID: undefined }
    }
    return {
      selectedComponent: visibleComponents[0],
      missingComponentID: requestedComponentID,
    }
  }

  return {
    selectedComponent: visibleComponents[0],
    missingComponentID: undefined,
  }
}

export function getDefaultStateNamespace(
  objectInfo: ObjectInfo,
  objectKey: string | undefined,
  stateNamespace: string[] | undefined,
): string[] {
  if (stateNamespace) {
    return stateNamespace
  }
  if (objectInfo?.info?.case === 'unixfsObjectInfo') {
    const unixfsId = objectInfo.info.value.unixfsId || 'none'
    const unixfsPath = objectInfo.info.value.path || '/'
    return ['objectViewer', 'unixfs', unixfsId, unixfsPath]
  }
  return ['objectViewer', objectKey ?? 'none']
}

type SpaceContext = ReturnType<typeof SpaceContainerContext.useContextSafe>

// useViewerSource resolves the type, state, and viewers of an object. World
// objects use the standard setup hook and unixfs objects resolve directly.
function useViewerSource(
  objectInfo: ObjectInfo,
  worldState: Resource<IWorldState>,
  spaceContext: SpaceContext,
) {
  const infoCase = objectInfo?.info?.case
  const worldObjectKey =
    infoCase === 'worldObjectInfo'
      ? (objectInfo.info?.value as { objectKey?: string })?.objectKey
      : undefined
  const worldObjectType =
    infoCase === 'worldObjectInfo'
      ? (objectInfo.info?.value as { objectType?: string })?.objectType ||
        undefined
      : undefined

  const worldSetup = useObjectViewerSetup(worldState, worldObjectKey, {
    typeIDHint: worldObjectType,
    loadObjectState: worldObjectType !== UnixFSTypeID,
  })

  const isUnixfs = infoCase === 'unixfsObjectInfo'
  const rootResource = RootContext.useContext()
  const allViewers = useAllViewers(
    rootResource,
    spaceContext?.spaceState.engineId,
  )
  const unixfsComponents = useMemo(() => {
    if (!isUnixfs) return []
    return getViewersForType(UnixFSTypeID, allViewers)
  }, [isUnixfs, allViewers])

  return {
    isUnixfs,
    typeID: isUnixfs ? UnixFSTypeID : worldSetup.typeID,
    objectState: worldSetup.objectState,
    rootRef: isUnixfs ? undefined : worldSetup.rootRef,
    visibleComponents: isUnixfs
      ? unixfsComponents
      : worldSetup.visibleComponents,
    objectKey: isUnixfs ? undefined : worldObjectKey,
  }
}

interface ViewerSelectionOptions {
  objectInfo: ObjectInfo
  objectKey: string | undefined
  stateNamespace: string[] | undefined
  visibleComponents: ObjectViewerComponent[]
  typeID: string | undefined
  preferredComponentID: string | undefined
}

// useViewerSelection resolves the selected viewer component, persisting the
// user's choice in the state namespace.
function useViewerSelection({
  objectInfo,
  objectKey,
  stateNamespace,
  visibleComponents,
  typeID,
  preferredComponentID,
}: ViewerSelectionOptions) {
  const defaultNs = useMemo(
    () => getDefaultStateNamespace(objectInfo, objectKey, stateNamespace),
    [objectInfo, objectKey, stateNamespace],
  )
  const namespace = useStateNamespace(defaultNs)

  const [selectedComponentID, setSelectedComponentID] = useStateAtom<
    string | undefined
  >(namespace, 'selectedComponentID', undefined)

  const { selectedComponent, missingComponentID } = useMemo(() => {
    const selection = resolveObjectViewerSelection(
      visibleComponents,
      selectedComponentID,
      preferredComponentID,
    )
    if (
      shouldHoldDebugViewerFallback(
        typeID,
        selection.selectedComponent,
        selectedComponentID,
        preferredComponentID,
      )
    ) {
      return {
        selectedComponent: undefined,
        missingComponentID: selection.missingComponentID ?? typeID,
      }
    }
    return selection
  }, [visibleComponents, selectedComponentID, preferredComponentID, typeID])

  const handleSelectComponent = useCallback(
    (component: ObjectViewerComponent) => {
      setSelectedComponentID(component.componentID)
    },
    [setSelectedComponentID],
  )

  const viewerContextValue = useMemo(
    () => ({
      visibleComponents,
      selectedComponent,
      onSelectComponent: handleSelectComponent,
    }),
    [visibleComponents, selectedComponent, handleSelectComponent],
  )

  return {
    selectedComponent,
    missingComponentID,
    handleSelectComponent,
    viewerContextValue,
  }
}

// useViewerTitle sets the document title while the viewer's tab is active.
function useViewerTitle(
  spaceContext: SpaceContext,
  objectKey: string | undefined,
  isUnixfs: boolean,
  visibleComponents: ObjectViewerComponent[],
  selectedComponent: ObjectViewerComponent | undefined,
) {
  const tabContext = useTabContext()
  const isTabActive = useIsTabActive()
  const focusedTabId = useDocumentTitleFocus()
  const isTopLevelObject = !!objectKey && spaceContext?.objectKey === objectKey
  const isFocusedLayoutTab =
    tabContext?.isObjectLayout === true &&
    !!tabContext.tabId &&
    tabContext.tabId === focusedTabId
  const objectTitle = objectKey
    ? getObjectDisplayName(objectKey) || 'Object'
    : isUnixfs
      ? 'Files'
      : 'Object'
  const viewTitle =
    visibleComponents.length > 1 && selectedComponent?.name
      ? `${objectTitle} · ${selectedComponent.name}`
      : objectTitle

  useDocumentTitle(
    { view: viewTitle, space: spaceContext?.spaceName ?? 'Space' },
    {
      active: isTabActive && (isTopLevelObject || isFocusedLayoutTab),
      priority: isFocusedLayoutTab ? 30 : 20,
    },
  )
}

// objectExportURL returns the export URL of an object, when it has one.
function objectExportURL(
  exportUrl: string | undefined,
  objectKey: string | undefined,
): string | undefined {
  if (!exportUrl || !objectKey) return undefined
  return `${exportUrl}/-/${encodeURIComponent(objectKey)}`
}

// useExportCommand registers the Export Object command, active while a world
// object with an export URL is shown in the active tab.
function useExportCommand(objectExportUrl: string | undefined) {
  const isTabActive = useIsTabActive()

  useCommand({
    commandId: 'spacewave.file.export-object',
    label: 'Export Object',
    description: 'Download object contents',
    menuPath: 'File/Export Object',
    menuGroup: 30,
    menuOrder: 3,
    active: isTabActive && !!objectExportUrl,
    handler: useCallback(() => {
      if (!objectExportUrl) return
      const a = document.createElement('a')
      a.href = objectExportUrl
      a.download = ''
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
    }, [objectExportUrl]),
  })
}

// barComponentLabel names the selected viewer component, or the type when no
// viewer is selected, for the bottom bar. It is null when there is nothing to
// name.
function barComponentLabel(
  selectedComponent: ObjectViewerComponent | undefined,
  typeID: string | undefined,
): string | null {
  if (selectedComponent) return selectedComponent.name
  if (typeID) return `Type: ${typeID}`
  return null
}

const barLabelDivider = <div className="bg-border mx-2 h-3 w-px" />

interface ViewerButtonOptions {
  barId: string
  displayKey: string
  typeID: string | undefined
  visibleComponents: ObjectViewerComponent[]
  selectedComponent: ObjectViewerComponent | undefined
  onSelectComponent: (component: ObjectViewerComponent) => void
}

// useViewerButton renders the bottom bar button of the viewer and the key that
// changes when it must re-render.
function useViewerButton({
  barId,
  displayKey,
  typeID,
  visibleComponents,
  selectedComponent,
  onSelectComponent,
}: ViewerButtonOptions) {
  const isLastItem = useIsLastBottomBarItem(barId)
  const [selectorOpen, setSelectorOpen] = useState(false)
  const selectedComponentIDDisplay = selectedComponent?.componentID ?? 'default'
  const hasMultipleComponents = visibleComponents.length > 1

  const buttonKeyValue = useMemo(
    () =>
      [
        displayKey,
        selectedComponentIDDisplay,
        hasMultipleComponents ? 'multi' : 'single',
        selectorOpen ? 'open' : 'closed',
        typeID ?? 'none',
      ].join(':'),
    [
      displayKey,
      selectedComponentIDDisplay,
      hasMultipleComponents,
      selectorOpen,
      typeID,
    ],
  )

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => {
      const label =
        selected || isLastItem
          ? barComponentLabel(selectedComponent, typeID)
          : null
      const text = (
        <div className="text-muted-foreground truncate text-xs">{label}</div>
      )
      // The selector is a button, so it sits beside the item's button.
      const selector =
        label !== null && selectedComponent && visibleComponents.length > 1 ? (
          <div className="flex min-w-0 items-center">
            {barLabelDivider}
            <ComponentSelector
              open={selectorOpen}
              onOpenChange={setSelectorOpen}
              components={visibleComponents}
              selectedComponent={selectedComponent}
              onSelectComponent={onSelectComponent}
            >
              {text}
            </ComponentSelector>
          </div>
        ) : undefined

      return (
        <BottomBarItem
          selected={selected}
          onClick={onClick}
          className={className}
          trailing={selector}
        >
          <div className="flex-shrink flex-grow truncate">{displayKey}</div>
          {label !== null && !selector && (
            <>
              {barLabelDivider}
              {text}
            </>
          )}
        </BottomBarItem>
      )
    },
    [
      displayKey,
      isLastItem,
      selectedComponent,
      selectorOpen,
      visibleComponents,
      onSelectComponent,
      typeID,
    ],
  )

  return { buttonRender, buttonKeyValue, selectedComponentIDDisplay }
}

interface ViewerOverlayOptions {
  spaceContext: SpaceContext
  objectKey: string | undefined
  typeID: string | undefined
  rootRef: string | undefined
  objectExportUrl: string | undefined
  visibleComponents: ObjectViewerComponent[]
  selectedComponent: ObjectViewerComponent | undefined
  missingComponentID: string | undefined
  selectedComponentIDDisplay: string
  onSelectComponent: (component: ObjectViewerComponent) => void
}

// useViewerOverlay renders the object details overlay and the key that changes
// when it must re-render.
function useViewerOverlay({
  spaceContext,
  objectKey,
  typeID,
  rootRef,
  objectExportUrl,
  visibleComponents,
  selectedComponent,
  missingComponentID,
  selectedComponentIDDisplay,
  onSelectComponent,
}: ViewerOverlayOptions) {
  const setOpenMenu = useBottomBarSetOpenMenu()

  const overlayKeyValue = useMemo(
    () =>
      [
        objectKey ?? 'none',
        typeID ?? 'none',
        rootRef ?? 'none',
        selectedComponentIDDisplay,
        spaceContext?.canDeleteObjects ? 'delete' : 'read-only',
      ].join(':'),
    [objectKey, typeID, rootRef, selectedComponentIDDisplay, spaceContext],
  )

  const handleCloseDetails = useCallback(() => {
    setOpenMenu?.('')
  }, [setOpenMenu])

  const handleDeleteObject = useCallback(async () => {
    if (!objectKey || !spaceContext?.canDeleteObjects) return
    await spaceContext.spaceWorld.deleteObject(objectKey)
    setOpenMenu?.('')
    spaceContext.navigateToRoot()
  }, [objectKey, setOpenMenu, spaceContext])

  const overlayContent = useMemo(
    () =>
      typeID && objectKey ? (
        <ObjectViewerDetails
          key={selectedComponent?.componentID}
          objectKey={objectKey}
          typeID={typeID}
          rootRef={rootRef ?? ''}
          exportUrl={objectExportUrl}
          availableComponents={visibleComponents}
          selectedComponent={selectedComponent}
          missingComponentID={missingComponentID}
          onComponentSelect={onSelectComponent}
          onCloseClick={handleCloseDetails}
          onDeleteConfirm={
            spaceContext?.canDeleteObjects ? handleDeleteObject : undefined
          }
        />
      ) : undefined,
    [
      typeID,
      objectKey,
      rootRef,
      objectExportUrl,
      visibleComponents,
      selectedComponent,
      missingComponentID,
      onSelectComponent,
      handleCloseDetails,
      handleDeleteObject,
      spaceContext?.canDeleteObjects,
    ],
  )

  return { overlayContent, overlayKeyValue }
}

// useViewerContextMenu builds the bottom bar context menu that navigates
// between the objects of the space.
function useViewerContextMenu(
  spaceContext: SpaceContext,
  objectKey: string | undefined,
  displayKey: string,
  hasDetails: boolean,
) {
  const tabContext = useTabContext()
  const loadedSpaceObjectTargets = spaceContext?.spaceObjectTargets?.targets
  const spaceObjectTargets = useMemo(
    () => loadedSpaceObjectTargets ?? [],
    [loadedSpaceObjectTargets],
  )
  const moreSpaceObjectTargets = !!spaceContext?.spaceObjectTargets?.more
  const handleOpenObject = useCallback(
    (target: { objectKey: string }) => {
      spaceContext?.navigateToObjects([target.objectKey])
    },
    [spaceContext],
  )
  const replaceTab =
    tabContext?.isObjectLayout === true ? tabContext.replaceTab : undefined
  const tabId = tabContext?.isObjectLayout === true ? tabContext.tabId : ''
  const positionOwner = useMemo(
    () => ({
      tabId,
      replaceTab,
      switchObjectAtCurrentPosition:
        spaceContext?.switchObjectAtCurrentPosition,
    }),
    [tabId, replaceTab, spaceContext?.switchObjectAtCurrentPosition],
  )
  const handleSwitchObjectHere = useCallback(
    (target: (typeof spaceObjectTargets)[number]) =>
      switchObjectAtViewerPosition(target, positionOwner),
    [positionOwner],
  )

  const contextMenuItems = useMemo(
    () =>
      createSpaceObjectNavigationActions({
        targets: spaceObjectTargets,
        moreTargets: moreSpaceObjectTargets,
        currentObjectKey: objectKey,
        openDetails: hasDetails ? () => {} : undefined,
        openObject: spaceContext?.navigateToObjects
          ? handleOpenObject
          : undefined,
        switchObjectHere: hasObjectViewerSwitchOwner(positionOwner)
          ? handleSwitchObjectHere
          : undefined,
      }),
    [
      spaceObjectTargets,
      moreSpaceObjectTargets,
      objectKey,
      hasDetails,
      spaceContext?.navigateToObjects,
      handleOpenObject,
      positionOwner,
      handleSwitchObjectHere,
    ],
  )
  const contextMenuKey = useMemo(
    () =>
      [
        displayKey,
        objectKey ?? 'none',
        hasDetails ? 'details' : 'no-details',
        spaceContext?.navigateToObjects ? 'open' : 'no-open',
        hasObjectViewerSwitchOwner(positionOwner) ? 'switch' : 'no-switch',
        spaceObjectTargets
          .map((target) => `${target.objectKey}:${target.objectType}`)
          .join('|'),
        moreSpaceObjectTargets ? 'more' : 'all',
      ].join(':'),
    [
      displayKey,
      objectKey,
      hasDetails,
      spaceContext?.navigateToObjects,
      positionOwner,
      spaceObjectTargets,
      moreSpaceObjectTargets,
    ],
  )

  return {
    contextMenuItems,
    contextMenuKey,
    contextMenuLabel: `${displayKey} actions`,
  }
}

// useObjectViewer extracts all object viewer state logic for both world and
// unixfs objects. Shared by ObjectViewer across all embedding contexts.
export function useObjectViewer({
  objectInfo,
  worldState,
  bottomBarId,
  stateNamespace,
  exportUrl,
  preferredComponentID,
}: UseObjectViewerProps): UseObjectViewerResult {
  const spaceContext = SpaceContainerContext.useContextSafe()
  const source = useViewerSource(objectInfo, worldState, spaceContext)
  const { typeID, rootRef, objectKey, visibleComponents } = source
  const selection = useViewerSelection({
    objectInfo,
    objectKey,
    stateNamespace,
    visibleComponents,
    typeID,
    preferredComponentID,
  })
  const { selectedComponent, missingComponentID } = selection
  const onSelectComponent = selection.handleSelectComponent
  const displayKey = objectKey ?? (source.isUnixfs ? 'UnixFS' : 'No object')
  const objectExportUrl = objectExportURL(exportUrl, objectKey)

  useViewerTitle(
    spaceContext,
    objectKey,
    source.isUnixfs,
    visibleComponents,
    selectedComponent,
  )
  useExportCommand(objectExportUrl)
  const button = useViewerButton({
    barId: bottomBarId ?? 'objectViewer',
    displayKey,
    typeID,
    visibleComponents,
    selectedComponent,
    onSelectComponent,
  })
  const overlay = useViewerOverlay({
    spaceContext,
    objectKey,
    typeID,
    rootRef,
    objectExportUrl,
    visibleComponents,
    selectedComponent,
    missingComponentID,
    selectedComponentIDDisplay: button.selectedComponentIDDisplay,
    onSelectComponent,
  })
  const contextMenu = useViewerContextMenu(
    spaceContext,
    objectKey,
    displayKey,
    !!overlay.overlayContent,
  )

  return {
    objectState: source.objectState,
    typeID,
    rootRef,
    objectKey,
    visibleComponents,
    selectedComponent,
    missingComponentID,
    onSelectComponent,
    viewerContextValue: selection.viewerContextValue,
    buttonRender: button.buttonRender,
    overlayContent: overlay.overlayContent,
    buttonKeyValue: button.buttonKeyValue,
    overlayKeyValue: overlay.overlayKeyValue,
    ...contextMenu,
  }
}
