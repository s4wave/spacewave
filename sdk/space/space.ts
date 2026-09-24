import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import {
  SpaceResourceService,
  SpaceResourceServiceClient,
} from './space_srpc.pb.js'
import { Engine } from '../world/engine.js'
import { EngineWorldState } from '../world/engine-state.js'
import { SpaceContents } from './contents.js'
import { PluginFrontend } from './plugin-frontend.js'
import {
  BuildSpacePluginRequest,
  BuildSpacePluginResponse,
  CreateSecretRequest,
  CreateSecretResponse,
  SpaceSharingState,
  SpaceState,
  WatchSpaceSharingStateRequest,
  WatchSpaceStateRequest,
} from './space.pb.js'

// Space is a World (Engine) wrapped into a SharedObject which contains objects.
//
// The objects are rendered by type-specific frontend components.
export class Space extends Resource {
  private service: SpaceResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SpaceResourceServiceClient(resourceRef.client)
  }

  // watchSpaceState watches the SpaceState for the component.
  public watchSpaceState(
    req?: WatchSpaceStateRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SpaceState> {
    return this.service.WatchSpaceState(req ?? {}, abortSignal)
  }

  // watchSpaceSharingState watches the sharing snapshot for the space.
  public watchSpaceSharingState(
    req?: WatchSpaceSharingStateRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SpaceSharingState> {
    return this.service.WatchSpaceSharingState(req ?? {}, abortSignal)
  }

  // accessWorld accesses the Engine associated with the space.
  // Returns an Engine resource, used for creating transactions.
  // For convenience methods that auto-manage transactions, use accessWorldState.
  public async accessWorld(abortSignal?: AbortSignal): Promise<Engine> {
    const response = await this.service.AccessWorld({}, abortSignal)
    return this.resourceRef.createResource(response.resourceId ?? 0, Engine)
  }

  // mountSpaceContents activates plugins for the space and returns a
  // sub-resource for monitoring plugin status.
  public async mountSpaceContents(
    abortSignal?: AbortSignal,
  ): Promise<SpaceContents> {
    const response = await this.service.MountSpaceContents({}, abortSignal)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      SpaceContents,
    )
  }

  // addSpacePlugin adds a plugin manifest ID to the Space settings plugin list.
  // The Space contents state re-projects to reflect the new plugin lifecycle.
  public async addSpacePlugin(
    pluginId: string,
    manifestKey?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.AddSpacePlugin({ pluginId, manifestKey }, abortSignal)
  }

  // removeSpacePlugin removes a plugin manifest ID from the Space settings
  // plugin list. The Space contents state re-projects after removal.
  public async removeSpacePlugin(
    pluginId: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.RemoveSpacePlugin({ pluginId }, abortSignal)
  }

  // buildSpacePlugin queues an immutable source snapshot on a registered device.
  // The returned Forge execution owns progress, cancellation, logs, and outputs.
  public async buildSpacePlugin(
    request: BuildSpacePluginRequest,
    abortSignal?: AbortSignal,
  ): Promise<BuildSpacePluginResponse> {
    return this.service.BuildSpacePlugin(request, abortSignal)
  }

  /** openPluginFrontend retains live source on the selected native device. */
  public async openPluginFrontend(
    request: BuildSpacePluginRequest,
    abortSignal?: AbortSignal,
  ): Promise<PluginFrontend> {
    const response = await this.service.OpenPluginFrontend(request, abortSignal)
    return new PluginFrontend(
      this.resourceRef.createRef(response.resourceId ?? 0),
      response.executionKey ?? '',
    )
  }

  // createSecret creates a redacted Secret world object plus its nested
  // payload SharedObject through the mounted Space resource.
  public async createSecret(
    request: CreateSecretRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateSecretResponse> {
    return await this.service.CreateSecret(request, abortSignal)
  }

  // accessWorldState accesses the Engine as a WorldState-like interface.
  // This automatically creates short-lived transactions for each operation.
  // This is the recommended API for most use cases.
  public async accessWorldState(
    write: boolean = true,
    abortSignal?: AbortSignal,
  ): Promise<EngineWorldState> {
    const engine = await this.accessWorld(abortSignal)
    return new EngineWorldState(engine, write, true)
  }
}
