package provider_local

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// readAccountSettings reads the canonical local account state.
func (a *ProviderAccount) readAccountSettings(ctx context.Context) (*account_settings.AccountSettings, error) {
	// Resolve the settings object reference.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}

	// Mount the settings object and decode its state.
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	defer release()

	// Decode the settings state.
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := account_settings.ReadSnapshot(ctx, snapshot)
	return settings, err
}

// commitAccountSettingsOp writes op and waits for a readable snapshot whose
// replay applied it.
func commitAccountSettingsOp(ctx context.Context, so sobject.SharedObject, op *account_settings.AccountSettingsOp) error {
	data, err := op.MarshalVT()
	if err != nil {
		return err
	}
	_, err = sobject.WriteOperation(ctx, so, data, account_settings.ProcessAccountSettingsOps)
	return err
}

// registerPairingReplicas publishes the approved identity bindings before
// exporting settings, so every client learns the same account membership.
func (a *ProviderAccount) registerPairingReplicas(ctx context.Context, enrollment *pairing.Enrollment, source, receiving peer.ID) error {
	// Build the member list from both proof identities.
	members := []*account_settings.AccountSession{
		{PeerId: source.String(), StoragePeerId: enrollment.Offer.GetStoragePeerId()},
		{PeerId: enrollment.Identity.GetSessionProof().GetResponderPeerId(), StoragePeerId: enrollment.Identity.GetStorageProof().GetResponderPeerId()},
	}
	if enrollment.Choice.Merging() {
		members = append(members, &account_settings.AccountSession{PeerId: receiving.String(), StoragePeerId: enrollment.Identity.GetStorageProof().GetResponderPeerId()})
	}

	// Reject a member that a previous enrollment removed.
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	for _, member := range members {
		if settings.FindAccountSession(member.GetPeerId()).GetRevoked() {
			return errors.New("removed Session must pair with a new identity")
		}
	}

	// Mount the settings object for the enrollment operations.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()

	// Publish each approved member binding durably.
	for _, member := range members {
		if err := commitAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_UpsertAccountSession{UpsertAccountSession: member},
		}); err != nil {
			return errors.Wrap(err, "register account replica")
		}
	}

	// Name the receiving Session with the label its client sent.
	if label := enrollment.RemoteLabel; label != "" {
		if err := commitAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
				UpsertSessionPresentation: &account_settings.SessionPresentation{
					PeerId:     members[1].GetPeerId(),
					Label:      label,
					DeviceType: "linked",
				},
			},
		}); err != nil {
			return errors.Wrap(err, "name paired Session")
		}
	}

	// Seed existing objects through the same catalog operation used by creation.
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if err := commitAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_UpsertCatalogEntry{
				UpsertCatalogEntry: &account_settings.AccountCatalogEntry{Entry: entry.CloneVT()},
			},
		}); err != nil {
			return errors.Wrap(err, "publish account catalog")
		}
	}
	return nil
}

// enrollAccountMemberObject issues grants only for a stored, approved binding.
// The caller validates current membership before invoking this host mutation.
func (a *ProviderAccount) enrollAccountMemberObject(ctx context.Context, entry *sobject.SharedObjectListEntry, member *account_settings.AccountSession) (*pairing.SharedObject, error) {
	// Require a stored, approved binding.
	if member == nil || member.GetRevoked() {
		return nil, errors.New("account Session is not authorized")
	}

	// Mount the object and resolve the volume's owner key.
	so, release, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
	if err != nil {
		return nil, err
	}
	defer release()
	local := so.(*SharedObject)
	owner, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	key, err := owner.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}

	// Grant OWNER participation to the Session and storage identities.
	for _, id := range []string{member.GetPeerId(), member.GetStoragePeerId()} {
		participant, publicKey, err := peer.ParsePeerIDWithPubKey(id)
		if err != nil {
			return nil, err
		}
		if _, err := sobject.AddSOParticipant(ctx, local.soHost, so.GetSharedObjectID(), key, owner.GetPeerID().String(), participant.String(), publicKey, sobject.SOParticipantRole_SOParticipantRole_OWNER, "", ""); err != nil {
			return nil, err
		}
	}

	// Export the durable state and its config lineage.
	state, err := local.soHost.GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	lineage, err := local.soHost.ReadConfigLineage(ctx, state.GetConfig().GetConfigChainHash())
	if err != nil {
		return nil, err
	}
	if err := local.soHost.WaitDurable(ctx); err != nil {
		return nil, err
	}
	return &pairing.SharedObject{Entry: entry.CloneVT(), State: state.CloneVT(), ConfigLineage: lineage}, nil
}

// revokeAccountReplicaAccess lets the initiating replica finish its one signed
// revocation lineage, including objects discovered after the initial request.
// Other replicas receive those configuration changes through SharedObject sync.
func (a *ProviderAccount) revokeAccountReplicaAccess(ctx context.Context, settings *account_settings.AccountSettings, member *account_settings.AccountSession) error {
	// Remove the revoked member only from this replica's own revocation.
	writer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return err
	}
	if member.GetRevokedByStoragePeerId() != writer.GetPeerID().String() {
		return nil
	}

	// Serialize revocation with other membership mutations.
	release, err := a.replicaAuth.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Remove the member's storage identity only when no other Session shares it.
	identities := []string{member.GetPeerId()}
	shared := false
	for _, current := range settings.GetSessions() {
		if !current.GetRevoked() && current.GetStoragePeerId() == member.GetStoragePeerId() {
			shared = true
			break
		}
	}
	if !shared && member.GetStoragePeerId() != member.GetPeerId() {
		identities = append(identities, member.GetStoragePeerId())
	}

	// Remove the revoked identities from every mounted object.
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		so, releaseSO, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
		if err != nil {
			return err
		}

		// Remove each matching participant from the object's config.
		err = func() error {
			// Load the host state under the released-on-exit closure.
			defer releaseSO()
			host := so.(*SharedObject)
			state, err := host.soHost.GetHostState(ctx)
			if err != nil {
				return err
			}

			// Remove each revoked identity present in the config.
			for _, identity := range identities {
				// Remove the participant when the identity matches.
				for _, participant := range state.GetConfig().GetParticipants() {
					if participant.GetPeerId() == identity {
						if err := a.removeSOParticipant(ctx, so, identity); err != nil {
							return err
						}
						break
					}
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}
