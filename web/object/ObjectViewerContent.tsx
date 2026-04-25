import { cn } from '@s4wave/web/style/utils.js'
import type {
  ObjectViewerComponent,
  ObjectViewerComponentProps,
} from './object.js'
import { getObjectKey } from './object.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ObjectInfo } from './object.pb.js'

interface ObjectViewerContentProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  objectState?: IObjectState
  typeID: string
  component?: ObjectViewerComponent
  standalone?: boolean
}

export function ObjectViewerContent({
  objectInfo,
  worldState,
  objectState,
  typeID,
  component,
  standalone,
}: ObjectViewerContentProps) {
  if (!component) {
    const objectKey = getObjectKey(objectInfo)
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center p-3">
        <div className="bg-background-dark border-border flex h-full w-full items-center justify-center rounded-xl border">
          {!objectKey ?
            <span>No object selected</span>
          : !typeID ?
            <span>Object has no type</span>
          : <span>No viewer available for type: {typeID}</span>}
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
  }

  return (
    <div
      className={cn(
        'flex h-full w-full flex-col overflow-hidden',
        !disablePadding && 'p-[5px]',
      )}
    >
      <Component {...props} />
    </div>
  )
}
