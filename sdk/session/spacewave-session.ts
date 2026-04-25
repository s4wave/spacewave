import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  SpacewaveSessionResourceServiceClient,
  SpacewaveSessionResourceServiceDefinition,
} from './spacewave-session_srpc.pb.js'
import { SharedObjectSelfEnrollment } from './shared-object-self-enrollment.js'
import type { MountSharedObjectSelfEnrollmentResponse } from './spacewave-session.pb.js'
import {
  BillingInterval,
  CreateOrgInviteRequest as CreateOrgInviteMessage,
  CreateOrgInviteResponse as CreateOrgInviteResponseMessage,
} from '../provider/spacewave/spacewave.pb.js'
import type {
  AddEmailResponse,
  CancelCheckoutSessionResponse,
  CancelSubscriptionResponse,
  ConfirmDeleteNowCodeResponse,
  CreateBillingPortalResponse,
  CreateCheckoutSessionRequest,
  CreateCheckoutSessionResponse,
  CreateLinkedLocalSessionResponse,
  CreateOrgInviteRequest,
  CreateOrgInviteResponse,
  CreateOrganizationResponse,
  DeleteOrganizationResponse,
  EncryptForHandoffRequest,
  EncryptForHandoffResponse,
  EnrollSpaceMemberResponse,
  RemoveSpaceMemberResponse,
  RefreshBillingStateResponse,
  GetLinkedLocalSessionResponse,
  LookupInviteCodeResponse,
  JoinOrganizationResponse,
  LeaveOrganizationResponse,
  ProcessMailboxEntryResponse,
  RequestDeleteNowEmailResponse,
  WatchOrganizationsResponse,
  ReactivateSubscriptionResponse,
  RepairSharedObjectResponse,
  RemoveEmailResponse,
  SetPrimaryEmailResponse,
  ReinitializeSharedObjectResponse,
  RemoveOrgMemberResponse,
  ResetSessionRequest,
  RevokeOrgInviteResponse,
  SendVerificationEmailResponse,
  SwitchBillingIntervalResponse,
  TransferResourceResponse,
  AssignBillingAccountResponse,
  DetachBillingAccountResponse,
  ListManagedBillingAccountsResponse,
  StartDesktopSSOLinkRequest,
  StartDesktopPasskeyReauthRequest,
  StartDesktopPasskeyReauthResponse,
  StartDesktopSSOLinkResponse,
  UnlinkLocalSessionResponse,
  UpdateOrganizationResponse,
  UndoDeleteNowResponse,
  VerifyEmailCodeResponse,
  WatchCheckoutStatusResponse,
  WatchEmailsResponse,
  WatchBillingStateResponse,
  WatchOrganizationStateResponse,
  WatchOnboardingStatusResponse,
  WatchSubscriptionStatusResponse,
} from '../provider/spacewave/spacewave.pb.js'
import type { SOParticipantRole } from '../../core/sobject/sobject.pb.js'

