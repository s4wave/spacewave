package resource_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space "github.com/s4wave/spacewave/core/space"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// errPluginCallerDenied is returned for a denied call from a plugin outside
// the host set.
var errPluginCallerDenied = errors.New("not available to a plugin outside the host set")

// pluginCallerDenied lists the methods a plugin outside the host set may not
// call on the root or on any Resource beneath it, keyed by service and method
// ID. Each group describes the class of state it protects.
var pluginCallerDenied = map[string]struct{}{
	// Approvals: which plugins run for the person, in a session or a Space,
	// and building or restarting a Space's plugins.
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetPluginApproval"):   {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetAccountPlugin"):    {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetBackgroundPlugin"): {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "DeployManifests"):         {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "AddSpacePlugin"):          {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "ImportSpacePlugin"):       {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "RemoveSpacePlugin"):       {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "BuildSpacePlugin"):        {},
	webMethod(s4wave_space.SRPCSpaceContentsResourceServiceServiceID, "RestartPlugin"):   {},

	// Sessions: unlocking, locking, linking and pairing sessions and devices.
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "UnlockSession"):                                    {},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "ResetSession"):                                     {},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "MoveSession"):                                      {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetLockMode"):                                {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "UnlockSession"):                              {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "LockSession"):                                {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "GeneratePairingCode"):                        {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "CompletePairing"):                            {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SelectPairingAccount"):                       {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "ConfirmSASMatch"):                            {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "ConfirmPairing"):                             {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "CreateLocalPairingOffer"):                    {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "AcceptLocalPairingOffer"):                    {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "AcceptLocalPairingAnswer"):                   {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "UnlinkDevice"):                               {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "StartTransfer"):                              {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "CancelTransfer"):                             {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateLinkedLocalSession"):          {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "UnlinkLocalSession"):                {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ResetSession"):                      {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "RevokeSession"):                              {},
	webMethod(s4wave_provider_spacewave.SRPCSpacewaveProviderResourceServiceServiceID, "ReauthenticateSession"): {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "EnrollForHandoff"):                  {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "StartDesktopSSOLink"):               {},

	// Membership: who belongs to a Space or an organization, who governs a
	// Space, and how an organization is set up.
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "CreateSpaceInvite"):                                       {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "RevokeSpaceInvite"):                                       {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "RemoveSpaceParticipants"):                                 {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "ResolveSpaceJoinRequest"):                                 {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "JoinSpaceViaInvite"):                                      {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "LeaveSpace"):                                              {},
	webMethod(s4wave_session.SRPCSharedObjectSelfEnrollmentResourceServiceServiceID, "Start"):                                {},
	webMethod(s4wave_session.SRPCSharedObjectSelfEnrollmentResourceServiceServiceID, "Skip"):                                 {},
	webMethod(s4wave_session.SRPCLocalSessionResourceServiceServiceID, "ApproveSpaceLink"):                                   {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ApproveSpaceLink"):                               {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ApproveGuestSpaceLink"):                          {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "EnrollSpaceMember"):                              {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RemoveSpaceMember"):                              {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateOrgInvite"):                                {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RevokeOrgInvite"):                                {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "JoinOrganization"):                               {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "LeaveOrganization"):                              {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RemoveOrgMember"):                                {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateTargetedInvitation"):                       {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateSpaceTargetedInvitationByUsername"):        {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateOrganizationTargetedInvitationByUsername"): {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "AcceptSpaceTargetedInvitation"):                  {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "AcceptOrganizationTargetedInvitation"):           {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RevokeTargetedInvitation"):                       {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "TransferResource"):                               {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "LinkSpacePublicOrigin"):                          {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "SetSpacePublicRead"):                             {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RedeemInvite"):                                   {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ProcessTargetedInvitation"):                      {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ProcessMailboxEntry"):                            {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "UpdateOrganization"):                             {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "SetSpaceSequencer"):                                           {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "SetSpaceControl"):                                             {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "SetSpaceRoster"):                                              {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "ApproveSpaceChange"):                                          {},

	// Keys: entity keypairs, Space secrets, and the credentials and addresses
	// that recover an account.
	webMethod(s4wave_session.SRPCLocalSessionResourceServiceServiceID, "AddEntityKeypair"):                     {},
	webMethod(s4wave_session.SRPCLocalSessionResourceServiceServiceID, "RemoveEntityKeypair"):                  {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "CreateSecret"):                                  {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "ReadSecretPayload"):                             {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "WriteSecretPayload"):                            {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "AddSecretParticipant"):                          {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "ChangePassword"):                            {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "AddAuthMethod"):                             {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "RemoveAuthMethod"):                          {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "SetSecurityLevel"):                          {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "GenerateBackupKey"):                         {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "UnlockEntityKeypair"):                       {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "LockEntityKeypair"):                         {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "LockAllEntityKeypairs"):                     {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "SignWithEntityKeypair"):                     {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "LinkSSO"):                                   {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "SSOCodeExchange"):                           {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "StartDesktopPasskeyRegister"):               {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "StartDesktopPasskeyRegisterHandoff"):        {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "PasskeyRegisterOptions"):                    {},
	webMethod(s4wave_account.SRPCAccountResourceServiceServiceID, "PasskeyRegisterVerify"):                     {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "AddEmail"):                         {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RemoveEmail"):                      {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "SetPrimaryEmail"):                  {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "StartDesktopPasskeyReauth"):        {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "SendVerificationEmail"):            {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "VerifyEmailCode"):                  {},
	webMethod(s4wave_provider_spacewave.SRPCSpacewaveProviderResourceServiceServiceID, "RequestRecoveryEmail"): {},
	webMethod(s4wave_provider_spacewave.SRPCSpacewaveProviderResourceServiceServiceID, "RecoverVerify"):        {},
	webMethod(s4wave_provider_spacewave.SRPCSpacewaveProviderResourceServiceServiceID, "RecoverExecute"):       {},

	// Storage and network: where a session stores its Spaces, what storage
	// costs, which host folders a Drive exposes, and whether it connects to
	// peers directly.
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "AddStorageBackend"):        {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetDefaultStorageBackend"): {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetStorageBackendPricing"): {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "MoveSpaceStorage"):         {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "SetDirectP2PEnabled"):      {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "LinkFolder"):                   {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "ResetFolderRoot"):              {},

	// Runtime: the web listeners this device serves and which runtime holds
	// them, including the person's answer to a listener yield prompt.
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "StopWebListener"):              {},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "ReclaimRuntime"):               {},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "RespondToListenerYieldPrompt"): {},

	// Billing: subscriptions, spending limits and billing accounts.
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateCheckoutSession"):   {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CancelCheckoutSession"):   {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CancelSubscription"):      {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ReactivateSubscription"):  {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "SetBillingSpendingLimit"): {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateBillingPortal"):     {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "CreateBillingAccount"):    {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "AssignBillingAccount"):    {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "DetachBillingAccount"):    {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RenameBillingAccount"):    {},

	// Deletion: sessions, accounts, Spaces, secrets, storage, organizations,
	// billing accounts, the Space root aliases an upsert overwrites, and the
	// delete-now email and its undo.
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "DeleteSession"):                           {},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "UpsertSpaceRootAlias"):                    {},
	webMethod(s4wave_root.SRPCRootResourceServiceServiceID, "RemoveSpaceRootAlias"):                    {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "DeleteSpace"):                       {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "DeleteAccount"):                     {},
	webMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "RemoveStorageBackend"):              {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "DeleteSecret"):                          {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "DeleteOrganization"):       {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "DeleteBillingAccount"):     {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ConfirmDeleteNowCode"):     {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "RequestDeleteNowEmail"):    {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "UndoDeleteNow"):            {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ReinitializeSharedObject"): {},
	webMethod(s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID, "ReleaseSpaceHostedCopies"): {},
}

