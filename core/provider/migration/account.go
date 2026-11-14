package provider_migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/crypto"
)

// Info identifies the provider's current account authority before migration.
type Info struct {
	// Settings is the account settings object.
	Settings *sobject.SharedObjectRef
	// Endpoint is the cloud endpoint the account lives on, empty for a local
	// account.
	Endpoint string
	// ParticipantEntity is the entity a migrated object authorizes as the
	// account's participant, empty for a local account.
	ParticipantEntity string
	// ParticipantUsername is the username of ParticipantEntity.
	ParticipantUsername string
	// SessionPeers are the peers of the account's Sessions.
	SessionPeers []string
	// PendingSessionPeers still need to attach after an earlier local migration.
	PendingSessionPeers []string
	// ParticipantPeers are the peers a migrated object must admit as
	// participants: the Session peers and, for a local account, the storage
	// peers that hold its objects.
	ParticipantPeers []string
	// Transition is the account's committed transition, nil before one.
	Transition *provider.AccountTransition
}

// Account owns migration storage, Session authority, and returning-client enrollment.
// Mutating methods are called only after the pairing engine's bilateral approval.
type Account interface {
	provider.ProviderAccount
	sobject.SharedObjectProvider
	session.SessionProvider
	// GetProviderID returns the identifier of the account's provider.
	GetProviderID() string
	// GetAccountID returns the account's identifier within its provider.
	GetAccountID() string
	// MigrationInfo returns the account's current authority, as the Session
	// holding key sees it.
	MigrationInfo(context.Context, crypto.PrivKey) (*Info, error)
	// CheckMigrationSessions refuses a transition this destination cannot
	// take, such as a conflict or an overflow, before any data moves.
	CheckMigrationSessions(context.Context, *provider.AccountTransition, crypto.PrivKey, crypto.PrivKey) error
	// AcceptMigrationSessions durably authorizes the returning Session peers
	// of the transition before the source retires.
	AcceptMigrationSessions(context.Context, *provider.AccountTransition, crypto.PrivKey, crypto.PrivKey) error
	// ImportMigrationObject copies an object of the source account into this
	// one with its accepted state, before the transition commits.
	ImportMigrationObject(context.Context, Account, sobject.SharedObject, *sobject.SharedObjectListEntry, *sobject.SOState) error
	// SpaceBus returns the bus this account's Spaces resolve their operation
	// types on, so an import from it names the payloads of a plugin running
	// beside the source.
	SpaceBus() bus.Bus
	// CommitAccountTransition publishes the transition while the source
	// keeps its data for recovery.
	CommitAccountTransition(context.Context, *provider.AccountTransition, crypto.PrivKey, crypto.PrivKey) error
	// AttachMigratedSession attaches a Session of the source to this account
	// under the transition, keeping its key.
	AttachMigratedSession(context.Context, session.Session, *provider.AccountTransition) (*session.SessionRef, error)
}

// CredentialSource exposes only the mounted Session's own credential store.
// Its retained Session handle owns the store lifetime through migration.
type CredentialSource interface {
	// SessionCredentialStore returns the store of the Session's own
	// encrypted credential.
	SessionCredentialStore() object.ObjectStore
	// RetireAccountTransport releases this credential's old account connection.
	// It is called after the pairing receipt and before the destination starts it.
	RetireAccountTransport(context.Context) error
}

