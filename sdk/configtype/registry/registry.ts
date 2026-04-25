import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  ConfigTypeRegistryResourceService,
  ConfigTypeRegistryResourceServiceClient,
} from './registry_srpc.pb.js'
import {
  RegisterConfigTypeResponse,
  WatchConfigTypesResponse,
} from './registry.pb.js'

// ConfigTypeRegistry is a resource that provides config type registration for plugins.
// Plugins register config type editors via registerConfigType and watch for changes via watchConfigTypes.
export class ConfigTypeRegistry extends Resource {
  // service is the config type registry resource service.
  private service: ConfigTypeRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new ConfigTypeRegistryResourceServiceClient(
      resourceRef.client,
    )
  }

  // registerConfigType registers a config type editor from a plugin.
  public async registerConfigType(
    configId: string,
    pluginId: string,
    displayName: string,
    scriptPath: string,
    category?: string,
    abortSignal?: AbortSignal,
  ): Promise<RegisterConfigTypeResponse> {
    return await this.service.RegisterConfigType(
      { configId, pluginId, displayName, scriptPath, category: category ?? '' },
      abortSignal,
    )
  }

  // watchConfigTypes streams all registered config types.
  public watchConfigTypes(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchConfigTypesResponse> {
    return this.service.WatchConfigTypes({}, abortSignal)
  }
}
