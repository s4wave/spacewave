import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import {
  SpaceResourceService,
  SpaceResourceServiceClient,
} from './space_srpc.pb.js'
import { Engine } from '../world/engine.js'
import { EngineWorldState } from '../world/engine-state.js'
import { SpaceContents } from './contents.js'
import {
  ObjectWizardRegistryResourceService,
  ObjectWizardRegistryResourceServiceClient,
} from '../world/wizard/wizard_srpc.pb.js'
import type { ObjectWizard } from '../world/wizard/wizard.pb.js'
import {
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
  private wizardService: ObjectWizardRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SpaceResourceServiceClient(resourceRef.client)
    this.wizardService = new ObjectWizardRegistryResourceServiceClient(
      resourceRef.client,
    )
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

  // accessWorldState accesses the Engine as a WorldState-like interface.
  // This automatically creates short-lived transactions for each operation.
  // This is the recommended API for most use cases.
  public async accessWorldState(
    write: boolean = true,
    abortSignal?: AbortSignal,
  ): Promise<EngineWorldState> {
    const engine = await this.accessWorld(abortSignal)
    return new EngineWorldState(engine, write)
  }

  // listWizards returns all registered object creation wizards.
  public async listWizards(abortSignal?: AbortSignal): Promise<ObjectWizard[]> {
    const response = await this.wizardService.ListWizards({}, abortSignal)
    return response.wizards ?? []
  }
}