// Merge preserves account identities, resource lineage, and source recovery.
// Provider commits make retries safe; source attachments change only after the
// destination resources and returning Session authorization are durable.
func Merge(ctx context.Context, source Account, mounted session.Session, destination provider.ProviderAccount, destinationRef *session.SessionRef) (func(context.Context) (*session.SessionRef, error), error) {
	// Mount the destination account's Session.
	target, ok := destination.(Account)
	if !ok {
		return nil, errors.New("destination provider does not support account migration")
	}
	targetSession, releaseSession, err := target.MountSession(ctx, destinationRef, nil)
	if err != nil {
		return nil, err
	}
	defer releaseSession()

	// Collect migration info from both accounts, refusing a source with
	// pending Sessions from an earlier merge.
	sourceInfo, err := source.MigrationInfo(ctx, mounted.GetPrivKey())
	if err != nil {
		return nil, err
	}
	if len(sourceInfo.PendingSessionPeers) != 0 {
		return nil, errors.Errorf("reconnect %d Session(s) from the previous account merge before moving this account again", len(sourceInfo.PendingSessionPeers))
	}
	targetInfo, err := target.MigrationInfo(ctx, targetSession.GetPrivKey())
	if err != nil {
		return nil, err
	}

	// Build the account transition and derive its stable operation id.
	transition := &provider.AccountTransition{
		Source: sourceInfo.Settings.GetProviderResourceRef().CloneVT(), Destination: targetInfo.Settings.GetProviderResourceRef().CloneVT(),
		DestinationEndpoint: targetInfo.Endpoint, DestinationPeerIds: uniquePeers(targetInfo.SessionPeers), SessionPeerIds: uniquePeers(sourceInfo.SessionPeers),
		SourceEndpoint: sourceInfo.Endpoint,
	}
	identity, err := transition.MarshalVT()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(identity)
	transition.OperationId = hex.EncodeToString(digest[:])

	// Reuse the source's committed transition and reject a different destination.
	if sourceInfo.Transition != nil {
		if !sourceInfo.Transition.GetDestination().EqualVT(transition.GetDestination()) {
			return nil, errors.New("source account already moved to another destination")
		}
		transition = sourceInfo.Transition.CloneVT()
	}

	// Validate the transition and check both accounts' Sessions.
	if err := transition.Validate(); err != nil {
		return nil, err
	}
	if err := target.CheckMigrationSessions(ctx, transition, mounted.GetPrivKey(), targetSession.GetPrivKey()); err != nil {
		return nil, err
	}

	// Preflight every affected permission before granting or copying any resource.
	list, releaseList, err := source.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer releaseList()
	inventory, err := list.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	entries := make([]*sobject.SharedObjectListEntry, 0, len(inventory.GetSharedObjects()))
	objects := make([]sobject.SharedObject, 0, len(inventory.GetSharedObjects()))
	for _, entry := range inventory.GetSharedObjects() {
		id := entry.GetRef().GetProviderResourceRef().GetId()
		settings := id == sourceInfo.Settings.GetProviderResourceRef().GetId()
		if entry.GetMeta().GetAccountPrivate() && !settings && entry.GetSource() != "migration-recovery" {
			continue
		}
		if !entry.GetMeta().GetAccountPrivate() && entry.GetMeta().GetBodyType() != "space" {
			return nil, errors.Errorf("resource %s has unsupported migration body %s", id, entry.GetMeta().GetBodyType())
		}
		object, release, err := source.MountSharedObject(ctx, entry.GetRef(), nil)
		if err != nil {
			return nil, errors.Wrapf(err, "mount resource %s", id)
		}
		defer release()
		if err := CheckObject(ctx, object, targetInfo.ParticipantPeers); err != nil {
			return nil, errors.Wrapf(err, "resource %s", id)
		}
		entries = append(entries, entry.CloneVT())
		objects = append(objects, object)
	}

	// Copy every non-settings resource into the destination.
	// Checkpoints keep their exact roots and complete permission histories.
	for i, entry := range entries {
		if entry.GetRef().GetProviderResourceRef().GetId() == sourceInfo.Settings.GetProviderResourceRef().GetId() {
			continue
		}
		state, err := AuthorizeObject(ctx, objects[i], targetInfo.ParticipantPeers, targetInfo.ParticipantEntity, targetInfo.ParticipantUsername)
		if err != nil {
			return nil, errors.Wrapf(err, "authorize resource %s", objects[i].GetSharedObjectID())
		}
		if err := target.ImportMigrationObject(ctx, source, objects[i], entry, state); err != nil {
			return nil, errors.Wrapf(err, "copy resource %s", objects[i].GetSharedObjectID())
		}
	}

	// Admit the mounted Sessions on a connected source account.
	if sourceInfo.Endpoint != "" {
		if err := target.AcceptMigrationSessions(ctx, transition, mounted.GetPrivKey(), targetSession.GetPrivKey()); err != nil {
			return nil, err
		}
	}

	// Commit the transition on the source account.
	if err := source.CommitAccountTransition(ctx, transition, mounted.GetPrivKey(), targetSession.GetPrivKey()); err != nil {
		return nil, err
	}

	// Retain the source's signed redirect in the destination's sync lifecycle so
	// an offline local replica can discover it after the original client leaves.
	if sourceInfo.Endpoint == "" {
		for i, entry := range entries {
			if entry.GetRef().GetProviderResourceRef().GetId() != sourceInfo.Settings.GetProviderResourceRef().GetId() {
				continue
			}
			state, err := AuthorizeObject(ctx, objects[i], targetInfo.ParticipantPeers, targetInfo.ParticipantEntity, targetInfo.ParticipantUsername)
			if err != nil {
				return nil, err
			}
			entry.Source = "migration-recovery"
			if err := target.ImportMigrationObject(ctx, source, objects[i], entry, state); err != nil {
				return nil, errors.Wrap(err, "retain source recovery")
			}
		}
		// Admission is last: returning local Sessions cannot leave their source
		// until the destination also retains its independently deliverable redirect.
		if err := target.AcceptMigrationSessions(ctx, transition, mounted.GetPrivKey(), targetSession.GetPrivKey()); err != nil {
			return nil, err
		}
	}

	// The caller acknowledges the completed account transfer before retiring
	// the transport that carries its pairing receipt.
	return func(ctx context.Context) (*session.SessionRef, error) {
		ref, err := target.AttachMigratedSession(ctx, mounted, transition)
		if err != nil {
			return nil, err
		}
		if err := RebindSession(ctx, mounted, ref); err != nil {
			return nil, err
		}
		return ref, nil
	}, nil
}

// RebindSession publishes an already durable provider attachment at its old UI index.
func RebindSession(ctx context.Context, mounted session.Session, ref *session.SessionRef) error {
	// Resolve the Session controller on the mounted Session's bus.
	controller, release, err := session.ExLookupSessionController(ctx, mounted.GetBus(), "", false, nil)
	if err != nil {
		return err
	}
	defer release.Release()

	// Commit the session transition through the transition controller.
	transition, ok := controller.(session.SessionTransitionController)
	if !ok {
		return errors.New("Session controller cannot commit account transitions")
	}
	return transition.TransitionSession(ctx, mounted.GetSessionRef(), ref)
}

func uniquePeers(peers []string) []string {
	next := slices.Clone(peers)
	slices.Sort(next)
	return slices.Compact(next)
}
