import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type {
  AddLocalEntityKeypairRequest,
  AddLocalEntityKeypairResponse,
  RemoveLocalEntityKeypairRequest,
  RemoveLocalEntityKeypairResponse,
  SetLocalDisplayNameRequest,
  SetLocalDisplayNameResponse,
  WatchLocalDisplayNameRequest,
  WatchLocalDisplayNameResponse,
  WatchLocalEntityKeypairsRequest,
  WatchLocalEntityKeypairsResponse,
} from './local-session.pb.js'
import {
  LocalSessionResourceService,
  LocalSessionResourceServiceClient,
} from './local-session_srpc.pb.js'
import type {
  ExportBackupKeyRequest,
  ExportBackupKeyResponse,
} from './session.pb.js'

// LocalSession wraps the LocalSessionResourceService SRPC client.
// Access via session.localProvider on a local session.
export class LocalSession extends Resource {
  private service: LocalSessionResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new LocalSessionResourceServiceClient(resourceRef.client)
  }

  // exportBackupKey exports the session entity private key as a PEM file.
  public async exportBackupKey(
    request: ExportBackupKeyRequest,
    abortSignal?: AbortSignal,
  ): Promise<ExportBackupKeyResponse> {
    return await this.service.ExportBackupKey(request, abortSignal)
  }

  // addEntityKeypair derives an entity key from a credential and adds it to AccountSettings.
  public async addEntityKeypair(
    request: AddLocalEntityKeypairRequest,
    abortSignal?: AbortSignal,
  ): Promise<AddLocalEntityKeypairResponse> {
    return await this.service.AddEntityKeypair(request, abortSignal)
  }

  // removeEntityKeypair removes an entity keypair from AccountSettings.
  public async removeEntityKeypair(
    request: RemoveLocalEntityKeypairRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveLocalEntityKeypairResponse> {
    return await this.service.RemoveEntityKeypair(request, abortSignal)
  }

  // watchEntityKeypairs streams entity keypairs from the AccountSettings SO.
  public watchEntityKeypairs(
    req?: WatchLocalEntityKeypairsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchLocalEntityKeypairsResponse> {
    return this.service.WatchEntityKeypairs(req ?? {}, abortSignal)
  }

  // setDisplayName updates the local provider account display name.
  public async setDisplayName(
    request: SetLocalDisplayNameRequest,
    abortSignal?: AbortSignal,
  ): Promise<SetLocalDisplayNameResponse> {
    return await this.service.SetDisplayName(request, abortSignal)
  }

  // watchDisplayName streams the local provider account display name.
  public watchDisplayName(
    request?: WatchLocalDisplayNameRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchLocalDisplayNameResponse> {
    return this.service.WatchDisplayName(request ?? {}, abortSignal)
  }
}
