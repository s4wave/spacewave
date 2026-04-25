import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  SessionResourceService,
  SessionResourceServiceClient,
} from './session_srpc.pb.js'
import {
  AcceptLocalPairingAnswerResponse,
  AcceptLocalPairingOfferResponse,
  AccessSessionStateAtomRequest,
  CreateLocalPairingOfferResponse,
  CreateSpaceInviteResponse,
  CreateSpaceRequest,
  CreateSpaceResponse,
  GetSessionInfoResponse,
  GetTransferInventoryResponse,
  GetTransferStatusResponse,
  JoinSpaceViaInviteResponse,
  ListSpaceInvitesResponse,
  ListSpaceParticipantsResponse,
  RemoveSpaceParticipantResponse,
  RenameSpaceRequest,
  MountSharedObjectRequest,
  RevokeSpaceInviteResponse,
  StartTransferRequest,
  WatchLockStateRequest,
  WatchLockStateResponse,
  WatchPairedDevicesResponse,
  WatchPairingStatusResponse,
  WatchResourcesListRequest,
  WatchResourcesListResponse,
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
  WatchSyncStatusRequest,
  WatchSyncStatusResponse,
  WatchSessionStateAtomsRequest,
  WatchSessionStateAtomsResponse,
  WatchTransferProgressResponse,
} from './session.pb.js'
import { SessionLockMode } from '../../core/session/session.pb.js'
import type {
  SOInviteMessage,
  SOParticipantRole,
} from '../../core/sobject/sobject.pb.js'
import { SharedObject } from '../sobject/sobject.js'
import { SystemStatus } from '../status/status.js'
import { LocalSession } from './local-session.js'
import { SpacewaveSession } from './spacewave-session.js'
import { StateAtom } from '@aptre/bldr-sdk/state/state.js'

