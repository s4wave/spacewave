import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  WorldOpRegistryResourceService,
  WorldOpRegistryResourceServiceClient,
} from './registry_srpc.pb.js'
import {
  RegisterWorldOpResponse,
  WatchWorldOpsResponse,
} from './registry.pb.js'

// WorldOpRegistry is a resource that provides world op registration for plugins.
// Plugins register world ops via registerWorldOp and watch for changes via watchWorldOps.
export class WorldOpRegistry extends Resource {
  // service is the world op registry resource service.
  private service: WorldOpRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new WorldOpRegistryResourceServiceClient(resourceRef.client)
  }

  // registerWorldOp registers a world op from a plugin.
  public async registerWorldOp(
    operationTypeId: string,
    pluginId: string,
    abortSignal?: AbortSignal,
  ): Promise<RegisterWorldOpResponse> {
    return await this.service.RegisterWorldOp(
      { operationTypeId, pluginId },
      abortSignal,
    )
  }

  // watchWorldOps streams all registered world ops.
  public watchWorldOps(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchWorldOpsResponse> {
    return this.service.WatchWorldOps({}, abortSignal)
  }
}
