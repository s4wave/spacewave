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
import { UnixFSTypeID } from '@s4wave/web/hooks/useUnixFSHandle.js'
import {
  useAllViewers,
  getViewersForType,
} from '@s4wave/web/hooks/useViewerRegistry.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import type { ObjectViewerComponent } from './object.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { ComponentSelector } from './ComponentSelector.js'
import { ObjectViewerDetails } from './ObjectViewerDetails.js'
import {
  useIsLastBottomBarItem,
  useBottomBarSetOpenMenu,
} from '@s4wave/web/frame/bottom-bar-context.js'

// UseObjectViewerProps are the props for the useObjectViewer hook.
export interface UseObjectViewerProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  bottomBarId?: string
  stateNamespace?: string[]
  exportUrl?: string
}

// UseObjectViewerResult is the return type of the useObjectViewer hook.
export interface UseObjectViewerResult {
  objectState: Resource<IObjectState | null>
  typeID: string | undefined
  rootRef: string | undefined
  objectKey: string | undefined
  visibleComponents: ObjectViewerComponent[]
  selectedComponent: ObjectViewerComponent | undefined
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

// useObjectViewer extracts all object viewer state logic for both world and
// unixfs objects. Shared by ObjectViewer across all embedding contexts.
export function useObjectViewer({
  objectInfo,
  worldState,
  bottomBarId,
  stateNamespace,
  exportUrl,
}: UseObjectViewerProps): UseObjectViewerResult {
  const infoCase = objectInfo?.info?.case
  const barId = bottomBarId ?? 'objectViewer'

  // For world objects, use the standard setup hook.
  const worldObjectKey =
    infoCase === 'worldObjectInfo' ?
      (objectInfo.info?.value as { objectKey?: string })?.objectKey
    : undefined

  const worldSetup = useObjectViewerSetup(worldState, worldObjectKey)

  // For unixfs objects, resolve typeID directly.
  const isUnixfs = infoCase === 'unixfsObjectInfo'
  const rootResource = RootContext.useContext()
  const allViewers = useAllViewers(rootResource)

  const unixfsComponents = useMemo(() => {
    if (!isUnixfs) return []
    return getViewersForType(UnixFSTypeID, allViewers)
  }, [isUnixfs, allViewers])

  // Merge the two paths.
  const typeID = isUnixfs ? UnixFSTypeID : worldSetup.typeID
  const objectState = worldSetup.objectState
  const rootRef = isUnixfs ? undefined : worldSetup.rootRef
  const visibleComponents =
    isUnixfs ? unixfsComponents : worldSetup.visibleComponents
  const objectKey = isUnixfs ? undefined : worldObjectKey

  // State namespace for component selection persistence.
  const defaultNs = useMemo(
    () => getDefaultStateNamespace(objectInfo, objectKey, stateNamespace),
    [objectInfo, objectKey, stateNamespace],
  )
  const namespace = useStateNamespace(defaultNs)

  const [selectedComponentName, setSelectedComponentName] = useStateAtom<
    string | undefined
  >(namespace, 'selectedComponent', undefined)

  const [selectorOpen, setSelectorOpen] = useState(false)

  const selectedComponent = useMemo(() => {
    if (visibleComponents.length === 0) return undefined
    if (selectedComponentName) {
      const found = visibleComponents.find(
        (c) => c.name === selectedComponentName,
      )
      if (found) return found
    }
    return visibleComponents[0]
  }, [visibleComponents, selectedComponentName])

  const handleSelectComponent = useCallback(
    (component: ObjectViewerComponent) => {
      setSelectedComponentName(component.name)
    },
    [setSelectedComponentName],
  )

  const viewerContextValue = useMemo(
    () => ({
      visibleComponents,
      selectedComponent,
      onSelectComponent: handleSelectComponent,
    }),
    [visibleComponents, selectedComponent, handleSelectComponent],
  )

  // Bottom bar state.
  const isLastItem = useIsLastBottomBarItem(barId)
  const setOpenMenu = useBottomBarSetOpenMenu()

  const selectedComponentDisplay = selectedComponent?.name ?? 'default'
  const hasMultipleComponents = visibleComponents.length > 1
  const displayKey = objectKey ?? (isUnixfs ? 'UnixFS' : 'No object')

  const buttonKeyValue = useMemo(
    () =>
      [
        displayKey,
        selectedComponentDisplay,
        hasMultipleComponents ? 'multi' : 'single',
        selectorOpen ? 'open' : 'closed',
        typeID ?? 'none',
      ].join(':'),
    [
      displayKey,
      selectedComponentDisplay,
      hasMultipleComponents,
      selectorOpen,
      typeID,
    ],
  )

  const overlayKeyValue = useMemo(
    () =>
      [
        objectKey ?? 'none',
        typeID ?? 'none',
        rootRef ?? 'none',
        selectedComponentDisplay,
      ].join(':'),
    [objectKey, typeID, rootRef, selectedComponentDisplay],
  )

  const handleCloseDetails = useCallback(() => {
    setOpenMenu?.('')
  }, [setOpenMenu])

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => {
      const showComponentName = selected || isLastItem
      return (
        <BottomBarItem
          selected={selected}
          onClick={onClick}
          className={className}
        >
          <div className="flex-shrink flex-grow truncate">{displayKey}</div>
          {showComponentName && selectedComponent && hasMultipleComponents ?
            <>
              <div className="bg-border mx-2 h-3 w-px" />
              <ComponentSelector
                open={selectorOpen}
                onOpenChange={setSelectorOpen}
                components={visibleComponents}
                selectedComponent={selectedComponent}
                onSelectComponent={handleSelectComponent}
              >
                <div className="text-muted-foreground truncate text-xs">
                  {selectedComponent.name}
                </div>
              </ComponentSelector>
            </>
          : showComponentName && selectedComponent ?
            <>
              <div className="bg-border mx-2 h-3 w-px" />
              <div className="text-muted-foreground truncate text-xs">
                {selectedComponent.name}
              </div>
            </>
          : showComponentName && typeID ?
            <>
              <div className="bg-border mx-2 h-3 w-px" />
              <div className="text-muted-foreground truncate text-xs">
                Type: {typeID}
              </div>
            </>
          : null}
        </BottomBarItem>
      )
    },
    [
      displayKey,
      isLastItem,
      selectedComponent,
      hasMultipleComponents,
      selectorOpen,
      visibleComponents,
      handleSelectComponent,
      typeID,
    ],
  )

