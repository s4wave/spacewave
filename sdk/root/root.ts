import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Cdn } from '../cdn/cdn.js'
import { DebugDb } from '../debugdb/debugdb.js'
import { Provider } from '../provider/provider.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  RootResourceService,
  RootResourceServiceClient,
} from './root_srpc.pb.js'
import {
  AccessStateAtomRequest,
  GetSessionMetadataResponse,
  ListProvidersResponse,
  ListSessionsResponse,
  MountSessionRequest,
  MountSessionByIdxRequest,
  MountSessionByIdxResponse,
  WatchListenerStatusRequest,
  WatchListenerStatusResponse,
  WatchStateAtomsRequest,
  WatchStateAtomsResponse,
  WatchAllAccountStatusesRequest,
  WatchAllAccountStatusesResponse,
  WatchSessionMetadataRequest,
  WatchSessionMetadataResponse,
  WatchSessionsRequest,
  WatchSessionsResponse,
} from './root.pb.js'
import { Session } from '../session/session.js'
import { StateAtom } from '@aptre/bldr-sdk/state/state.js'
import type { Changelog } from '@s4wave/core/changelog/changelog.pb.js'
import type { EntityCredential } from '@s4wave/core/session/session.pb.js'
import {
  Hash,
  HashType,
} from '@go/github.com/s4wave/spacewave/net/hash/hash.pb.js'

