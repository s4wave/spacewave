import { useState, useCallback } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'
import { HistoryRouter } from '@s4wave/web/router/HistoryRouter.js'
import type { To } from '@s4wave/web/router/router.js'
import { StateNamespaceProvider } from '@s4wave/web/state'

import { ObjectViewerContent } from './ObjectViewerContent.js'
import { ObjectViewerProvider } from './ObjectViewerContext.js'
import { ObjectViewerLoadingState } from './ObjectViewerLoadingState.js'
import { ObjectViewerNotFoundState } from './ObjectViewerNotFoundState.js'
import type { ObjectInfo } from './object.pb.js'
import { getObjectKey } from './object.js'
import { useObjectViewer } from './useObjectViewer.js'

const noopNavigate = () => {}

export interface ObjectViewerProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  spaceContents?: Resource<SpaceContents>
  standalone?: boolean
  bottomBarId?: string
  path?: string
  exportUrl?: string
  onNavigate?: (to: To) => void
  onBreadcrumbClick?: () => void
  stateNamespace?: string[]
  preferredComponentID?: string
}

export function ObjectViewer({
  objectInfo,
  worldState,
  spaceContents,
  standalone,
  bottomBarId,
  path,
  exportUrl,
  onNavigate,
  onBreadcrumbClick,
  stateNamespace,
  preferredComponentID,
}: ObjectViewerProps) {
  const barId = bottomBarId ?? 'objectViewer'

  const viewer = useObjectViewer({
    objectInfo,
    worldState,
    bottomBarId: barId,
    stateNamespace,
    exportUrl,
    preferredComponentID,
  })

  const routerPath = path ?? '/'
  const navigateHandler = onNavigate ?? noopNavigate

  const objectKey = getObjectKey(objectInfo)
  const barLabel = objectKey ?? 'Object'
  const requiresObjectState =
    objectInfo?.info?.case === 'worldObjectInfo' &&
    (viewer.selectedComponent?.requiresObjectState ?? true)
  const missingWorldObject =
    objectInfo?.info?.case === 'worldObjectInfo' &&
    requiresObjectState &&
    !!worldState.value &&
    !viewer.objectState.loading &&
    !viewer.objectState.value

  const worldReady =
    objectInfo?.info?.case !== 'worldObjectInfo' ||
    (!!worldState.value && (!requiresObjectState || !!viewer.objectState.value))
  const loading = viewer.typeID === undefined || !worldReady

  let content
  if (missingWorldObject) {
    content = <ObjectViewerNotFoundState objectKey={barLabel} />
  } else if (loading || viewer.typeID === undefined) {
    content = <ObjectViewerLoadingState />
  } else {
    content = (
      <HistoryRouter path={routerPath} onNavigate={navigateHandler}>
        <ObjectViewerContent
          objectInfo={objectInfo}
          worldState={worldState}
          spaceContents={spaceContents}
          objectState={viewer.objectState.value ?? undefined}
          typeID={viewer.typeID}
          component={viewer.selectedComponent}
          availableComponents={viewer.visibleComponents}
          missingComponentID={viewer.missingComponentID}
          onSelectComponent={viewer.onSelectComponent}
          standalone={standalone}
        />
      </HistoryRouter>
    )
  }

  const inner = (
    <ObjectViewerProvider value={viewer.viewerContextValue}>
      {content}
    </ObjectViewerProvider>
  )
  const namespacedInner = stateNamespace ? (
    <StateNamespaceProvider namespace={stateNamespace}>
      {inner}
    </StateNamespaceProvider>
  ) : (
    inner
  )

  const [openMenu, setOpenMenu] = useState('')
  const handleSetOpenMenu = useCallback((id: string) => setOpenMenu(id), [])

  if (standalone) {
    const frameContent = <ViewerFrame>{namespacedInner}</ViewerFrame>

    return (
      <div className="flex h-full w-full flex-col">
        <BottomBarRoot openMenu={openMenu} setOpenMenu={handleSetOpenMenu}>
          <BottomBarLevel
            id={barId}
            button={viewer.buttonRender}
            overlay={viewer.overlayContent}
            buttonKey={viewer.buttonKeyValue}
            overlayKey={viewer.overlayKeyValue}
            menuLabel={barLabel}
            onBreadcrumbClick={onBreadcrumbClick}
          >
            {frameContent}
          </BottomBarLevel>
        </BottomBarRoot>
      </div>
    )
  }

  return (
    <BottomBarLevel
      id={barId}
      button={viewer.buttonRender}
      overlay={viewer.overlayContent}
      buttonKey={viewer.buttonKeyValue}
      overlayKey={viewer.overlayKeyValue}
      menuLabel={barLabel}
      onBreadcrumbClick={onBreadcrumbClick}
    >
      {namespacedInner}
    </BottomBarLevel>
  )
}
