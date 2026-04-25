import React, {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'
import type { WatchOrganizationStateResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { SpaceSharingState, SpaceState } from '@s4wave/sdk/space/space.pb.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

// NavigateToObjectsFunc navigates to one or more objects in the space.
//
// This will be interpreted differently depending on the space state:
//  - Single object: navigate to it
//  - Single object while we are in a Layout: open in a new tab
//  - Multiple objects: start a temporary local ObjectLayout
//  - Multiple objects while in a Layout: create a new split with a TabSet.
export type NavigateToObjectsFunc = (objectKeys: string[]) => void

// BuildObjectUrlsFunc builds the URLs for a list of object keys.
export type BuildObjectUrlsFunc = (objectKeys: string[]) => string[]

export interface SpaceContainerContextValue {
  // spaceId is the space shared object id
  spaceId: string
  // spaceState is the space state
  spaceState: SpaceState
  // spaceSharingState is the combined sharing snapshot for the space
  spaceSharingState?: SpaceSharingState | null
  // orgState is the combined organization snapshot for the owning org, if any
  orgState?: WatchOrganizationStateResponse | null
  // spaceWorldResource is the world resource
  spaceWorldResource: Resource<EngineWorldState>
  // spaceWorld is the world instance
  spaceWorld: EngineWorldState
  // navigateToRoot navigates to the root of the space.
  navigateToRoot: () => void
  // navigateToObjects is the NavigateToObjectsFunc for the space.
  navigateToObjects: NavigateToObjectsFunc
  // buildObjectUrls builds urls for the given object keys.
  buildObjectUrls: BuildObjectUrlsFunc
  // objectKey is the current object key we are displaying (navigated to)
  objectKey?: string
  // objectPath is the sub-path from the URL after the /-/ delimiter
  objectPath?: string
  // navigateToSubPath navigates to a sub-path within the current space object
  navigateToSubPath: (subpath: string) => void
}

const Context = createContext<SpaceContainerContextValue | null>(null)

const Provider: React.FC<
  SpaceContainerContextValue & { children?: ReactNode }
> = ({
  children,
  buildObjectUrls,
  navigateToObjects,
  navigateToRoot,
  spaceId,
  spaceState,
  spaceSharingState,
  orgState,
  spaceWorldResource,
  spaceWorld,
  objectKey,
  objectPath,
  navigateToSubPath,
}) => {
  const contextValue: SpaceContainerContextValue = useMemo(
    () => ({
      buildObjectUrls,
      navigateToObjects,
      navigateToRoot,
      spaceId,
      spaceState,
      spaceSharingState,
      orgState,
      spaceWorldResource,
      spaceWorld,
      objectKey,
      objectPath,
      navigateToSubPath,
    }),
    [
      buildObjectUrls,
      navigateToObjects,
      navigateToRoot,
      spaceId,
      spaceState,
      spaceSharingState,
      orgState,
      spaceWorldResource,
      spaceWorld,
      objectKey,
      objectPath,
      navigateToSubPath,
    ],
  )

  return <Context.Provider value={contextValue}>{children}</Context.Provider>
}

const useSpaceContainerContext = (): SpaceContainerContextValue => {
  const context = useContext(Context)
  if (!context) {
    throw new Error(
      'SpaceContainer context not found. Wrap component in SpaceContainerContext.Provider.',
    )
  }
  return context
}

// useSpaceContainerContextSafe returns the context value or null if not available.
const useSpaceContainerContextSafe = (): SpaceContainerContextValue | null => {
  return useContext(Context)
}

// SpaceContainerContext provides space navigation and state to child components.
export const SpaceContainerContext = {
  Provider,
  useContext: useSpaceContainerContext,
  useContextSafe: useSpaceContainerContextSafe,
}
