import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  QuickstartRegistryResourceService,
  QuickstartRegistryResourceServiceClient,
} from './registry_srpc.pb.js'
import {
  ExecuteQuickstartRequest,
  ExecuteQuickstartResponse,
  ListQuickstartsResponse,
  QuickstartRegistration,
  RegisterQuickstartResponse,
  WatchQuickstartsResponse,
} from './registry.pb.js'

// QuickstartRegistry is a resource that provides quickstart registration for plugins.
export class QuickstartRegistry extends Resource {
  // service is the quickstart registry resource service.
  private service: QuickstartRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new QuickstartRegistryResourceServiceClient(
      resourceRef.client,
    )
  }

  // registerQuickstart registers a Quickstart from a plugin.
  public async registerQuickstart(
    registration: QuickstartRegistration,
    abortSignal?: AbortSignal,
  ): Promise<RegisterQuickstartResponse> {
    return await this.service.RegisterQuickstart({ registration }, abortSignal)
  }

  // listQuickstarts returns all registered Quickstarts.
  public async listQuickstarts(
    abortSignal?: AbortSignal,
  ): Promise<ListQuickstartsResponse> {
    return await this.service.ListQuickstarts({}, abortSignal)
  }

  // watchQuickstarts streams all registered Quickstarts.
  public watchQuickstarts(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchQuickstartsResponse> {
    return this.service.WatchQuickstarts({}, abortSignal)
  }

  // executeQuickstart runs a registered Quickstart against a mounted Space resource.
  public async executeQuickstart(
    request: ExecuteQuickstartRequest,
    abortSignal?: AbortSignal,
  ): Promise<ExecuteQuickstartResponse> {
    return await this.service.ExecuteQuickstart(request, abortSignal)
  }
}
