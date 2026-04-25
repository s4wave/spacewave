import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  ProviderResourceService,
  ProviderResourceServiceClient,
} from './provider_srpc.pb.js'
import { ProviderInfo } from '../../core/provider/provider.pb.js'
import { Account } from '../account/account.js'

// Provider is a provider resource that provides access to provider functionality.
export class Provider extends Resource {
  // service is the provider resource service
  private service: ProviderResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new ProviderResourceServiceClient(resourceRef.client)
  }

  // getProviderInfo returns information about this provider.
  public async getProviderInfo(
    abortSignal?: AbortSignal,
  ): Promise<ProviderInfo | null> {
    const resp = await this.service.GetProviderInfo({}, abortSignal)
    return resp?.providerInfo ?? null
  }

  // mountAccount mounts a provider account and returns an Account resource.
  public async mountAccount(
    accountId: string,
    abortSignal?: AbortSignal,
  ): Promise<Account> {
    const resp = await this.service.AccessProviderAccount(
      { accountId },
      abortSignal,
    )
    return this.resourceRef.createResource(resp.resourceId ?? 0, Account)
  }
}
