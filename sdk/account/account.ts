import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  AccountResourceService,
  AccountResourceServiceClient,
} from './account_srpc.pb.js'
import {
  AddAuthMethodRequest,
  AddAuthMethodResponse,
  ChangePasswordRequest,
  ChangePasswordResponse,
  GenerateBackupKeyRequest,
  GenerateBackupKeyResponse,
  LinkSSORequest,
  LinkSSOResponse,
  LockAllEntityKeypairsResponse,
  LockEntityKeypairResponse,
  StartDesktopPasskeyRegisterHandoffResponse,
  StartDesktopPasskeyRegisterResponse,
  PasskeyRegisterOptionsResponse,
  PasskeyRegisterVerifyRequest,
  PasskeyRegisterVerifyResponse,
  RemoveAuthMethodRequest,
  RemoveAuthMethodResponse,
  RevokeSessionRequest,
  RevokeSessionResponse,
  SetSecurityLevelRequest,
  SetSecurityLevelResponse,
  UnlockEntityKeypairResponse,
  WatchEntityKeypairsRequest,
  WatchEntityKeypairsResponse,
  WatchAccountInfoRequest,
  WatchAccountInfoResponse,
  WatchAuthMethodsRequest,
  WatchAuthMethodsResponse,
  WatchSessionsRequest,
  WatchSessionsResponse,
} from './account.pb.js'
import type { EntityCredential } from '../../core/session/session.pb.js'

// Account is an account resource that provides access to account functionality.
export class Account extends Resource {
  // service is the account resource service.
  private service: AccountResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new AccountResourceServiceClient(resourceRef.client)
  }

  // watchAccountInfo streams information about this account.
  public watchAccountInfo(
    req?: WatchAccountInfoRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchAccountInfoResponse> {
    return this.service.WatchAccountInfo(req ?? {}, abortSignal)
  }

  // watchAuthMethods streams the account auth-method rows for this account.
  public watchAuthMethods(
    req?: WatchAuthMethodsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchAuthMethodsResponse> {
    return this.service.WatchAuthMethods(req ?? {}, abortSignal)
  }

  // watchSessions streams the attached sessions snapshot for this account.
  public watchSessions(
    req?: WatchSessionsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSessionsResponse> {
    return this.service.WatchSessions(req ?? {}, abortSignal)
  }

  // addAuthMethod adds a new entity keypair (auth method) to the account.
  public addAuthMethod(
    req: AddAuthMethodRequest,
    abortSignal?: AbortSignal,
  ): Promise<AddAuthMethodResponse> {
    return this.service.AddAuthMethod(req, abortSignal)
  }

  // removeAuthMethod removes an entity keypair from the account.
  public removeAuthMethod(
    req: RemoveAuthMethodRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveAuthMethodResponse> {
    return this.service.RemoveAuthMethod(req, abortSignal)
  }

  // setSecurityLevel updates the auth threshold for the account.
  public setSecurityLevel(
    req: SetSecurityLevelRequest,
    abortSignal?: AbortSignal,
  ): Promise<SetSecurityLevelResponse> {
    return this.service.SetSecurityLevel(req, abortSignal)
  }

  // selfRevokeSession revokes the current session using session-signed auth.
  // No entity key or credential is needed. Pass the session peer ID.
  public selfRevokeSession(
    sessionPeerId: string,
    abortSignal?: AbortSignal,
  ): Promise<RevokeSessionResponse> {
    return this.service.RevokeSession({ sessionPeerId }, abortSignal)
  }

  // revokeSession revokes a session by peer ID.
  public revokeSession(
    req: RevokeSessionRequest,
    abortSignal?: AbortSignal,
  ): Promise<RevokeSessionResponse> {
    return this.service.RevokeSession(req, abortSignal)
  }

  // generateBackupKey generates an Ed25519 backup keypair, registers it
  // with the cloud, and returns the private key PEM for download.
  public generateBackupKey(
    req: GenerateBackupKeyRequest,
    abortSignal?: AbortSignal,
  ): Promise<GenerateBackupKeyResponse> {
    return this.service.GenerateBackupKey(req, abortSignal)
  }

  // linkSSO links a Google or GitHub identity to the current account.
  public linkSSO(
    req: LinkSSORequest,
    abortSignal?: AbortSignal,
  ): Promise<LinkSSOResponse> {
    return this.service.LinkSSO(req, abortSignal)
  }

  // changePassword changes the account password by deriving a new entity
  // keypair from the new password, adding it, and removing the old one.
  public changePassword(
    req: ChangePasswordRequest,
    abortSignal?: AbortSignal,
  ): Promise<ChangePasswordResponse> {
    return this.service.ChangePassword(req, abortSignal)
  }

  // watchEntityKeypairs streams entity keypairs with their lock state.
  public watchEntityKeypairs(
    req?: WatchEntityKeypairsRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchEntityKeypairsResponse> {
    return this.service.WatchEntityKeypairs(req ?? {}, abortSignal)
  }

  // unlockEntityKeypair derives the entity private key from a credential and holds it in memory.
  public unlockEntityKeypair(
    peerId: string,
    credential: EntityCredential,
    abortSignal?: AbortSignal,
  ): Promise<UnlockEntityKeypairResponse> {
    return this.service.UnlockEntityKeypair({ peerId, credential }, abortSignal)
  }

  // lockEntityKeypair drops a previously unlocked entity private key.
  public lockEntityKeypair(
    peerId: string,
    abortSignal?: AbortSignal,
  ): Promise<LockEntityKeypairResponse> {
    return this.service.LockEntityKeypair({ peerId }, abortSignal)
  }

  // lockAllEntityKeypairs drops all unlocked entity private keys.
  public lockAllEntityKeypairs(
    abortSignal?: AbortSignal,
  ): Promise<LockAllEntityKeypairsResponse> {
    return this.service.LockAllEntityKeypairs({}, abortSignal)
  }

  // startDesktopPasskeyRegister starts the native desktop passkey register flow.
  public startDesktopPasskeyRegister(
    abortSignal?: AbortSignal,
  ): Promise<StartDesktopPasskeyRegisterResponse> {
    return this.service.StartDesktopPasskeyRegister({}, abortSignal)
  }

  // startDesktopPasskeyRegisterHandoff runs the native desktop add-passkey handoff.
  public startDesktopPasskeyRegisterHandoff(
    abortSignal?: AbortSignal,
  ): Promise<StartDesktopPasskeyRegisterHandoffResponse> {
    return this.service.StartDesktopPasskeyRegisterHandoff({}, abortSignal)
  }

  // passkeyRegisterOptions fetches WebAuthn registration options from the cloud.
  public passkeyRegisterOptions(
    abortSignal?: AbortSignal,
  ): Promise<PasskeyRegisterOptionsResponse> {
    return this.service.PasskeyRegisterOptions({}, abortSignal)
  }

  // passkeyRegisterVerify verifies a WebAuthn registration credential and
  // registers the passkey with the cloud.
  public passkeyRegisterVerify(
    req: PasskeyRegisterVerifyRequest,
    abortSignal?: AbortSignal,
  ): Promise<PasskeyRegisterVerifyResponse> {
    return this.service.PasskeyRegisterVerify(req, abortSignal)
  }
}
