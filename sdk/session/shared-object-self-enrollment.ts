import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import type {
  SkipSharedObjectSelfEnrollmentResponse,
  StartSharedObjectSelfEnrollmentResponse,
  WatchSharedObjectSelfEnrollmentStateResponse,
} from './shared-object-self-enrollment.pb.js'
import {
  SharedObjectSelfEnrollmentResourceServiceClient,
  type SharedObjectSelfEnrollmentResourceService,
} from './shared-object-self-enrollment_srpc.pb.js'

// SharedObjectSelfEnrollment wraps post-sign-in self-enrollment state.
export class SharedObjectSelfEnrollment extends Resource {
  private service: SharedObjectSelfEnrollmentResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SharedObjectSelfEnrollmentResourceServiceClient(
      resourceRef.client,
    )
  }

  // watchState streams self-enrollment state changes.
  public watchState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSharedObjectSelfEnrollmentStateResponse> {
    return this.service.WatchState({}, abortSignal)
  }

  // start runs self-enrollment for the current pending set.
  public async start(
    abortSignal?: AbortSignal,
  ): Promise<StartSharedObjectSelfEnrollmentResponse> {
    return await this.service.Start({}, abortSignal)
  }

  // skip records the user's skip choice for the given generation.
  public async skip(
    generationKey: string,
    abortSignal?: AbortSignal,
  ): Promise<SkipSharedObjectSelfEnrollmentResponse> {
    return await this.service.Skip({ generationKey }, abortSignal)
  }
}
