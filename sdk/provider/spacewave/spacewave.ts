import { Provider } from '../provider.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  SpacewaveProviderResourceService,
  SpacewaveProviderResourceServiceClient,
} from './spacewave_srpc.pb.js'
import type {
  CloudProviderConfig,
  ConfirmDesktopPasskeyRequest,
  ConfirmDesktopPasskeyResponse,
  ConfirmDesktopSSORequest,
  ConfirmDesktopSSOResponse,
  CreateAccountRequest,
  CreateAccountResponse,
  GenerateAuthKeypairsResponse,
  GeneratePasskeyPrfSaltResponse,
  GetCloudProviderConfigRequest,
  GetLinkedCloudSessionResponse,
  LoginAccountRequest,
  LoginAccountResponse,
  LoginOrCreateAccountRequest,
  LoginOrCreateAccountResponse,
  LoginWithEntityKeyResponse,
  PasskeyCheckUsernameRequest,
  PasskeyCheckUsernameResponse,
  PasskeyConfirmSignupRequest,
  PasskeyConfirmSignupResponse,
  PasskeyAuthOptionsResponse,
  PasskeyAuthOptionsRequest,
  PasskeyAuthVerifyRequest,
  PasskeyAuthVerifyResponse,
  PasskeyRegisterChallengeRequest,
  PasskeyRegisterChallengeResponse,
  RequestRecoveryEmailRequest,
  RequestRecoveryEmailResponse,
  ReauthenticateSessionRequest,
  ReauthenticateSessionResponse,
  RecoverExecuteRequest,
  SSONonceExchangeRequest,
  SSOCodeExchangeRequest,
  SSOCodeExchangeResponse,
  RecoverExecuteResponse,
  RecoverVerifyResponse,
  StartBrowserHandoffRequest,
  StartBrowserHandoffResponse,
  StartDesktopPasskeyRequest,
  StartDesktopPasskeyResponse,
  StartDesktopSSORequest,
  StartDesktopSSOResponse,
  UnwrapWithPasskeyPrfRequest,
  UnwrapWithPasskeyPrfResponse,
  UnwrapPemWithPinResponse,
  WrapWithPasskeyPrfRequest,
  WrapWithPasskeyPrfResponse,
  WrapPemWithPinResponse,
} from './spacewave.pb.js'