  const overlayContent = useMemo(
    () =>
      typeID && objectKey ?
        <ObjectViewerDetails
          key={selectedComponent?.name}
          objectKey={objectKey}
          typeID={typeID}
          rootRef={rootRef ?? ''}
          exportUrl={
            exportUrl && objectKey ?
              `${exportUrl}/-/${encodeURIComponent(objectKey)}`
            : undefined
          }
          availableComponents={visibleComponents}
          selectedComponent={selectedComponent}
          onComponentSelect={handleSelectComponent}
          onCloseClick={handleCloseDetails}
        />
      : undefined,
    [
      typeID,
      objectKey,
      rootRef,
      exportUrl,
      visibleComponents,
      selectedComponent,
      handleSelectComponent,
      handleCloseDetails,
    ],
  )

  // Export object command: active when viewing a world object with an export URL.
  const isTabActive = useIsTabActive()
  const objectExportUrl = useMemo(
    () =>
      exportUrl && objectKey ?
        `${exportUrl}/-/${encodeURIComponent(objectKey)}`
      : undefined,
    [exportUrl, objectKey],
  )

  useCommand({
    commandId: 'spacewave.file.export-object',
    label: 'Export Object',
    description: 'Download object contents',
    menuPath: 'File/Export Object',
    menuGroup: 4,
    menuOrder: 2,
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

  return {
    objectState,
    typeID,
    rootRef,
    objectKey,
    visibleComponents,
    selectedComponent,
    onSelectComponent: handleSelectComponent,
    viewerContextValue,
    buttonRender,
    overlayContent,
    buttonKeyValue,
    overlayKeyValue,
  }
}
