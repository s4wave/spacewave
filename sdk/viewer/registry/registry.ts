import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  ViewerRegistryResourceService,
  ViewerRegistryResourceServiceClient,
} from './registry_srpc.pb.js'
import {
  RegisterViewerResponse,
  ViewerSurface,
  ListViewersResponse,
  WatchViewersResponse,
  ViewerRegistration,
} from './registry.pb.js'

// ViewerRegistry is a resource that provides viewer registration for plugins.
// Plugins register viewers via registerViewer and watch for changes via watchViewers.
export class ViewerRegistry extends Resource {
  // service is the viewer registry resource service
  private service: ViewerRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new ViewerRegistryResourceServiceClient(resourceRef.client)
  }

  // registerViewer registers a viewer for an object type.
  public async registerViewer(
    registration: ViewerRegistration,
    abortSignal?: AbortSignal,
  ): Promise<RegisterViewerResponse> {
    return await this.service.RegisterViewer({ registration }, abortSignal)
  }

  // listViewers returns registered viewers for a surface.
  public async listViewers(
    surface: ViewerSurface,
    abortSignal?: AbortSignal,
  ): Promise<ListViewersResponse> {
    return await this.service.ListViewers({ surface }, abortSignal)
  }

  // watchViewers streams viewer registration changes for a surface.
  public watchViewers(
    surface: ViewerSurface,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchViewersResponse> {
    return this.service.WatchViewers({ surface }, abortSignal)
  }
}