// SpacewaveProvider wraps a Provider resource with spacewave-specific pre-auth RPCs.
// Post-auth session-scoped RPCs have moved to SpacewaveSession
// (sdk/session/spacewave-session.ts), accessed via session.spacewave.
export class SpacewaveProvider extends Provider {
  private swService: SpacewaveProviderResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.swService = new SpacewaveProviderResourceServiceClient(
      resourceRef.client,
    )
  }

  // createAccount creates an account on the spacewave provider.
  public async createAccount(
    request: CreateAccountRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateAccountResponse> {
    return await this.swService.CreateAccount(request, abortSignal)
  }

  // loginAccount attempts to log in without creating an account.
  public async loginAccount(
    request: LoginAccountRequest,
    abortSignal?: AbortSignal,
  ): Promise<LoginAccountResponse> {
    return await this.swService.LoginAccount(request, abortSignal)
  }

  // loginOrCreateAccount logs in or creates an account on the spacewave provider.
  public async loginOrCreateAccount(
    request: LoginOrCreateAccountRequest,
    abortSignal?: AbortSignal,
  ): Promise<LoginOrCreateAccountResponse> {
    return await this.swService.LoginOrCreateAccount(request, abortSignal)
  }

  // loginWithEntityKey creates a session using a pre-resolved entity PEM key.
  public async loginWithEntityKey(
    pemPrivateKey: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<LoginWithEntityKeyResponse> {
    return await this.swService.LoginWithEntityKey(
      { pemPrivateKey },
      abortSignal,
    )
  }

  // generateAuthKeypairs creates account and session identity key material.
  public async generateAuthKeypairs(
    abortSignal?: AbortSignal,
  ): Promise<GenerateAuthKeypairsResponse> {
    return await this.swService.GenerateAuthKeypairs({}, abortSignal)
  }

  // wrapPemWithPin wraps an entity PEM with PIN encryption.
  public async wrapPemWithPin(
    pemPrivateKey: string,
    pin: string,
    abortSignal?: AbortSignal,
  ): Promise<WrapPemWithPinResponse> {
    return await this.swService.WrapPemWithPin(
      { pemPrivateKey, pin },
      abortSignal,
    )
  }

  // unwrapPemWithPin unwraps an entity PEM with PIN encryption.
  public async unwrapPemWithPin(
    wrappedPemBase64: string,
    pin: string,
    abortSignal?: AbortSignal,
  ): Promise<UnwrapPemWithPinResponse> {
    return await this.swService.UnwrapPemWithPin(
      { wrappedPemBase64, pin },
      abortSignal,
    )
  }

  // generatePasskeyPrfSalt creates a provider-owned WebAuthn PRF salt.
  public async generatePasskeyPrfSalt(
    abortSignal?: AbortSignal,
  ): Promise<GeneratePasskeyPrfSaltResponse> {
    return await this.swService.GeneratePasskeyPrfSalt({}, abortSignal)
  }

  // wrapWithPasskeyPrf wraps an auth blob with passkey PRF encryption.
  public async wrapWithPasskeyPrf(
    request: WrapWithPasskeyPrfRequest,
    abortSignal?: AbortSignal,
  ): Promise<WrapWithPasskeyPrfResponse> {
    return await this.swService.WrapWithPasskeyPrf(request, abortSignal)
  }

  // unwrapWithPasskeyPrf unwraps an auth blob with passkey PRF encryption.
  public async unwrapWithPasskeyPrf(
    request: UnwrapWithPasskeyPrfRequest,
    abortSignal?: AbortSignal,
  ): Promise<UnwrapWithPasskeyPrfResponse> {
    return await this.swService.UnwrapWithPasskeyPrf(request, abortSignal)
  }

  // passkeyCheckUsername acknowledges the opaque first passkey step.
  public async passkeyCheckUsername(
    request: PasskeyCheckUsernameRequest,
    abortSignal?: AbortSignal,
  ): Promise<PasskeyCheckUsernameResponse> {
    return await this.swService.PasskeyCheckUsername(request, abortSignal)
  }

  // passkeyRegisterChallenge fetches WebAuthn registration options for signup.
  public async passkeyRegisterChallenge(
    request: PasskeyRegisterChallengeRequest,
    abortSignal?: AbortSignal,
  ): Promise<PasskeyRegisterChallengeResponse> {
    return await this.swService.PasskeyRegisterChallenge(request, abortSignal)
  }

  // passkeyAuthOptions fetches WebAuthn authentication options from the cloud.
  public async passkeyAuthOptions(
    request: PasskeyAuthOptionsRequest = {},
    abortSignal?: AbortSignal,
  ): Promise<PasskeyAuthOptionsResponse> {
    return await this.swService.PasskeyAuthOptions(request, abortSignal)
  }

  // passkeyAuthVerify verifies a WebAuthn authentication credential.
  public async passkeyAuthVerify(
    request: PasskeyAuthVerifyRequest,
    abortSignal?: AbortSignal,
  ): Promise<PasskeyAuthVerifyResponse> {
    return await this.swService.PasskeyAuthVerify(request, abortSignal)
  }

  // passkeyConfirmSignup confirms browser-owned passkey signup for the web flow.
  public async passkeyConfirmSignup(
    request: PasskeyConfirmSignupRequest,
    abortSignal?: AbortSignal,
  ): Promise<PasskeyConfirmSignupResponse> {
    return await this.swService.PasskeyConfirmSignup(request, abortSignal)
  }

  // ssoCodeExchange exchanges an OAuth authorization code for account info.
  public async ssoCodeExchange(
    request: SSOCodeExchangeRequest,
    abortSignal?: AbortSignal,
  ): Promise<SSOCodeExchangeResponse> {
    return await this.swService.SSOCodeExchange(request, abortSignal)
  }

  // ssoNonceExchange exchanges an auth-session nonce for stored SSO result.
  public async ssoNonceExchange(
    request: SSONonceExchangeRequest,
    abortSignal?: AbortSignal,
  ): Promise<SSOCodeExchangeResponse> {
    return await this.swService.SSONonceExchange(request, abortSignal)
  }

  // getCloudProviderConfig fetches pre-auth provider config.
  public async getCloudProviderConfig(
    abortSignal?: AbortSignal,
  ): Promise<CloudProviderConfig> {
    return await this.swService.GetCloudProviderConfig(
      {} satisfies GetCloudProviderConfigRequest,
      abortSignal,
    )
  }

  // startBrowserHandoff opens the browser auth handoff flow on native clients.
  public async startBrowserHandoff(
    request: StartBrowserHandoffRequest,
    abortSignal?: AbortSignal,
  ): Promise<StartBrowserHandoffResponse> {
    return await this.swService.StartBrowserHandoff(request, abortSignal)
  }

  // startDesktopSSO starts the native desktop SSO flow.
  public async startDesktopSSO(
    request: StartDesktopSSORequest,
    abortSignal?: AbortSignal,
  ): Promise<StartDesktopSSOResponse> {
    return await this.swService.StartDesktopSSO(request, abortSignal)
  }

  // confirmDesktopSSO completes native desktop SSO account creation.
  public async confirmDesktopSSO(
    request: ConfirmDesktopSSORequest,
    abortSignal?: AbortSignal,
  ): Promise<ConfirmDesktopSSOResponse> {
    return await this.swService.ConfirmDesktopSSO(request, abortSignal)
  }

  // confirmSSO completes browser or native SSO account creation.
  public async confirmSSO(
    request: ConfirmDesktopSSORequest,
    abortSignal?: AbortSignal,
  ): Promise<ConfirmDesktopSSOResponse> {
    return await this.swService.ConfirmDesktopSSO(request, abortSignal)
  }

  // startDesktopPasskey starts the native desktop passkey flow.
  public async startDesktopPasskey(
    request: StartDesktopPasskeyRequest,
    abortSignal?: AbortSignal,
  ): Promise<StartDesktopPasskeyResponse> {
    return await this.swService.StartDesktopPasskey(request, abortSignal)
  }

  // confirmDesktopPasskey completes native desktop passkey account creation.
  public async confirmDesktopPasskey(
    request: ConfirmDesktopPasskeyRequest,
    abortSignal?: AbortSignal,
  ): Promise<ConfirmDesktopPasskeyResponse> {
    return await this.swService.ConfirmDesktopPasskey(request, abortSignal)
  }

  // requestRecoveryEmail requests a recovery email for an account.
  public async requestRecoveryEmail(
    request: RequestRecoveryEmailRequest,
    abortSignal?: AbortSignal,
  ): Promise<RequestRecoveryEmailResponse> {
    return await this.swService.RequestRecoveryEmail(request, abortSignal)
  }

  // recoverVerify verifies a recovery token from an email link.
  public async recoverVerify(
    token: string,
    abortSignal?: AbortSignal,
  ): Promise<RecoverVerifyResponse> {
    return await this.swService.RecoverVerify({ token }, abortSignal)
  }

  // recoverExecute completes account recovery by deriving a new password keypair.
  public async recoverExecute(
    request: RecoverExecuteRequest,
    abortSignal?: AbortSignal,
  ): Promise<RecoverExecuteResponse> {
    return await this.swService.RecoverExecute(request, abortSignal)
  }

  // getLinkedCloudSession returns the linked cloud session index if it exists.
  public async getLinkedCloudSession(
    abortSignal?: AbortSignal,
  ): Promise<GetLinkedCloudSessionResponse> {
    return await this.swService.GetLinkedCloudSession({}, abortSignal)
  }

  // reauthenticateSession re-authenticates a session whose key became stale.
  public async reauthenticateSession(
    request: ReauthenticateSessionRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReauthenticateSessionResponse> {
    return await this.swService.ReauthenticateSession(request, abortSignal)
  }
}
