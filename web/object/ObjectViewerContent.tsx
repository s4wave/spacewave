import { Suspense, useCallback } from 'react'
import { WebViewErrorBoundary } from '@aptre/bldr-react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import {
  SpacePluginLifecycleState,
  type SpaceContentsState,
} from '@s4wave/sdk/space/space.pb.js'

import type {
  ObjectViewerComponent,
  ObjectViewerComponentProps,
} from './object.js'
import { getObjectKey } from './object.js'
import type { ObjectInfo } from './object.pb.js'
import { ObjectViewerLoadingState } from './ObjectViewerLoadingState.js'
import { debugViewerComponentID } from './useObjectViewer.js'

interface ObjectViewerContentProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  objectState?: IObjectState
  spaceContents?: Resource<SpaceContents>
  typeID: string
  component?: ObjectViewerComponent
  availableComponents?: ObjectViewerComponent[]
  missingComponentID?: string
  onSelectComponent?: (component: ObjectViewerComponent) => void
  standalone?: boolean
}

// noSpaceContents stands in for the Space contents of a viewer outside a Space.
const noSpaceContents: Resource<SpaceContents> = {
  value: null,
  loading: false,
  error: null,
  retry: () => {},
}

// pluginsSettled reports whether no Space plugin runtime is still starting, so
// a viewer it registers would already be listed.
function pluginsSettled(state: SpaceContentsState | null): boolean {
  if (!state?.ready) {
    return false
  }
  return !(state.plugins ?? []).some(
    (plugin) =>
      plugin.state ===
      SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING,
  )
}

// MissingViewerCard explains why no viewer is mounted and offers the raw object
// viewer when it is installed. While a Space plugin is still loading, it shows
// the loading state, since that plugin may register the viewer.
function MissingViewerCard({
  objectInfo,
  spaceContents,
  typeID,
  availableComponents,
  missingComponentID,
  onSelectComponent,
}: Pick<
  ObjectViewerContentProps,
  | 'objectInfo'
  | 'spaceContents'
  | 'typeID'
  | 'availableComponents'
  | 'missingComponentID'
  | 'onSelectComponent'
>) {
  const contentsState = useStreamingResource(
    spaceContents ?? noSpaceContents,
    useCallback(
      (contents: SpaceContents, signal: AbortSignal) =>
        contents.watchState({}, signal),
      [],
    ),
    [],
  )
  if (
    spaceContents &&
    !contentsState.error &&
    !pluginsSettled(contentsState.value)
  ) {
    return (
      <ObjectViewerLoadingState
        title="Loading plugins"
        detail="Waiting for the Space plugins that open this object."
      />
    )
  }

  const objectKey = getObjectKey(objectInfo)
  const debugComponent = availableComponents?.find(
    (candidate) => candidate.componentID === debugViewerComponentID,
  )

  let body
  if (!objectKey) {
    body = <span>No object selected</span>
  } else if (!typeID) {
    body = <span>Object has no type</span>
  } else {
    body = (
      <div className="flex w-full max-w-lg flex-col gap-4 text-left">
        <div className="text-center">
          <h2 className="text-foreground text-sm font-semibold">
            Can't open this object yet
          </h2>
          <p className="mt-1 text-xs leading-relaxed">
            No installed viewer handles this object type. You can inspect its
            object record when the raw viewer is available.
          </p>
        </div>
        <InfoCard title="About this object">
          <div className="grid gap-3 sm:grid-cols-2">
            <CopyableField label="Object key" value={objectKey} />
            <CopyableField label="Object type" value={typeID} />
          </div>
          {missingComponentID && missingComponentID !== typeID ? (
            <p className="text-foreground-alt/60 mt-3 text-xs leading-relaxed">
              Requested viewer unavailable:{' '}
              <span className="font-mono">{missingComponentID}</span>
            </p>
          ) : null}
        </InfoCard>
        {debugComponent ? (
          <div className="flex justify-center">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onSelectComponent?.(debugComponent)}
            >
              Open raw object
            </Button>
          </div>
        ) : (
          <p className="text-center text-xs leading-relaxed">
            No raw object viewer is installed.
          </p>
        )}
      </div>
    )
  }

  return (
    <div className="text-muted-foreground flex h-full items-center justify-center p-3">
      <div className="bg-background-dark border-border flex h-full w-full items-center justify-center rounded-xl border p-6 text-center">
        {body}
      </div>
    </div>
  )
}

// MountedViewer mounts the selected viewer component with the object it shows,
// in a slot that is the size container named pane. A viewer lays out by the
// pane's width and height through the @pane-* variants, since it sits in a grid
// cell the person resizes on both axes. The slot fills its parent, which must
// give it a definite height: a size container ignores its content's size.
function MountedViewer({
  component,
  standalone,
  ...props
}: ObjectViewerComponentProps & {
  component: ObjectViewerComponent
  standalone?: boolean
}) {
  const Component = component.component
  const disablePadding = standalone || component.disablePadding === true

  return (
    <div
      className={cn(
        'viewer-pane flex h-full w-full flex-col overflow-hidden',
        !disablePadding && 'p-1.25',
      )}
    >
      <WebViewErrorBoundary>
        <Suspense fallback={<ObjectViewerLoadingState />}>
          <Component {...props} />
        </Suspense>
      </WebViewErrorBoundary>
    </div>
  )
}

// ObjectViewerContent mounts the selected viewer, or explains that none is
// available for the object.
export function ObjectViewerContent({
  objectInfo,
  worldState,
  objectState,
  spaceContents,
  typeID,
  component,
  availableComponents,
  missingComponentID,
  onSelectComponent,
  standalone,
}: ObjectViewerContentProps) {
  if (!component) {
    return (
      <MissingViewerCard
        objectInfo={objectInfo}
        spaceContents={spaceContents}
        typeID={typeID}
        availableComponents={availableComponents}
        missingComponentID={missingComponentID}
        onSelectComponent={onSelectComponent}
      />
    )
  }

  return (
    <MountedViewer
      component={component}
      standalone={standalone}
      objectInfo={objectInfo}
      worldState={worldState}
      objectState={objectState}
      spaceContents={spaceContents}
    />
  )
}
