package provider_local

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// MergePairingAccount moves the approved source through the destination provider.
func (a *ProviderAccount) MergePairingAccount(ctx context.Context, mounted session.Session, destination provider.ProviderAccount, ref *session.SessionRef) (func(context.Context) (*session.SessionRef, error), error) {
	return provider_migration.Merge(ctx, a, mounted, destination, ref)
}

// MigrationInfo reads the canonical registry and keeps the active Session in the census.
func (a *ProviderAccount) MigrationInfo(ctx context.Context, key crypto.PrivKey) (*provider_migration.Info, error) {
	// Read the settings, receiving reference, storage peer, and local peer.
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Build the census from the local Session and storage peer.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}
	storage, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	current, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	info := &provider_migration.Info{
		Settings: ref, SessionPeers: []string{current.String()}, ParticipantPeers: []string{current.String(), storage.GetPeerID().String()}, Transition: settings.GetTransition().CloneVT(),
	}

	// Add each active member's Session and storage peer to the census.
	for _, member := range settings.GetSessions() {
		if member.GetRevoked() {
			continue
		}
		info.SessionPeers = append(info.SessionPeers, member.GetPeerId())
		info.ParticipantPeers = append(info.ParticipantPeers, member.GetPeerId(), member.GetStoragePeerId())
	}

	// An offline predecessor can follow its original signed redirect after the
	// merging machine leaves. Keep that destination until it has attached.
	for _, accepted := range settings.GetAcceptedMigrations() {
		for _, id := range accepted.GetSessionPeerIds() {
			if settings.FindAccountSession(id) == nil && id != current.String() {
				info.PendingSessionPeers = append(info.PendingSessionPeers, id)
				info.SessionPeers = append(info.SessionPeers, id)
			}
		}
	}

	// Sort and deduplicate each peer list.
	slices.Sort(info.PendingSessionPeers)
	info.PendingSessionPeers = slices.Compact(info.PendingSessionPeers)
	slices.Sort(info.SessionPeers)
	info.SessionPeers = slices.Compact(info.SessionPeers)
	slices.Sort(info.ParticipantPeers)
	info.ParticipantPeers = slices.Compact(info.ParticipantPeers)
	return info, nil
}

// CheckMigrationSessions checks the destination's complete current permission set.
// An externally owned Space can block enrollment; it is never silently omitted.
func (a *ProviderAccount) CheckMigrationSessions(ctx context.Context, transition *provider.AccountTransition, _ crypto.PrivKey, _ crypto.PrivKey) error {
	// Validate the transition and the destination's current settings.
	if err := a.validateMigrationDestination(ctx, transition); err != nil {
		return err
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	if current := settings.FindAccountMigration(transition.GetOperationId()); current != nil && !current.EqualVT(transition) {
		return errors.New("destination already accepted another migration authorization")
	}

	// Require every authorized Session peer to remain active.
	for _, id := range transition.GetSessionPeerIds() {
		if settings.FindAccountSession(id).GetRevoked() {
			return errors.New("a removed destination Session must pair with a new key before merging")
		}
	}

	// Check each mounted object grants the destination Sessions access.
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		object, release, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
		if err != nil {
			return err
		}
		err = provider_migration.CheckObject(ctx, object, transition.GetSessionPeerIds())
		release()
		if err != nil {
			return errors.Wrapf(err, "destination resource %s", entry.GetRef().GetProviderResourceRef().GetId())
		}
	}
	return nil
}

// AcceptMigrationSessions durably authorizes each returning Session peer before
// source retirement. Its storage identity is bound only after possession is proven.
func (a *ProviderAccount) AcceptMigrationSessions(ctx context.Context, transition *provider.AccountTransition, sourceKey, destinationKey crypto.PrivKey) error {
	// Validate the transition and lock replica authorization against mutations.
	if err := a.CheckMigrationSessions(ctx, transition, sourceKey, destinationKey); err != nil {
		return err
	}
	release, err := a.replicaAuth.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		object, releaseObject, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
		if err != nil {
			return err
		}
		_, err = provider_migration.AuthorizeObject(ctx, object, transition.GetSessionPeerIds(), "", "")
		releaseObject()
		if err != nil {
			return err
		}
	}

	// Commit the accepted migration into the account settings.
	return a.commitMigrationSettings(ctx, &account_settings.AccountSettingsOp{Op: &account_settings.AccountSettingsOp_AcceptAccountMigration{AcceptAccountMigration: transition}})
}

