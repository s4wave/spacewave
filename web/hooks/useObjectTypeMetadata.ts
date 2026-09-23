import { useMemo } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import { ObjectTypeRegistryResourceServiceClient } from '@s4wave/sdk/objecttype/registry/registry_srpc.pb.js'
import {
  WatchObjectTypesRequest,
  WatchObjectTypesResponse,
  ObjectTypeRegistration,
} from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import {
  buildObjectTypeMetadataMap,
  type ObjectTypeMetadataById,
} from '@s4wave/web/space/object-tree.js'
import { useDynamicRegistrations } from './useDynamicRegistrations.js'

const objectTypeCreateStream = (
  root: Root,
  req: WatchObjectTypesRequest,
  signal: AbortSignal,
) =>
  new ObjectTypeRegistryResourceServiceClient(root.client).WatchObjectTypes(
    req,
    signal,
  )

const objectTypeGetRegs = (resp: WatchObjectTypesResponse | null) =>
  resp?.registrations ?? []

export function useObjectTypeMetadata(
  rootResource: Resource<Root>,
  instanceKey = '',
): ObjectTypeMetadataById {
  const registrations = useDynamicRegistrations(
    rootResource.value,
    objectTypeCreateStream,
    { instanceKey },
    WatchObjectTypesRequest.equals,
    WatchObjectTypesResponse.equals,
    objectTypeGetRegs,
    keepObjectTypeRegistration,
    ObjectTypeRegistration.equals,
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
