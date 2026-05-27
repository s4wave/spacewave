import { useMemo } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import { ObjectTypeRegistryResourceServiceClient } from '@s4wave/sdk/objecttype/registry/registry_srpc.pb.js'
import {
  WatchObjectTypesRequest,
  WatchObjectTypesResponse,
  type ObjectTypeRegistration,
} from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import {
  buildObjectTypeMetadataMap,
  type ObjectTypeMetadataById,
} from '@s4wave/web/space/object-tree.js'
import { useDynamicRegistrations } from './useDynamicRegistrations.js'

const objectTypeCreateStream = (
  root: Root,
  _req: WatchObjectTypesRequest,
  signal: AbortSignal,
) =>
  new ObjectTypeRegistryResourceServiceClient(root.client).WatchObjectTypes(
    {},
    signal,
  )

const objectTypeGetRegs = (resp: WatchObjectTypesResponse | null) =>
  resp?.registrations ?? []

export function useObjectTypeMetadata(
  rootResource: Resource<Root>,
): ObjectTypeMetadataById {
  const registrations = useDynamicRegistrations(
    rootResource.value,
    objectTypeCreateStream,
    {},
    WatchObjectTypesRequest.equals,
    WatchObjectTypesResponse.equals,
    objectTypeGetRegs,
    keepObjectTypeRegistration,
  )
  return useMemo(
    () => buildObjectTypeMetadataMap(registrations),
    [registrations],
  )
}

function keepObjectTypeRegistration(
  registration: ObjectTypeRegistration,
): ObjectTypeRegistration | null {
  return registration.typeId ? registration : null
}