// CommitAccountTransition publishes the signed redirect while retaining source data.
func (a *ProviderAccount) CommitAccountTransition(ctx context.Context, transition *provider.AccountTransition, _, _ crypto.PrivKey) error {
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	if !transition.GetSource().EqualVT(ref.GetProviderResourceRef()) {
		return errors.New("account transition belongs to another source")
	}
	return a.commitMigrationSettings(ctx, &account_settings.AccountSettingsOp{Op: &account_settings.AccountSettingsOp_CommitAccountTransition{CommitAccountTransition: transition}})
}

func (a *ProviderAccount) commitMigrationSettings(ctx context.Context, op *account_settings.AccountSettingsOp) error {
	// Mount the settings object and commit the operation.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}

	// Mount the settings object and commit the operation.
	object, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()
	return commitAccountSettingsOp(ctx, object, op)
}

func (a *ProviderAccount) validateMigrationDestination(ctx context.Context, transition *provider.AccountTransition) error {
	// Validate the transition and require this account as the destination.
	if err := transition.Validate(); err != nil {
		return err
	}
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	if !transition.GetDestination().EqualVT(ref.GetProviderResourceRef()) || transition.GetDestinationEndpoint() != "" {
		return errors.New("migration belongs to another destination account")
	}
	return nil
}

// ImportMigrationObject copies the accepted data before publishing its verified
// checkpoint. Source blocks and grants remain available after failure or retry.
func (a *ProviderAccount) ImportMigrationObject(ctx context.Context, _ provider_migration.Account, object sobject.SharedObject, entry *sobject.SharedObjectListEntry, state *sobject.SOState) error {
	// Create the destination block store for the migrated object.
	id := entry.GetRef().GetProviderResourceRef().GetId()
	blockID := SobjectBlockStoreID(id)
	if _, err := a.CreateBlockStore(ctx, blockID); err != nil && !errors.Is(err, bstore.ErrBlockStoreExists) {
		return err
	}

	// Mount the destination block store for the copy.
	ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), id, blockID)
	blocksRef := &bstore.BlockStoreRef{ProviderResourceRef: ref.GetProviderResourceRef().CloneVT()}
	blocksRef.ProviderResourceRef.Id = blockID
	blocks, release, err := a.MountBlockStore(ctx, blocksRef, nil)
	if err != nil {
		return err
	}
	defer release()

	// Checkpoint and copy a Space.
	if entry.GetMeta().GetBodyType() == "space" {
		var world *sobject_world_engine.InnerState
		state, world, err = provider_migration.Checkpoint(ctx, a.le, a.t.p.b, a.t.p.sfs, entry.GetRef(), object, state)
		if err != nil {
			return err
		}
		if err := provider_migration.CopyWorld(ctx, object, world, blocks); err != nil {
			return err
		}
	}

	// Mount the enrolled object with the source's config lineage.
	var lineage []*sobject.SOConfigChange
	if host, ok := object.(sobject.InviteHost); ok {
		lineage, err = host.GetSOHost().ReadConfigLineage(ctx, state.GetConfig().GetConfigChainHash())
		if err != nil {
			return err
		}
	}
	if err := a.mountEnrolledSO(ctx, id, entry.GetMeta(), entry.GetSource(), state, lineage, object.GetPeerID()); err != nil {
		return err
	}
	next := entry.CloneVT()
	next.Ref = ref
	return a.publishAccountCatalogEntry(ctx, next, false)
}

var (
	_ pairing.AccountMerger      = (*ProviderAccount)(nil)
	_ provider_migration.Account = (*ProviderAccount)(nil)
)
