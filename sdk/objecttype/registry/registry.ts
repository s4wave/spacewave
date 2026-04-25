import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  ObjectTypeRegistryResourceService,
  ObjectTypeRegistryResourceServiceClient,
} from './registry_srpc.pb.js'
import {
  RegisterObjectTypeResponse,
  WatchObjectTypesResponse,
} from './registry.pb.js'

// ObjectTypeRegistry is a resource that provides object type registration for plugins.
// Plugins register object types via registerObjectType and watch for changes via watchObjectTypes.
export class ObjectTypeRegistry extends Resource {
  // service is the object type registry resource service.
  private service: ObjectTypeRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new ObjectTypeRegistryResourceServiceClient(
      resourceRef.client,
    )
  }

  // registerObjectType registers an object type from a plugin.
  public async registerObjectType(
    typeId: string,
    pluginId: string,
    abortSignal?: AbortSignal,
  ): Promise<RegisterObjectTypeResponse> {
    return await this.service.RegisterObjectType(
      { typeId, pluginId },
      abortSignal,
    )
  }

  // watchObjectTypes streams all registered object types.
  public watchObjectTypes(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchObjectTypesResponse> {
    return this.service.WatchObjectTypes({}, abortSignal)
  }
}