// Root is the root resource of the spacewave sdk.
// This allows accessing all other resources from the top level.
//
// Remember to call "release()" or use the "using" keyword with Resources.
//   using myProvider = await root.lookupProvider("provider-id")
// This will automatically release() when the function returns (or scope exits).
export class Root extends Resource {
  // service is the root resource service
  private service: RootResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new RootResourceServiceClient(resourceRef.client)
  }

  // lookupProvider looks up a Provider resource by ID and returns the handle to it.
  public async lookupProvider(
    providerId: string,
    abortSignal?: AbortSignal,
  ): Promise<Provider> {
    const resp = await this.service.LookupProvider({ providerId }, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, Provider)
  }

  // mountSession mounts a session with a session ref.
  public async mountSession(
    request?: MountSessionRequest,
    abortSignal?: AbortSignal,
  ): Promise<Session> {
    const resp = await this.service.MountSession(request ?? {}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, Session)
  }

  // mountSessionByIdx mounts a session by index.
  public async mountSessionByIdx(
    request: MountSessionByIdxRequest,
    abortSignal?: AbortSignal,
  ): Promise<{
    session: Session
    sessionRef: MountSessionByIdxResponse['sessionRef']
  } | null> {
    const resp = await this.service.MountSessionByIdx(
      request ?? {},
      abortSignal,
    )
    if (resp.notFound) {
      return null
    }
    return {
      session: this.resourceRef.createResource(resp.resourceId ?? 0, Session),
      sessionRef: resp.sessionRef,
    }
  }

  // marshalHash marshals a Hash to a base58 string.
  public async marshalHash(
    hash: Hash | null | undefined,
    abortSignal?: AbortSignal,
  ): Promise<string> {
    const resp = await this.service.MarshalHash(
      { hash: hash ?? undefined },
      abortSignal,
    )
    return resp.hashStr ?? ''
  }

  // parseHash parses a Hash from a base58 string.
  public async parseHash(
    hashStr: string,
    abortSignal?: AbortSignal,
  ): Promise<Hash | null> {
    const resp = await this.service.ParseHash({ hashStr }, abortSignal)
    return resp.hash ?? null
  }

  // hashSum computes a hash of the given data with the specified hash type.
  public async hashSum(
    hashType: HashType,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<Hash | null> {
    const resp = await this.service.HashSum({ hashType, data }, abortSignal)
    return resp.hash ?? null
  }

  // hashValidate validates a hash object.
  // Returns { valid: true } if valid, or { valid: false, error: string } if invalid.
  public async hashValidate(
    hash: Hash | null | undefined,
    abortSignal?: AbortSignal,
  ): Promise<{ valid: boolean; error?: string }> {
    const resp = await this.service.HashValidate(
      { hash: hash ?? undefined },
      abortSignal,
    )
    return {
      valid: resp.valid ?? false,
      error: resp.error || undefined,
    }
  }

  // accessStateAtom accesses the global state atom resource.
  // The state atom provides persistent, cross-window synchronized state storage.
  public async accessStateAtom(
    request?: AccessStateAtomRequest,
    abortSignal?: AbortSignal,
  ): Promise<StateAtom> {
    const resp = await this.service.AccessStateAtom(request ?? {}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, StateAtom)
  }

  // watchStateAtoms streams the known root state atom store ids on change.
  public watchStateAtoms(
    request?: WatchStateAtomsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchStateAtomsResponse> {
    return this.service.WatchStateAtoms(request ?? {}, abortSignal)
  }

  // listProviders lists the available providers.
  public async listProviders(
    abortSignal?: AbortSignal,
  ): Promise<ListProvidersResponse> {
    return await this.service.ListProviders({}, abortSignal)
  }

  // listSessions lists the configured sessions.
  public async listSessions(
    abortSignal?: AbortSignal,
  ): Promise<ListSessionsResponse> {
    return await this.service.ListSessions({}, abortSignal)
  }

  // watchSessions streams the session list, updating when sessions change.
  public watchSessions(
    req?: WatchSessionsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSessionsResponse> {
    return this.service.WatchSessions(req ?? {}, abortSignal)
  }

  // watchAllAccountStatuses streams provider account status by session index.
  public watchAllAccountStatuses(
    req?: WatchAllAccountStatusesRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchAllAccountStatusesResponse> {
    return this.service.WatchAllAccountStatuses(req ?? {}, abortSignal)
  }

  // getSessionMetadata returns metadata for a session by index.
  // Does not require the session to be mounted.
  public async getSessionMetadata(
    sessionIdx: number,
    abortSignal?: AbortSignal,
  ): Promise<GetSessionMetadataResponse> {
    return await this.service.GetSessionMetadata({ sessionIdx }, abortSignal)
  }

  // watchSessionMetadata streams metadata for a session by index.
  public watchSessionMetadata(
    request: WatchSessionMetadataRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSessionMetadataResponse> {
    return this.service.WatchSessionMetadata(request, abortSignal)
  }

  // watchListenerStatus streams the desktop resource listener status:
  // effective socket path, whether the listener is currently bound,
  // and the count of connected resource clients.
  public watchListenerStatus(
    request?: WatchListenerStatusRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchListenerStatusResponse> {
    return this.service.WatchListenerStatus(request ?? {}, abortSignal)
  }

  // unlockSession unlocks a PIN-locked session before mounting.
  // The PIN is used to decrypt the session key and unblock the session tracker.
  public async unlockSession(
    sessionIdx: number,
    pin: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.UnlockSession({ sessionIdx, pin }, abortSignal)
  }

  // deleteSession removes a session from the local session list by index.
  public async deleteSession(
    sessionIdx: number,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.DeleteSession({ sessionIdx }, abortSignal)
  }

  // resetSession resets a PIN-locked session via entity key verification.
  public async resetSession(
    sessionIdx: number,
    credential: EntityCredential,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.ResetSession({ sessionIdx, credential }, abortSignal)
  }

  // getChangelog returns the application changelog.
  public async getChangelog(abortSignal?: AbortSignal): Promise<Changelog> {
    const resp = await this.service.GetChangelog({}, abortSignal)
    return resp.changelog ?? {}
  }

  // getDebugDb returns a DebugDb resource for storage diagnostics and benchmarking.
  public async getDebugDb(abortSignal?: AbortSignal): Promise<DebugDb> {
    const resp = await this.service.GetDebugDb({}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, DebugDb)
  }

  // getCdn mounts a process-scoped CDN resource for the selected CDN
  // instance. Empty cdnId selects the default CDN. Unknown ids fail with a
  // wrapped ErrUnknownCdn error. Returns the Cdn resource handle plus the
  // CDN Space ULID so callers do not need a follow-up getCdnSpaceId call.
  public async getCdn(
    cdnId: string = '',
    abortSignal?: AbortSignal,
  ): Promise<{ cdn: Cdn; cdnSpaceId: string }> {
    const resp = await this.service.GetCdn({ cdnId }, abortSignal)
    const cdn = this.resourceRef.createResource(resp.resourceId ?? 0, Cdn)
    return { cdn, cdnSpaceId: resp.cdnSpaceId ?? '' }
  }
}