// Session is a session resource that provides access to session functionality.
//
// The MountSession directive will remain active until this resource is released.
export class Session extends Resource {
  // service is the session resource service
  private service: SessionResourceService
  // _spacewave is the lazy-initialized SpacewaveSession wrapper.
  private _spacewave: SpacewaveSession | null = null
  // _localProvider is the lazy-initialized LocalSession wrapper.
  private _localProvider: LocalSession | null = null
  // _systemStatus is the lazy-initialized SystemStatus wrapper.
  private _systemStatus: SystemStatus | null = null

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SessionResourceServiceClient(resourceRef.client)
  }

  // spacewave returns the SpacewaveSession wrapper for session-scoped spacewave RPCs.
  // Both SessionResourceService and SpacewaveSessionResourceService are on the
  // same mux, so the same resourceRef works for both.
  public get spacewave(): SpacewaveSession {
    if (!this._spacewave) {
      this._spacewave = new SpacewaveSession(this.resourceRef)
    }
    return this._spacewave
  }

  // systemStatus returns the SystemStatus wrapper for system status Watch RPCs.
  // SystemStatusService is on the same session mux, so the same resourceRef works.
  public get systemStatus(): SystemStatus {
    if (!this._systemStatus) {
      this._systemStatus = new SystemStatus(this.resourceRef)
    }
    return this._systemStatus
  }

  // getSessionInfo returns information about this session.
  public async getSessionInfo(
    abortSignal?: AbortSignal,
  ): Promise<GetSessionInfoResponse> {
    return await this.service.GetSessionInfo({}, abortSignal)
  }

  // createSpace creates a new Space as a SharedObject within the Session.
  public async createSpace(
    req: CreateSpaceRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateSpaceResponse> {
    return await this.service.CreateSpace(req, abortSignal)
  }

  // watchResourcesList returns a stream of the full spaces list snapshots.
  public watchResourcesList(
    req?: WatchResourcesListRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchResourcesListResponse> {
    return this.service.WatchResourcesList(req ?? {}, abortSignal)
  }

  // watchSharedObjectHealth streams SharedObject health by SharedObject ID.
  public watchSharedObjectHealth(
    req: WatchSharedObjectHealthRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSharedObjectHealthResponse> {
    return this.service.WatchSharedObjectHealth(req, abortSignal)
  }

  // watchSyncStatus streams session sync status snapshots.
  public watchSyncStatus(
    req?: WatchSyncStatusRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSyncStatusResponse> {
    return this.service.WatchSyncStatus(req ?? {}, abortSignal)
  }

  // mountSharedObject mounts a shared object and returns the SharedObject resource.
  public async mountSharedObject(
    req: MountSharedObjectRequest,
    abortSignal?: AbortSignal,
  ): Promise<SharedObject> {
    const resp = await this.service.MountSharedObject(req, abortSignal)
    const { resourceId, ...meta } = resp
    return this.resourceRef.createResource(resourceId ?? 0, SharedObject, meta)
  }

  // watchLockState streams the current lock state and updates on changes.
  public watchLockState(
    req?: WatchLockStateRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchLockStateResponse> {
    return this.service.WatchLockState(req ?? {}, abortSignal)
  }

  // unlockSession unlocks a PIN-locked session with the given PIN.
  public async unlockSession(
    pin: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.UnlockSession({ pin }, abortSignal)
  }

  // setLockMode changes the session lock mode.
  public async setLockMode(
    mode: SessionLockMode,
    pin?: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetLockMode({ mode, pin }, abortSignal)
  }

  // lockSession locks a running session, scrubbing the privkey and
  // requiring PIN re-entry. Only works when PIN mode is configured.
  public async lockSession(abortSignal?: AbortSignal): Promise<void> {
    await this.service.LockSession({}, abortSignal)
  }

  // generatePairingCode creates a pairing code for P2P device linking.
  public async generatePairingCode(abortSignal?: AbortSignal): Promise<string> {
    const resp = await this.service.GeneratePairingCode({}, abortSignal)
    return resp.code ?? ''
  }

  // completePairing resolves a pairing code to link a remote session.
  public async completePairing(
    code: string,
    abortSignal?: AbortSignal,
  ): Promise<string> {
    const resp = await this.service.CompletePairing({ code }, abortSignal)
    return resp.remotePeerId ?? ''
  }

  // getSASEmoji derives the SAS emoji verification sequence for a remote peer.
  public async getSASEmoji(
    remotePeerId: string,
    abortSignal?: AbortSignal,
  ): Promise<string[]> {
    const resp = await this.service.GetSASEmoji({ remotePeerId }, abortSignal)
    return resp.emoji ?? []
  }

  // confirmSASMatch sends the user's SAS emoji verification decision to the
  // bilateral confirmation exchange running over the bifrost link.
  public async confirmSASMatch(
    confirmed: boolean,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.ConfirmSASMatch({ confirmed }, abortSignal)
  }

  // confirmPairing confirms a verified pairing, adding the remote peer as
  // OWNER on all SharedObjects and persisting the paired device.
  public async confirmPairing(
    remotePeerId: string,
    displayName?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.ConfirmPairing(
      { remotePeerId, displayName: displayName ?? '' },
      abortSignal,
    )
  }

  // unlinkDevice removes a paired device and revokes its SO access.
  public async unlinkDevice(
    peerId: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.UnlinkDevice({ peerId }, abortSignal)
  }

  // deleteSpace deletes a space and its associated storage.
  public async deleteSpace(
    sharedObjectId: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.DeleteSpace({ sharedObjectId }, abortSignal)
  }

  // renameSpace updates the display name metadata for a space.
  public async renameSpace(
    request: RenameSpaceRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.RenameSpace(request, abortSignal)
  }

  // deleteAccount deletes the account associated with this session.
  // Cleans ObjectStore keys, removes GC edges, runs volume GC, and
  // removes the session from the list.
  public async deleteAccount(
    sessionIdx: number,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.DeleteAccount({ sessionIdx }, abortSignal)
  }

  // accessStateAtom accesses a session-scoped state atom resource.
  // State is persisted in the session's provider account volume.
  public async accessStateAtom(
    request?: AccessSessionStateAtomRequest,
    abortSignal?: AbortSignal,
  ): Promise<StateAtom> {
    const resp = await this.service.AccessStateAtom(request ?? {}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, StateAtom)
  }

  // watchStateAtoms streams the known session state atom store ids on change.
  public watchStateAtoms(
    request?: WatchSessionStateAtomsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSessionStateAtomsResponse> {
    return this.service.WatchStateAtoms(request ?? {}, abortSignal)
  }

  // localProvider returns a LocalSession wrapper for local-provider-specific RPCs.
  public get localProvider(): LocalSession {
    if (!this._localProvider) {
      this._localProvider = new LocalSession(this.resourceRef)
    }
    return this._localProvider
  }

  // getTransferInventory returns the list of spaces on a session for transfer planning.
  public async getTransferInventory(
    sessionIndex: number,
    abortSignal?: AbortSignal,
  ): Promise<GetTransferInventoryResponse> {
    return await this.service.GetTransferInventory(
      { sessionIndex },
      abortSignal,
    )
  }

  // startTransfer starts a transfer operation between two sessions.
  public async startTransfer(
    req: StartTransferRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.StartTransfer(req, abortSignal)
  }

  // watchTransferProgress streams transfer state updates for an active transfer.
  public watchTransferProgress(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchTransferProgressResponse> {
    return this.service.WatchTransferProgress({}, abortSignal)
  }

  // cancelTransfer stops an in-progress transfer.
  public async cancelTransfer(abortSignal?: AbortSignal): Promise<void> {
    await this.service.CancelTransfer({}, abortSignal)
  }

  // getTransferStatus returns whether a transfer is active or a checkpoint exists.
  public async getTransferStatus(
    abortSignal?: AbortSignal,
  ): Promise<GetTransferStatusResponse> {
    return await this.service.GetTransferStatus({}, abortSignal)
  }

  // watchPairedDevices streams the list of paired devices from the account settings SO.
  public watchPairedDevices(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchPairedDevicesResponse> {
    return this.service.WatchPairedDevices({}, abortSignal)
  }

  // watchPairingStatus streams pairing state changes during device linking.
  public watchPairingStatus(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchPairingStatusResponse> {
    return this.service.WatchPairingStatus({}, abortSignal)
  }

  // createSpaceInvite creates an invite for a space shared object.
  // Returns the full response including the invite message and optional short code.
  public async createSpaceInvite(
    spaceId: string,
    role: SOParticipantRole,
    abortSignal?: AbortSignal,
  ): Promise<CreateSpaceInviteResponse> {
    return await this.service.CreateSpaceInvite({ spaceId, role }, abortSignal)
  }

  // listSpaceInvites lists invites on a space shared object.
  public async listSpaceInvites(
    spaceId: string,
    abortSignal?: AbortSignal,
  ): Promise<ListSpaceInvitesResponse> {
    return await this.service.ListSpaceInvites({ spaceId }, abortSignal)
  }

  // listSpaceParticipants lists participants on a space shared object.
  public async listSpaceParticipants(
    spaceId: string,
    abortSignal?: AbortSignal,
  ): Promise<ListSpaceParticipantsResponse> {
    return await this.service.ListSpaceParticipants({ spaceId }, abortSignal)
  }

  // removeSpaceParticipant removes a participant from a space shared object by peer ID.
  public async removeSpaceParticipant(
    spaceId: string,
    peerId: string,
    abortSignal?: AbortSignal,
  ): Promise<RemoveSpaceParticipantResponse> {
    return await this.service.RemoveSpaceParticipant(
      { spaceId, peerId },
      abortSignal,
    )
  }

  // revokeSpaceInvite revokes an invite on a space shared object.
  public async revokeSpaceInvite(
    spaceId: string,
    inviteId: string,
    abortSignal?: AbortSignal,
  ): Promise<RevokeSpaceInviteResponse> {
    return await this.service.RevokeSpaceInvite(
      { spaceId, inviteId },
      abortSignal,
    )
  }

  // joinSpaceViaInvite joins a space using an out-of-band invite message.
  public async joinSpaceViaInvite(
    inviteMessage: SOInviteMessage,
    abortSignal?: AbortSignal,
  ): Promise<JoinSpaceViaInviteResponse> {
    return await this.service.JoinSpaceViaInvite({ inviteMessage }, abortSignal)
  }

  // createLocalPairingOffer generates a WebRTC SDP offer for no-cloud pairing.
  public async createLocalPairingOffer(
    abortSignal?: AbortSignal,
  ): Promise<CreateLocalPairingOfferResponse> {
    return await this.service.CreateLocalPairingOffer({}, abortSignal)
  }

  // acceptLocalPairingOffer accepts a remote offer and returns an answer.
  public async acceptLocalPairingOffer(
    offerPayload: string,
    abortSignal?: AbortSignal,
  ): Promise<AcceptLocalPairingOfferResponse> {
    return await this.service.AcceptLocalPairingOffer(
      { offerPayload },
      abortSignal,
    )
  }

  // acceptLocalPairingAnswer accepts a remote answer to complete the connection.
  public async acceptLocalPairingAnswer(
    answerPayload: string,
    abortSignal?: AbortSignal,
  ): Promise<AcceptLocalPairingAnswerResponse> {
    return await this.service.AcceptLocalPairingAnswer(
      { answerPayload },
      abortSignal,
    )
  }
}
