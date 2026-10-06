import { useState, useCallback, type ReactNode } from 'react'

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
import {
  useObjectViewer,
  type UseObjectViewerResult,
} from './useObjectViewer.js'

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
  renderMissingComponent?: (
    componentID: string,
    objectKey: string,
    typeID: string,
  ) => ReactNode
}

// worldViewState reports whether the object a viewer needs is missing from the
// world or still loading.
function worldViewState(
  objectInfo: ObjectInfo,
  worldState: ObjectViewerProps['worldState'],
  viewer: UseObjectViewerResult,
): { missing: boolean; ready: boolean } {
  if (objectInfo?.info?.case !== 'worldObjectInfo') {
    return { missing: false, ready: true }
  }
  if (!(viewer.selectedComponent?.requiresObjectState ?? true)) {
    return { missing: false, ready: !!worldState.value }
  }
  return {
    missing:
      !!worldState.value &&
      !viewer.objectState.loading &&
      !viewer.objectState.value,
    ready: !!worldState.value && !!viewer.objectState.value,
  }
}

// ObjectViewerBody picks what the viewer shows: not found, loading, the missing
// component fallback, or the viewer content.
function ObjectViewerBody({
  viewer,
  props,
}: {
  viewer: UseObjectViewerResult
  props: ObjectViewerProps
}) {
  const { objectInfo, worldState, renderMissingComponent } = props
  const objectKey = getObjectKey(objectInfo)
  const barLabel = objectKey ?? 'Object'
  const { missing, ready } = worldViewState(objectInfo, worldState, viewer)

  if (missing) {
    return <ObjectViewerNotFoundState objectKey={barLabel} />
  }
  if (viewer.typeID === undefined || !ready) {
    return <ObjectViewerLoadingState />
  }
  if (viewer.missingComponentID && renderMissingComponent) {
    return renderMissingComponent(
      viewer.missingComponentID,
      barLabel,
      viewer.typeID,
    )
  }

  return (
    <HistoryRouter
      path={props.path ?? '/'}
      onNavigate={props.onNavigate ?? noopNavigate}
    >
      <ObjectViewerContent
        objectInfo={objectInfo}
        worldState={worldState}
        spaceContents={props.spaceContents}
        objectState={viewer.objectState.value ?? undefined}
        typeID={viewer.typeID}
        component={viewer.selectedComponent}
        availableComponents={viewer.visibleComponents}
        missingComponentID={viewer.missingComponentID}
        onSelectComponent={viewer.onSelectComponent}
        standalone={props.standalone}
      />
    </HistoryRouter>
  )
}

// ViewerBottomBarLevel registers the viewer in the bottom bar around its body.
function ViewerBottomBarLevel({
  viewer,
  barId,
  barLabel,
  onBreadcrumbClick,
  children,
}: {
  viewer: UseObjectViewerResult
  barId: string
  barLabel: string
  onBreadcrumbClick?: () => void
  children: ReactNode
}) {
  return (
    <BottomBarLevel
      id={barId}
      button={viewer.buttonRender}
      overlay={viewer.overlayContent}
      buttonKey={viewer.buttonKeyValue}
      overlayKey={viewer.overlayKeyValue}
      menuLabel={barLabel}
      contextMenuLabel={viewer.contextMenuLabel}
      contextMenuKey={viewer.contextMenuKey}
      contextMenuItems={viewer.contextMenuItems}
      onBreadcrumbClick={onBreadcrumbClick}
    >
      {children}
    </BottomBarLevel>
  )
}

// StandaloneViewerRoot owns the bottom bar of a viewer shown outside a layout.
function StandaloneViewerRoot({ children }: { children: ReactNode }) {
  const [openMenu, setOpenMenu] = useState('')
  const handleSetOpenMenu = useCallback((id: string) => setOpenMenu(id), [])

  return (
    <div className="flex h-full w-full flex-col">
      <BottomBarRoot openMenu={openMenu} setOpenMenu={handleSetOpenMenu}>
        {children}
      </BottomBarRoot>
    </div>
  )
}

// ObjectViewer shows an object with the selected viewer component and registers
// the viewer in the bottom bar.
export function ObjectViewer(props: ObjectViewerProps) {
  const { objectInfo, standalone, bottomBarId, stateNamespace } = props
  const barId = bottomBarId ?? 'objectViewer'
  const barLabel = getObjectKey(objectInfo) ?? 'Object'
  const viewer = useObjectViewer({
    objectInfo,
    worldState: props.worldState,
    bottomBarId: barId,
    stateNamespace,
    exportUrl: props.exportUrl,
    preferredComponentID: props.preferredComponentID,
  })

  const inner = (
    <ObjectViewerProvider value={viewer.viewerContextValue}>
      <ObjectViewerBody viewer={viewer} props={props} />
    </ObjectViewerProvider>
  )
  const namespacedInner = stateNamespace ? (
    <StateNamespaceProvider namespace={stateNamespace}>
      {inner}
    </StateNamespaceProvider>
  ) : (
    inner
  )

  const level = (
    <ViewerBottomBarLevel
      viewer={viewer}
      barId={barId}
      barLabel={barLabel}
      onBreadcrumbClick={props.onBreadcrumbClick}
    >
      {standalone ? (
        <ViewerFrame>{namespacedInner}</ViewerFrame>
      ) : (
        namespacedInner
      )}
    </ViewerBottomBarLevel>
  )

  return standalone ? (
    <StandaloneViewerRoot>{level}</StandaloneViewerRoot>
  ) : (
    level
  )
}
