import { Provider } from '../provider.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  LocalProviderResourceService,
  LocalProviderResourceServiceClient,
} from './local_srpc.pb.js'
import type { CreateAccountResponse } from './local.pb.js'

// LocalProvider wraps a Provider resource with local-provider-specific RPCs.
export class LocalProvider extends Provider {
  private localService: LocalProviderResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.localService = new LocalProviderResourceServiceClient(
      resourceRef.client,
    )
  }

  // createAccount creates a ProviderAccount and Session on the local provider.
  public async createAccount(
    abortSignal?: AbortSignal,
  ): Promise<CreateAccountResponse> {
    return await this.localService.CreateAccount({}, abortSignal)
  }
}
