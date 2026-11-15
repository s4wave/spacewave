import { useState, useCallback, type ReactNode } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
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
import { ViewerStatus } from './ViewerStatus.js'
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

// ObjectView is what the viewer body shows of its object:
//
// - loading: nothing has answered yet, or the World waits on its parent.
// - ready: the object and its type arrived; object is its handle, or
//   undefined for a viewer that needs none.
// - reopening: the watch failed after answering and is reopening; the
//   content stays drawn under a notice that it may be out of date.
// - missing: the watch answered that the object is absent.
// - failed: reading the object failed; kept is what arrived before, drawn
//   under the failure, or null when nothing did.
// - worldFailed: the World the object lives in failed to open.
type ObjectView =
  | { state: 'loading' }
  | { state: 'ready'; object: IObjectState | undefined; typeID: string }
  | {
      state: 'reopening'
      object: IObjectState | undefined
      typeID: string
    }
  | { state: 'missing' }
  | {
      state: 'failed'
      error: Error
      retry: () => void
      kept: { object: IObjectState | undefined; typeID: string } | null
    }
  | { state: 'worldFailed'; error: Error; retry: () => void }

// objectView derives what the viewer shows from the World and the object's
// watch, in this order: the World, then the watch's failure, then the
// object's presence. Every World object key is watched, whatever viewer is
// selected, so the watch answers presence and failure for every viewer;
// requiresObjectState decides only whether the viewer also waits for the
// object's handle. Missing is shown only when the watch has answered that
// the object is absent, never for a failed or unanswered watch, and never
// for a source that names no World object, such as a UnixFS path, which has
// no object to be missing and shows its viewer as soon as its type is known.
function objectView(
  objectInfo: ObjectInfo,
  worldState: ObjectViewerProps['worldState'],
  viewer: UseObjectViewerResult,
): ObjectView {
  const { objectState, typeID } = viewer
  const object = objectState.value ?? undefined
  if (
    objectInfo?.info?.case !== 'worldObjectInfo' ||
    !objectInfo.info.value.objectKey
  ) {
    return typeID === undefined
      ? { state: 'loading' }
      : { state: 'ready', object, typeID }
  }

  // The World comes first: without it nothing about the object is known. A
  // World with no value and no error is waiting on its parent, or reopening
  // after its handle was released, so it is loading; only an error has a
  // Retry that can open it. No World here has settled empty: the containers
  // render a viewer only below a mounted SharedObject and body, through
  // SharedObjectRouteState's mounted branch on the Space route and
  // isSpaceMounting on the Display route, and the Space and World mounts
  // below them never settle to null.
  if (!worldState.value) {
    if (!worldState.error) return { state: 'loading' }
    return {
      state: 'worldFailed',
      error: worldState.error,
      retry: worldState.retry,
    }
  }

  // A viewer that reads the World itself needs only the type, not the handle.
  const needsHandle = viewer.selectedComponent?.requiresObjectState ?? true
  if (objectState.error) {
    const last = objectState.last ?? undefined
    return {
      state: 'failed',
      error: objectState.error,
      retry: objectState.retry,
      kept:
        typeID !== undefined && (last || !needsHandle)
          ? { object: last, typeID }
          : null,
    }
  }
  if (typeID !== undefined && (object || !needsHandle)) {
    return objectState.loading && objectState.reopening
      ? { state: 'reopening', object, typeID }
      : { state: 'ready', object, typeID }
  }
  if (objectState.loading || object) return { state: 'loading' }
  return { state: 'missing' }
}

// ObjectViewerBody draws the viewer's state: loading, the object, its
// reopening, missing or failed state, a World that failed to open, or the
// missing component fallback.
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
  const view = objectView(objectInfo, worldState, viewer)

  switch (view.state) {
    case 'loading':
      return <ObjectViewerLoadingState />
    case 'missing':
      return <ObjectViewerNotFoundState objectKey={barLabel} />
    case 'worldFailed':
      return (
        <div className="bg-background-primary viewer-pane flex h-full w-full flex-1 flex-col">
          <ViewerStatus
            state="failed"
            message={`Could not open the Space that holds ${barLabel}: ${view.error.message}`}
            retry={view.retry}
          />
        </div>
      )
  }

  // A notice drawn over content that arrived keeps that content mounted at
  // the same place in the tree, so it never remounts or blanks.
  const notice =
    view.state === 'failed' ? (
      <ViewerStatus
        state="failed"
        message={`Could not read ${barLabel}: ${view.error.message}`}
        retry={view.retry}
      />
    ) : view.state === 'reopening' ? (
      <ObjectViewerReopeningNotice />
    ) : null
  const shown = view.state === 'failed' ? view.kept : view
  if (!shown) {
    return (
      <div className="bg-background-primary viewer-pane flex h-full w-full flex-1 flex-col">
        {notice}
      </div>
    )
  }
  if (viewer.missingComponentID && renderMissingComponent) {
    return renderMissingComponent(
      viewer.missingComponentID,
      barLabel,
      shown.typeID,
    )
  }

  return (
    <>
      {notice}
      <HistoryRouter
        path={props.path ?? '/'}
        onNavigate={props.onNavigate ?? noopNavigate}
      >
        <ObjectViewerContent
          objectInfo={objectInfo}
          worldState={worldState}
          spaceContents={props.spaceContents}
          objectState={shown.object}
          typeID={shown.typeID}
          component={viewer.selectedComponent}
          availableComponents={viewer.visibleComponents}
          missingComponentID={viewer.missingComponentID}
          onSelectComponent={viewer.onSelectComponent}
          standalone={props.standalone}
        />
      </HistoryRouter>
    </>
  )
}

// ObjectViewerReopeningNotice says the content shown may be out of date while
// the object's watch reconnects.
function ObjectViewerReopeningNotice() {
  return (
    <p
      role="status"
      className="viewer-inset-x min-h-viewer-fact-row text-foreground-alt flex shrink-0 items-center text-xs"
    >
      Reconnecting. This view may be out of date.
    </p>
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
      menuLabel={barLabel}
      contextMenuLabel={viewer.contextMenuLabel}
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
