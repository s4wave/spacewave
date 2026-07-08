import { Suspense } from 'react'
import { WebViewErrorBoundary } from '@aptre/bldr-react'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'

import type {
  ObjectViewerComponent,
  ObjectViewerComponentProps,
} from './object.js'
import { getObjectKey } from './object.js'
import type { ObjectInfo } from './object.pb.js'
import { ObjectViewerLoadingState } from './ObjectViewerLoadingState.js'

const debugViewerComponentID = 'spacewave.debug.viewer'
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
    const objectKey = getObjectKey(objectInfo)
    const debugComponent = availableComponents?.find(
      (candidate) => candidate.componentID === debugViewerComponentID,
    )
    const handleOpenDebugViewer = debugComponent
      ? () => onSelectComponent?.(debugComponent)
      : undefined

    return (
      <div className="text-muted-foreground flex h-full items-center justify-center p-3">
        <div className="bg-background-dark border-border flex h-full w-full items-center justify-center rounded-xl border p-6 text-center">
          {!objectKey ? (
            <span>No object selected</span>
          ) : !typeID ? (
            <span>Object has no type</span>
          ) : (
            <div className="flex max-w-md flex-col items-center gap-3">
              <div>
                <h2 className="text-foreground text-sm font-semibold">
                  Can't open this object yet
                </h2>
                <p className="mt-1 text-xs">
                  No installed viewer handles this object type.
                </p>
              </div>
              <div className="border-border bg-background/60 text-foreground rounded-md border px-3 py-2 font-mono text-xs">
                {typeID}
              </div>
              {missingComponentID && missingComponentID !== typeID ? (
                <p className="text-xs">
                  The requested viewer is not available: {missingComponentID}
                </p>
              ) : null}
              {debugComponent ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleOpenDebugViewer}
                >
                  Open Debug Viewer
                </Button>
              ) : null}
            </div>
          )}
        </div>
      </div>
    )
  }

  const Component = component.component
  const disablePadding = standalone || component.disablePadding === true
  const props: ObjectViewerComponentProps = {
    objectInfo,
    worldState,
    objectState,
    spaceContents,
  }

  return (
    <div
      className={cn(
        'flex h-full w-full flex-col overflow-hidden',
        !disablePadding && 'p-[5px]',
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