// pluginCallerSecretMethods are the denied methods that grant admits: a
// plugin whose Space loaded it reads, creates and writes that Space's Secrets
// when the Device grants access. Granting participants and deleting stay
// denied.
var pluginCallerSecretMethods = map[string]struct{}{
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "CreateSecret"):       {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "ReadSecretPayload"):  {},
	webMethod(s4wave_space.SRPCSpaceResourceServiceServiceID, "WriteSecretPayload"): {},
}

// NewPluginCallerFilter returns the InvokerFilter for the core root
// Resource service. It refuses the pluginCallerDenied methods to a call that
// carries a PluginCaller outside the host set, on the root and on every
// Resource beneath it. It admits the pluginCallerSecretMethods to a caller
// whose binding is a Space when grant permits that plugin in that Space. A
// nil grant permits none. The filter cannot see which Space a Resource
// serves, so the Space Resource refuses a caller bound to another Space.
//
// A call with no PluginCaller passes. That path is safe only because a plugin's
// calls reach core through the target's PluginRpc, where the host stamps them,
// while a plugin's root mux serves the host's calls alone. A path that lets a
// plugin reach core without passing through PluginRpc would bypass this filter.
func NewPluginCallerFilter(le *logrus.Entry, grant PluginSecretGrant) resource_server.InvokerFilter {
	return func(inv srpc.Invoker) srpc.Invoker {
		return &pluginCallerInvoker{le: le, grant: grant, inv: inv}
	}
}

// pluginCallerInvoker refuses the denied methods to plugins outside the host
// set, apart from the Secret methods grant admits.
type pluginCallerInvoker struct {
	le    *logrus.Entry
	grant PluginSecretGrant
	inv   srpc.Invoker
}

// InvokeMethod refuses a denied method to a caller outside the host set.
func (i *pluginCallerInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	caller := bldr_plugin.PluginCallerFromContext(strm.Context())
	if caller != nil && !caller.GetHostSet() {
		key := webMethod(serviceID, methodID)
		if _, denied := pluginCallerDenied[key]; denied && !i.granted(key, caller) {
			i.le.WithField("method", key).WithField("plugin-id", caller.GetPluginId()).Info("refused method to plugin")
			return true, errors.Wrap(errPluginCallerDenied, key)
		}
	}
	return i.inv.InvokeMethod(serviceID, methodID, strm)
}

// granted reports whether the grant admits the denied method key to caller.
func (i *pluginCallerInvoker) granted(key string, caller *bldr_plugin.PluginCaller) bool {
	if _, ok := pluginCallerSecretMethods[key]; !ok || i.grant == nil {
		return false
	}
	spaceID, ok := space.SpaceIdFromEngineId(caller.GetBindingKey())
	return ok && i.grant.PermitsSecretPlugin(spaceID, caller.GetPluginId())
}

// _ is a type assertion
var _ srpc.Invoker = (*pluginCallerInvoker)(nil)
