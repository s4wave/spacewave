package provider_local

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
)

// SessionCredentialStore retains this mounted Session's own encrypted credential.
func (s *Session) SessionCredentialStore() object.ObjectStore { return s.objStore }

// RetireAccountTransport releases this Session's old account transport before
// its unchanged key is mounted under the destination's independent lifetime.
func (s *Session) RetireAccountTransport(ctx context.Context) error {
	s.tkr.a.StopP2PSync()
	return s.tkr.a.stopSessionPeerTransport(ctx, s.GetPeerId())
}

// migrationOffer binds storage proofs to the approved account transition.
func migrationOffer(transition *provider.AccountTransition) *pairing.AccountOffer {
	ref := transition.GetDestination()
	return &pairing.AccountOffer{
		ProviderId: ref.GetProviderId(), AccountId: ref.GetProviderAccountId(), SettingsId: ref.GetId(),
		OperationId: transition.GetOperationId(), ProviderEndpoint: transition.GetDestinationEndpoint(),
		SelectionContext: transition.GetSource().GetProviderId() + "/" + transition.GetSource().GetProviderAccountId(),
	}
}

// AttachMigratedSession rewraps this machine's existing key and obtains verified
// destination grants. A returning client uses its old authenticated transport.
func (a *ProviderAccount) AttachMigratedSession(ctx context.Context, source session.Session, transition *provider.AccountTransition) (*session.SessionRef, error) {
	// Validate the transition and require this account as its destination.
	if err := transition.Validate(); err != nil {
		return nil, err
	}
	if !slices.Contains(transition.GetSessionPeerIds(), source.GetPeerId().String()) {
		return nil, errors.New("source Session is not authorized for this account transition")
	}
	destination := transition.GetDestination()
	if destination.GetProviderId() != a.GetProviderID() || destination.GetProviderAccountId() != a.GetAccountID() || transition.GetDestinationEndpoint() != "" {
		return nil, errors.New("account transition names another destination provider")
	}

	// Build the destination Session reference and storage key.
	credential, ok := source.(provider_migration.CredentialSource)
	if !ok {
		return nil, errors.New("source provider cannot move this Session's protected credential")
	}

	// Build the destination Session reference.
	ref := source.GetSessionRef().CloneVT()
	ref.ProviderResourceRef.ProviderId = a.GetProviderID()
	ref.ProviderResourceRef.ProviderAccountId = a.GetAccountID()

	// Resolve the destination storage peer and derive its key.
	storagePeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	storagePrivate, err := storagePeer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	storageKey, err := session_lock.DeriveStorageKey(storagePrivate)
	if err != nil {
		return nil, err
	}

	// Open the destination object store and copy the credential.
	handle, _, releaseStore, err := volume.ExBuildObjectStoreAPI(ctx, a.t.p.b, false, SessionObjectStoreID(a.GetProviderID(), a.GetAccountID()), a.vol.GetID(), nil)
	if err != nil {
		return nil, err
	}
	defer releaseStore.Release()
	if err := session_lock.CopyCredential(ctx, credential.SessionCredentialStore(), handle.GetObjectStore(), source.GetSessionRef().GetProviderResourceRef().GetId(), ref.GetProviderResourceRef().GetId(), source.GetPrivKey(), storageKey); err != nil {
		return nil, err
	}

	// Bind the settings object and read the current settings.
	offer := migrationOffer(transition)
	if err := a.bindPairingSettings(ctx, offer); err != nil {
		return nil, err
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Enroll this Session when the destination already accepted the migration.
	if settings.FindAccountMigration(transition.GetOperationId()) != nil {
		transport := a.GetSessionTransport()
		if transport == nil {
			return nil, ErrNoSessionTransport
		}
		server := transport.GetPeerID()
		if server != source.GetPeerId() {
			identity, err := pairing.BuildIdentity(offer, ref, source.GetPrivKey(), storagePrivate, server, source.GetPeerId())
			if err != nil {
				return nil, err
			}
			if _, err := a.enrollMigratedSession(ctx, transition, identity, server, source.GetPeerId()); err != nil {
				return nil, err
			}
		} else if member := settings.FindAccountSession(source.GetPeerId().String()); member.GetStoragePeerId() != storagePeer.GetPeerID().String() || member.GetRevoked() {
			return nil, errors.New("returning Session has no approved destination storage binding")
		}
	} else if err := a.fetchMigratedEnrollment(ctx, source, transition, ref, storagePrivate); err != nil {
		return nil, err
	}

	// Retire the source transport before the preserved Session mounts.
	if err := credential.RetireAccountTransport(ctx); err != nil {
		return nil, err
	}

	// The destination's temporary enrollment must retire before the preserved
	// Session mounts. Its mount then starts transport and sync on the same bus.
	if transport := a.GetSessionTransport(); transport != nil && transport.GetPeerID() != source.GetPeerId() {
		a.StopP2PSync()
		if err := a.stopSessionPeerTransport(ctx, transport.GetPeerID()); err != nil {
			return nil, err
		}
	}

	// The Session is already unlocked in the source. Preserve its persisted PIN
	// envelope while carrying that same in-memory capability into the new mount.
	keyData, err := keypem.MarshalPrivKeyPem(source.GetPrivKey())
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(keyData)
	reference, tracker, _ := a.sessions.AddKeyRef(ref.GetProviderResourceRef().GetId())
	defer reference.Release()
	tracker.ref.SetResult(ref, nil)
	tracker.unlockProm.SetResult(slices.Clone(keyData), nil)

	// Mount the destination Session and start its transport and sync.
	mounted, releaseSession, err := a.MountSession(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	defer releaseSession()
	if mounted.GetPeerId() != source.GetPeerId() {
		return nil, errors.New("destination Session does not retain the source client's identity")
	}
	if err := a.EnsureConfiguredSessionTransport(ctx, mounted.GetPrivKey()); err != nil {
		return nil, err
	}
	if err := a.AutoStartP2PSyncIfNeeded(ctx, a.GetSessionTransport()); err != nil {
		return nil, err
	}
	return ref, nil
}

// EnrollMigratedSession authorizes the actual authenticated stream peer, then
// verifies possession of the proposed destination storage key independently.
func (a *ProviderAccount) EnrollMigratedSession(ctx context.Context, request *provider_migration.MigratedSessionRequest) (*provider_migration.MigratedSessionResponse, error) {
	stream, err := link.MustGetMountedStreamContext(ctx)
	if err != nil {
		return nil, err
	}
	return a.enrollMigratedSession(ctx, request.GetTransition(), request.GetIdentity(), stream.GetLink().GetLocalPeer(), stream.GetPeerID())
}

func (a *ProviderAccount) enrollMigratedSession(ctx context.Context, transition *provider.AccountTransition, identity *pairing.Identity, server, caller peer.ID) (*provider_migration.MigratedSessionResponse, error) {
	// Authorize the caller against the destination's migration record.
	release, err := a.replicaAuth.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Validate the transition and read the destination settings.
	if err := a.validateMigrationDestination(ctx, transition); err != nil {
		return nil, err
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.FindAccountMigration(transition.GetOperationId()).EqualVT(transition) || !slices.Contains(transition.GetSessionPeerIds(), caller.String()) {
		return nil, errors.New("Session is not authorized by this destination's migration record")
	}
	if identity.GetSessionProof().GetResponderPeerId() != caller.String() {
		return nil, errors.New("migration proof belongs to another authenticated Session")
	}
	if err := pairing.ValidateIdentity(migrationOffer(transition), identity, server, caller); err != nil {
		return nil, err
	}

	// Grant the member on every local object and commit the binding.
	member := &account_settings.AccountSession{PeerId: caller.String(), StoragePeerId: identity.GetStorageProof().GetResponderPeerId()}
	if current := settings.FindAccountSession(caller.String()); current != nil && (current.GetRevoked() || current.GetStoragePeerId() != member.GetStoragePeerId()) {
		return nil, errors.New("this Session already has another destination storage binding")
	}
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if _, err := a.enrollAccountMemberObject(ctx, entry, member); err != nil {
			return nil, errors.Wrapf(err, "enroll returning Session in resource %s", entry.GetRef().GetProviderResourceRef().GetId())
		}
	}
	if err := a.commitMigrationSettings(ctx, &account_settings.AccountSettingsOp{Op: &account_settings.AccountSettingsOp_UpsertAccountSession{UpsertAccountSession: member}}); err != nil {
		return nil, err
	}

	// Export each enrolled object's checkpoint to the caller.
	response := &provider_migration.MigratedSessionResponse{}
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		object, err := a.enrollAccountMemberObject(ctx, entry, member)
		if err != nil {
			return nil, err
		}
		response.Objects = append(response.Objects, object)
	}
	return response, a.retainPairedAccountPeer(ctx, caller)
}

func (a *ProviderAccount) fetchMigratedEnrollment(ctx context.Context, source session.Session, transition *provider.AccountTransition, ref *session.SessionRef, storageKey crypto.PrivKey) error {
	// Connect to the destination through the source's pairing transport.
	owner, ok := source.(pairing.Session)
	if !ok {
		return errors.New("source Session cannot connect to the destination account")
	}
	transport, err := owner.GetPairingTransport(ctx, pairing.Relay{})
	if err != nil {
		return err
	}

	// Enroll with each destination peer until one accepts this Session.
	var lastErr error
	for _, id := range transition.GetDestinationPeerIds() {
		server, _, err := peer.ParsePeerIDWithPubKey(id)
		if err != nil {
			return err
		}
		if server == source.GetPeerId() {
			continue
		}
		identity, err := pairing.BuildIdentity(migrationOffer(transition), ref, source.GetPrivKey(), storageKey, server, source.GetPeerId())
		if err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		open := stream_srpc.NewOpenStreamFunc(transport.GetChildBus(), provider_migration.RecoveryProtocol, source.GetPeerId(), server, 0)
		response, err := provider_migration.NewSRPCAccountMigrationServiceClient(srpc.NewClient(open)).EnrollMigratedSession(requestCtx, &provider_migration.MigratedSessionRequest{Transition: transition, Identity: identity})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}

		// Install the returned checkpoints and require the settings object.
		settingsReceived := false
		for _, object := range response.GetObjects() {
			if err := a.installPairingObject(ctx, migrationOffer(transition), object, server); err != nil {
				return err
			}
			settingsReceived = settingsReceived || object.GetEntry().GetRef().GetProviderResourceRef().GetId() == transition.GetDestination().GetId()
		}
		if !settingsReceived {
			return errors.New("destination did not provide its canonical account settings")
		}
		return nil
	}

	// Report the last failure when no destination peer accepted.
	if lastErr == nil {
		lastErr = errors.New("no destination Session is reachable")
	}
	return lastErr
}