// SpacewaveSession wraps the SpacewaveSessionResourceService SRPC client.
// Access via session.spacewave on a spacewave session. All RPCs are post-auth
// and session-scoped: they resolve the spacewave provider account directly
// from the session without scanning.
export class SpacewaveSession extends Resource {
  private service: SpacewaveSessionResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SpacewaveSessionResourceServiceClient(resourceRef.client)
  }

  // watchOnboardingStatus streams onboarding state changes.
  public watchOnboardingStatus(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchOnboardingStatusResponse> {
    return this.service.WatchOnboardingStatus({}, abortSignal)
  }

  // createLinkedLocalSession creates a local provider session with cloud identity.
  public async createLinkedLocalSession(
    abortSignal?: AbortSignal,
  ): Promise<CreateLinkedLocalSessionResponse> {
    return await this.service.CreateLinkedLocalSession({}, abortSignal)
  }

  // getLinkedLocalSession returns the linked local session index if it exists.
  public async getLinkedLocalSession(
    abortSignal?: AbortSignal,
  ): Promise<GetLinkedLocalSessionResponse> {
    return await this.service.GetLinkedLocalSession({}, abortSignal)
  }

  // unlinkLocalSession unlinks the linked local session.
  public async unlinkLocalSession(
    abortSignal?: AbortSignal,
  ): Promise<UnlinkLocalSessionResponse> {
    return await this.service.UnlinkLocalSession({}, abortSignal)
  }

  // createCheckoutSession creates a Stripe Checkout Session for subscription.
  public async createCheckoutSession(
    request: CreateCheckoutSessionRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateCheckoutSessionResponse> {
    return await this.service.CreateCheckoutSession(request, abortSignal)
  }

  // cancelCheckoutSession cancels pending checkout and expires the Stripe session.
  public async cancelCheckoutSession(
    abortSignal?: AbortSignal,
  ): Promise<CancelCheckoutSessionResponse> {
    return await this.service.CancelCheckoutSession({}, abortSignal)
  }

  // watchSubscriptionStatus streams billing account state changes.
  public watchSubscriptionStatus(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSubscriptionStatusResponse> {
    return this.service.WatchSubscriptionStatus({}, abortSignal)
  }

  // watchBillingState streams combined billing account state and usage.
  public watchBillingState(
    billingAccountId?: string,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchBillingStateResponse> {
    return this.service.WatchBillingState({ billingAccountId }, abortSignal)
  }

  // watchCheckoutStatus streams checkout attempt status changes.
  public watchCheckoutStatus(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchCheckoutStatusResponse> {
    return this.service.WatchCheckoutStatus({}, abortSignal)
  }

  // encryptForHandoff encrypts the session privkey to a device pubkey.
  public async encryptForHandoff(
    request: EncryptForHandoffRequest,
    abortSignal?: AbortSignal,
  ): Promise<EncryptForHandoffResponse> {
    return await this.service.EncryptForHandoff(request, abortSignal)
  }

  // refreshBillingState invalidates the cached billing snapshot so watches reload.
  public async refreshBillingState(
    billingAccountId?: string,
    abortSignal?: AbortSignal,
  ): Promise<RefreshBillingStateResponse> {
    return await this.service.RefreshBillingState(
      { billingAccountId },
      abortSignal,
    )
  }

  // cancelSubscription cancels the active subscription.
  public async cancelSubscription(
    billingAccountId?: string,
    abortSignal?: AbortSignal,
  ): Promise<CancelSubscriptionResponse> {
    return await this.service.CancelSubscription(
      { billingAccountId },
      abortSignal,
    )
  }

  // reactivateSubscription reactivates a canceled subscription.
  public async reactivateSubscription(
    billingAccountId?: string,
    abortSignal?: AbortSignal,
  ): Promise<ReactivateSubscriptionResponse> {
    return await this.service.ReactivateSubscription(
      { billingAccountId },
      abortSignal,
    )
  }

  // switchBillingInterval switches between monthly and annual billing.
  public async switchBillingInterval(
    interval: BillingInterval,
    billingAccountId?: string,
    abortSignal?: AbortSignal,
  ): Promise<SwitchBillingIntervalResponse> {
    return await this.service.SwitchBillingInterval(
      { billingInterval: interval, billingAccountId },
      abortSignal,
    )
  }

  // createBillingPortal creates a Stripe billing portal session URL.
  public async createBillingPortal(
    billingAccountId?: string,
    abortSignal?: AbortSignal,
  ): Promise<CreateBillingPortalResponse> {
    return await this.service.CreateBillingPortal(
      { billingAccountId },
      abortSignal,
    )
  }

  // requestDeleteNowEmail sends a delete-now confirmation email with a code and link.
  public async requestDeleteNowEmail(
    abortSignal?: AbortSignal,
  ): Promise<RequestDeleteNowEmailResponse> {
    return await this.service.RequestDeleteNowEmail({}, abortSignal)
  }

  // confirmDeleteNowCode finalizes delete-now using the 6-digit code from email.
  public async confirmDeleteNowCode(
    code: string,
    abortSignal?: AbortSignal,
  ): Promise<ConfirmDeleteNowCodeResponse> {
    return await this.service.ConfirmDeleteNowCode({ code }, abortSignal)
  }

  // undoDeleteNow cancels a pending delete-now countdown.
  public async undoDeleteNow(
    abortSignal?: AbortSignal,
  ): Promise<UndoDeleteNowResponse> {
    return await this.service.UndoDeleteNow({}, abortSignal)
  }

  // watchOrganizations streams the user's org list, emitting on membership changes.
  public watchOrganizations(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchOrganizationsResponse> {
    return this.service.WatchOrganizations({}, abortSignal)
  }

  // createOrganization creates a new organization.
  public async createOrganization(
    displayName: string,
    abortSignal?: AbortSignal,
  ): Promise<CreateOrganizationResponse> {
    return await this.service.CreateOrganization({ displayName }, abortSignal)
  }

  // watchOrganizationState streams one organization's combined mutable state.
  public watchOrganizationState(
    orgId: string,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchOrganizationStateResponse> {
    return this.service.WatchOrganizationState({ orgId }, abortSignal)
  }

  // updateOrganization updates an organization's display name.
  public async updateOrganization(
    orgId: string,
    displayName: string,
    abortSignal?: AbortSignal,
  ): Promise<UpdateOrganizationResponse> {
    return await this.service.UpdateOrganization(
      { orgId, displayName },
      abortSignal,
    )
  }

  // deleteOrganization deletes an organization.
  public async deleteOrganization(
    orgId: string,
    abortSignal?: AbortSignal,
  ): Promise<DeleteOrganizationResponse> {
    return await this.service.DeleteOrganization({ orgId }, abortSignal)
  }

  // revokeOrgInvite revokes an invite by ID.
  public async revokeOrgInvite(
    orgId: string,
    inviteId: string,
    abortSignal?: AbortSignal,
  ): Promise<RevokeOrgInviteResponse> {
    return await this.service.RevokeOrgInvite({ orgId, inviteId }, abortSignal)
  }

  // leaveOrganization leaves an organization.
  public async leaveOrganization(
    orgId: string,
    abortSignal?: AbortSignal,
  ): Promise<LeaveOrganizationResponse> {
    return await this.service.LeaveOrganization({ orgId }, abortSignal)
  }

  // repairSharedObject retries owner-side recovery for a broken shared object.
  public async repairSharedObject(
    sharedObjectId: string,
    abortSignal?: AbortSignal,
  ): Promise<RepairSharedObjectResponse> {
    return await this.service.RepairSharedObject(
      { sharedObjectId },
      abortSignal,
    )
  }

  // reinitializeSharedObject destructively rewrites a broken shared object in place.
  public async reinitializeSharedObject(
    sharedObjectId: string,
    abortSignal?: AbortSignal,
  ): Promise<ReinitializeSharedObjectResponse> {
    return await this.service.ReinitializeSharedObject(
      { sharedObjectId },
      abortSignal,
    )
  }

  // mountSharedObjectSelfEnrollment mounts the post-sign-in self-enrollment resource.
  public async mountSharedObjectSelfEnrollment(
    abortSignal?: AbortSignal,
  ): Promise<SharedObjectSelfEnrollment> {
    const resp: MountSharedObjectSelfEnrollmentResponse =
      await this.service.MountSharedObjectSelfEnrollment({}, abortSignal)
    return this.resourceRef.createResource(
      resp.resourceId ?? 0,
      SharedObjectSelfEnrollment,
    )
  }

  // removeOrgMember removes a member from an organization.
  public async removeOrgMember(
    orgId: string,
    memberId: string,
    abortSignal?: AbortSignal,
  ): Promise<RemoveOrgMemberResponse> {
    return await this.service.RemoveOrgMember({ orgId, memberId }, abortSignal)
  }

  // createOrgInvite creates an invite for an organization.
  public async createOrgInvite(
    request: CreateOrgInviteRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateOrgInviteResponse> {
    const result = await this.client.request(
      SpacewaveSessionResourceServiceDefinition.typeName,
      SpacewaveSessionResourceServiceDefinition.methods.CreateOrgInvite.name,
      CreateOrgInviteMessage.toBinary(request),
      abortSignal,
    )
    return CreateOrgInviteResponseMessage.fromBinary(result)
  }

  // joinOrganization joins an organization via invite token.
  public async joinOrganization(
    token: string,
    abortSignal?: AbortSignal,
  ): Promise<JoinOrganizationResponse> {
    return await this.service.JoinOrganization({ token }, abortSignal)
  }

  // transferResource transfers a resource to a typed principal owner.
  // newOwnerType is "account" or "organization"; newOwnerId is the destination
  // principal id (account ULID or org ULID).
  public async transferResource(
    resourceId: string,
    newOwnerType: string,
    newOwnerId: string,
    abortSignal?: AbortSignal,
  ): Promise<TransferResourceResponse> {
    return await this.service.TransferResource(
      { resourceId, newOwnerType, newOwnerId },
      abortSignal,
    )
  }

  // listManagedBillingAccounts lists billing accounts the caller manages
  // (created_by_account_id = caller).
  public async listManagedBillingAccounts(
    abortSignal?: AbortSignal,
  ): Promise<ListManagedBillingAccountsResponse> {
    return await this.service.ListManagedBillingAccounts({}, abortSignal)
  }

  // createBillingAccount creates a new unassigned billing account managed by
  // the caller. The caller then runs checkout to activate it and separately
  // calls assignBillingAccount to bind it to a principal.
  public async createBillingAccount(
    displayName: string,
    abortSignal?: AbortSignal,
  ): Promise<string> {
    const resp = await this.service.CreateBillingAccount(
      { displayName },
      abortSignal,
    )
    return resp.billingAccountId ?? ''
  }

  // renameBillingAccount updates the display name on a billing account the
  // caller manages.
  public async renameBillingAccount(
    billingAccountId: string,
    displayName: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.RenameBillingAccount(
      { billingAccountId, displayName },
      abortSignal,
    )
  }

  // deleteBillingAccount permanently removes a canceled billing account the
  // caller manages once it is detached from every principal.
  public async deleteBillingAccount(
    billingAccountId: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.DeleteBillingAccount({ billingAccountId }, abortSignal)
  }

  // assignBillingAccount binds a billing account to a principal.
  // targetOwnerType is "account" or "organization"; targetOwnerId is the principal id.
  public async assignBillingAccount(
    billingAccountId: string,
    targetOwnerType: string,
    targetOwnerId: string,
    abortSignal?: AbortSignal,
  ): Promise<AssignBillingAccountResponse> {
    return await this.service.AssignBillingAccount(
      { billingAccountId, targetOwnerType, targetOwnerId },
      abortSignal,
    )
  }

  // detachBillingAccount clears the billing account assignment on a principal.
  public async detachBillingAccount(
    targetOwnerType: string,
    targetOwnerId: string,
    abortSignal?: AbortSignal,
  ): Promise<DetachBillingAccountResponse> {
    return await this.service.DetachBillingAccount(
      { targetOwnerType, targetOwnerId },
      abortSignal,
    )
  }

  // enrollSpaceMember enrolls an org member into a space by adding them as a participant.
  public async enrollSpaceMember(
    spaceId: string,
    accountId: string,
    role: SOParticipantRole,
    abortSignal?: AbortSignal,
  ): Promise<EnrollSpaceMemberResponse> {
    return await this.service.EnrollSpaceMember(
      { spaceId, accountId, role },
      abortSignal,
    )
  }

  // removeSpaceMember removes an org member from a space by removing them as a participant.
  public async removeSpaceMember(
    spaceId: string,
    accountId: string,
    abortSignal?: AbortSignal,
  ): Promise<RemoveSpaceMemberResponse> {
    return await this.service.RemoveSpaceMember(
      { spaceId, accountId },
      abortSignal,
    )
  }

  // resetSession resets a PIN-locked session.
  public async resetSession(
    request: ResetSessionRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.ResetSession(request, abortSignal)
  }

  // startDesktopSSOLink runs the native-owned desktop SessionDetails SSO-link
  // flow. The handler opens the system browser, waits on the cloud relay for
  // the OAuth result, and returns { provider, code } for LinkSSO completion.
  public async startDesktopSSOLink(
    request: StartDesktopSSOLinkRequest,
    abortSignal?: AbortSignal,
  ): Promise<StartDesktopSSOLinkResponse> {
    return await this.service.StartDesktopSSOLink(request, abortSignal)
  }

  // startDesktopPasskeyReauth runs the native-owned desktop passkey reauth
  // flow. The handler opens the system browser, waits on the cloud relay for
  // the browser-authenticated result, and returns the unwrap artifacts for the
  // existing unlock path.
  public async startDesktopPasskeyReauth(
    request: StartDesktopPasskeyReauthRequest,
    abortSignal?: AbortSignal,
  ): Promise<StartDesktopPasskeyReauthResponse> {
    return await this.service.StartDesktopPasskeyReauth(request, abortSignal)
  }

  // watchEmails streams the account's email list, emitting on changes.
  public watchEmails(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchEmailsResponse> {
    return this.service.WatchEmails({}, abortSignal)
  }

  // sendVerificationEmail sends a verification email to the given address.
  public async sendVerificationEmail(
    email: string,
    abortSignal?: AbortSignal,
  ): Promise<SendVerificationEmailResponse> {
    return await this.service.SendVerificationEmail({ email }, abortSignal)
  }

  // verifyEmailCode verifies a 6-digit code for in-app email verification.
  public async verifyEmailCode(
    email: string,
    code: string,
    abortSignal?: AbortSignal,
  ): Promise<VerifyEmailCodeResponse> {
    return await this.service.VerifyEmailCode({ email, code }, abortSignal)
  }

  // addEmail adds an email address and sends verification.
  public async addEmail(
    email: string,
    abortSignal?: AbortSignal,
  ): Promise<AddEmailResponse> {
    return await this.service.AddEmail({ email }, abortSignal)
  }

  // removeEmail removes an email address from the account.
  public async removeEmail(
    email: string,
    abortSignal?: AbortSignal,
  ): Promise<RemoveEmailResponse> {
    return await this.service.RemoveEmail({ email }, abortSignal)
  }

  // setPrimaryEmail promotes a verified email to primary. Rejects unverified
  // or unknown rows; already-primary is idempotent.
  public async setPrimaryEmail(
    email: string,
    abortSignal?: AbortSignal,
  ): Promise<SetPrimaryEmailResponse> {
    return await this.service.SetPrimaryEmail({ email }, abortSignal)
  }

  // lookupInviteCode resolves a short invite code to the full SOInviteMessage.
  public async lookupInviteCode(
    code: string,
    abortSignal?: AbortSignal,
  ): Promise<LookupInviteCodeResponse> {
    return await this.service.LookupInviteCode({ code }, abortSignal)
  }

  // processMailboxEntry accepts or rejects a mailbox entry.
  public async processMailboxEntry(
    spaceId: string,
    entryId: bigint,
    accept: boolean,
    abortSignal?: AbortSignal,
  ): Promise<ProcessMailboxEntryResponse> {
    return await this.service.ProcessMailboxEntry(
      { spaceId, entryId, accept },
      abortSignal,
    )
  }
}
