import { createContext, useContext, useMemo } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import {
  WatchViewersRequest,
  WatchViewersResponse,
  type ViewerRegistration,
} from '@s4wave/sdk/viewer/registry/registry.pb.js'
import React from 'react'

import { useDynamicRegistrations } from './useDynamicRegistrations.js'

const emptyViewers: ObjectViewerComponent[] = []

const ViewerRegistryContext =
  createContext<ObjectViewerComponent[]>(emptyViewers)

// ViewerRegistryProviderProps are the props for ViewerRegistryProvider.
interface ViewerRegistryProviderProps {
  staticViewers: ObjectViewerComponent[]
  children: React.ReactNode
}

// ViewerRegistryProvider supplies a static viewer list via context.
// Wrap the app with this provider so useAllViewers() can access the
// static viewers without a direct import of the viewer array module.
export function ViewerRegistryProvider({
  staticViewers,
  children,
}: ViewerRegistryProviderProps) {
  return (
    <ViewerRegistryContext.Provider value={staticViewers}>
      {children}
    </ViewerRegistryContext.Provider>
  )
}

// useStaticViewers returns the static viewers from ViewerRegistryProvider context.
export function useStaticViewers(): ObjectViewerComponent[] {
  return useContext(ViewerRegistryContext)
}

// useAllViewers returns all viewers: static from context + dynamic from RPC.
export function useAllViewers(
  rootResource: Resource<Root>,
): ObjectViewerComponent[] {
  const staticViewers = useStaticViewers()
  const dynamicViewers = useDynamicViewers(rootResource)
  return useMemo(
    () => [...staticViewers, ...dynamicViewers],
    [staticViewers, dynamicViewers],
  )
}

const viewerCreateStream = (
  root: Root,
  _req: WatchViewersRequest,
  signal: AbortSignal,
) =>
  new ViewerRegistryResourceServiceClient(root.client).WatchViewers({}, signal)

const viewerGetRegs = (resp: WatchViewersResponse | null) =>
  resp?.registrations ?? []

// useDynamicViewers subscribes to the ViewerRegistry RPC and returns
// dynamically registered ObjectViewerComponent entries.
function useDynamicViewers(
  rootResource: Resource<Root>,
): ObjectViewerComponent[] {
  return useDynamicRegistrations(
    rootResource.value,
    viewerCreateStream,
    {},
    WatchViewersRequest.equals,
    WatchViewersResponse.equals,
    viewerGetRegs,
    registrationToViewer,
  )
}

// getViewersForType filters a viewer list by type ID, returning type-specific
// viewers followed by prefix-matched viewers, then wildcard viewers. An empty
// typeID is a valid input (objects exist in the world graph without a type
// quad); only wildcard viewers match it.
export function getViewersForType(
  typeID: string,
  viewers: ObjectViewerComponent[],
): ObjectViewerComponent[] {
  const exact: ObjectViewerComponent[] = []
  const prefix: ObjectViewerComponent[] = []
  const wildcard: ObjectViewerComponent[] = []
  for (const v of viewers) {
    if (typeID && v.typeID === typeID) {
      exact.push(v)
    } else if (v.typeID === '*') {
      wildcard.push(v)
    } else if (
      typeID &&
      v.typeID.endsWith('/*') &&
      typeID.startsWith(v.typeID.slice(0, -1))
    ) {
      prefix.push(v)
    }
  }
  return [...exact, ...prefix, ...wildcard]
}

// registrationToViewer converts a ViewerRegistration to an ObjectViewerComponent.
// Uses React.lazy to dynamically load the viewer module from the script path.
function registrationToViewer(
  reg: ViewerRegistration,
): ObjectViewerComponent | null {
  const typeId = reg.typeId
  const scriptPath = reg.scriptPath
  if (!typeId || !scriptPath) return null

  return {
    typeID: typeId,
    name: reg.viewerName || typeId,
    category: reg.category || undefined,
    component: React.lazy(() => import(/* @vite-ignore */ scriptPath)),
  }
}
