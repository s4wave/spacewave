import { Suspense } from 'react'
import { WebViewErrorBoundary } from '@aptre/bldr-react'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
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
            <div className="flex w-full max-w-lg flex-col gap-4 text-left">
              <div className="text-center">
                <h2 className="text-foreground text-sm font-semibold">
                  Can't open this object yet
                </h2>
                <p className="mt-1 text-xs leading-relaxed">
                  No installed viewer handles this object type. You can inspect
                  its object record when the raw viewer is available.
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
                    onClick={handleOpenDebugViewer}
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
