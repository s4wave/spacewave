import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ObjectInfo } from './object.pb.js'

// ObjectViewerComponentProps are the props passed to object viewer components.
export interface ObjectViewerComponentProps {
  // objectInfo contains metadata about the object being viewed.
  objectInfo: ObjectInfo
  // worldState is the world state resource.
  worldState: Resource<IWorldState>
  // objectState is the world object state, undefined for standalone unixfs.
  objectState?: IObjectState
}

// getObjectKey extracts the objectKey from an ObjectInfo.
export function getObjectKey(info: ObjectInfo): string {
  if (info?.info?.case === 'worldObjectInfo') {
    return info.info.value.objectKey ?? ''
  }
  if (info?.info?.case === 'unixfsObjectInfo') {
    return info.info.value.unixfsId ?? ''
  }
  return ''
}

// getTypeID extracts the typeID from an ObjectInfo.
export function getTypeID(info: ObjectInfo): string {
  if (info?.info?.case === 'worldObjectInfo') {
    return info.info.value.objectType ?? ''
  }
  return 'unixfs/fs-node'
}

// ObjectViewerComponent describes a registered viewer component.
export interface ObjectViewerComponent {
  // typeID is the type identifier this component handles.
  typeID: string
  // name is the display name of the component.
  name: string
  // category is the grouping category for the component selector dropdown.
  category?: string
  // disablePadding removes the default 5px viewer content padding.
  disablePadding?: boolean
  // component is the React component to render.
  component: React.ComponentType<ObjectViewerComponentProps>
}
