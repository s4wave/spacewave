import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  TestbedResourceService,
  TestbedResourceServiceClient,
} from './testbed_srpc.pb.js'
import { Engine } from '../world/engine.js'
import { StateAtom } from '@aptre/bldr-sdk/state/state.js'

// TestbedRoot is the root resource for creating test world engines.
export class TestbedRoot extends Resource {
  // service is the testbed root resource service
  private service: TestbedResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new TestbedResourceServiceClient(resourceRef.client)
  }

  // createWorld creates a new world engine and returns an Engine resource.
  // The world engine remains active until the returned resource is released.
  // remember to call "release()" or use the "using" keyword with Resources.
  public async createWorld(
    engineId?: string,
    abortSignal?: AbortSignal,
  ): Promise<Engine> {
    const resp = await this.service.CreateWorld({ engineId }, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, Engine)
  }

  // markTestResult marks the test result (success or failure).
  // Used by test wrapper to signal test completion.
  public async markTestResult(
    success: boolean,
    errorMessage?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.MarkTestResult(
      { success, errorMessage: errorMessage ?? '' },
      abortSignal,
    )
  }

  // accessStateAtom accesses a persisted state atom resource.
  // Returns a StateAtom for Get/Set/Watch operations on persisted state.
  // If storeId is empty, uses the default "ui-state" store.
  public async accessStateAtom(
    storeId?: string,
    abortSignal?: AbortSignal,
  ): Promise<StateAtom> {
    const resp = await this.service.AccessStateAtom(
      { storeId: storeId ?? '' },
      abortSignal,
    )
    return this.resourceRef.createResource(resp.resourceId ?? 0, StateAtom)
  }
}
