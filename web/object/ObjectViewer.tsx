import { useState, useCallback } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
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

// noopNavigate is a fallback when no navigation handler is provided.
const noopNavigate = () => {}

// ObjectViewerProps are the props for the ObjectViewer component.
export interface ObjectViewerProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  standalone?: boolean
  bottomBarId?: string
  path?: string
  exportUrl?: string
  onNavigate?: (to: To) => void
  onBreadcrumbClick?: () => void
  stateNamespace?: string[]
}

// ObjectViewer is a reusable component that renders an object viewer with
// bottom bar integration. Two render modes:
//   standalone=false: registers a BottomBarLevel in the parent BottomBarRoot
//   standalone=true: wraps in its own BottomBarRoot + ViewerFrame
export function ObjectViewer({
  objectInfo,
  worldState,
  standalone,
  bottomBarId,
  path,
  exportUrl,
  onNavigate,
  onBreadcrumbClick,
  stateNamespace,
}: ObjectViewerProps) {
  const barId = bottomBarId ?? 'objectViewer'

  const viewer = useObjectViewer({
    objectInfo,
    worldState,
    bottomBarId: barId,
    stateNamespace,
    exportUrl,
  })

  const routerPath = path ?? '/'
  const navigateHandler = onNavigate ?? noopNavigate

  const objectKey = getObjectKey(objectInfo)
  const missingWorldObject =
    objectInfo?.info?.case === 'worldObjectInfo' &&
    !!worldState.value &&
    !viewer.objectState.loading &&
    !viewer.objectState.value

  const worldReady =
    objectInfo?.info?.case !== 'worldObjectInfo' ||
    (!!viewer.objectState.value && !!worldState.value)
  const loading =
    viewer.typeID === undefined ||
    // For world objects, require objectState and worldState to be loaded.
    !worldReady

  let content
  if (missingWorldObject) {
    content = <ObjectViewerNotFoundState objectKey={objectKey} />
  } else if (loading || viewer.typeID === undefined) {
    content = <ObjectViewerLoadingState />
  } else {
    content = (
      <HistoryRouter path={routerPath} onNavigate={navigateHandler}>
        <ObjectViewerContent
          objectInfo={objectInfo}
          worldState={worldState}
          objectState={viewer.objectState.value ?? undefined}
          typeID={viewer.typeID}
          component={viewer.selectedComponent}
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
      onBreadcrumbClick={onBreadcrumbClick}
    >
      {namespacedInner}
    </BottomBarLevel>
  )
}
