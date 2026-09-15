package provider_local

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// OfferPairingAccount identifies the canonical local account and its replica signer.
func (a *ProviderAccount) OfferPairingAccount(ctx context.Context, key crypto.PrivKey) (*pairing.AccountOffer, error) {
	settings, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}
	storagePeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	settingsSO, releaseSettings, err := a.MountSharedObject(ctx, settings, nil)
	if err != nil {
		return nil, err
	}
	defer releaseSettings()
	snapshot, err := settingsSO.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	accountSettings, _, err := decodeAccountSettingsSnapshot(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	name := accountSettings.GetDisplayName()
	if name == "" {
		name = "Local account"
	}
	offer := &pairing.AccountOffer{ProviderId: a.GetProviderID(), RevokedSessionPeerIds: nil, AccountId: a.GetAccountID(), SettingsId: settings.GetProviderResourceRef().GetId(), OperationId: ulid.NewULID(), StoragePeerId: storagePeer.GetPeerID().String(), DisplayName: name}
	current, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	active := map[string]bool{current.String(): true}
	for _, presentation := range accountSettings.GetSessionPresentations() {
		if presentation.GetPeerId() == current.String() {
			offer.MachineName = presentation.GetLabel()
		}
	}
	for _, member := range accountSettings.GetSessions() {
		if member.GetRevoked() {
			offer.RevokedSessionPeerIds = append(offer.RevokedSessionPeerIds, member.GetPeerId())
		} else {
			active[member.GetPeerId()] = true
		}
	}
	for _, transition := range accountSettings.GetAcceptedMigrations() {
		for _, id := range transition.GetSessionPeerIds() {
			if !accountSettings.FindAccountSession(id).GetRevoked() {
				active[id] = true
			}
		}
	}
	offer.SessionCount = uint32(len(active)) //nolint:gosec // the offer count is the in-memory session map cardinality.
	return offer, nil
}

// PreparePairingReceiver reserves a receiving Session without granting account access.
func (a *ProviderAccount) PreparePairingReceiver(ctx context.Context, offer *pairing.AccountOffer, sourcePeer, receivingPeer peer.ID) (*pairing.Receiver, error) {
	if offer.GetSettingsId() == "" || offer.GetAccountId() != a.GetAccountID() {
		return nil, errors.New("pairing source did not offer this local account")
	}
	if _, _, err := peer.ParsePeerIDWithPubKey(offer.GetStoragePeerId()); err != nil {
		return nil, err
	}
	ref, err := a.preparePairingSession(ctx)
	if err != nil {
		return nil, err
	}
	sess, release, err := a.MountSession(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	// Repair reserves a fresh key before approval when prior access was revoked.
	if slices.Contains(offer.GetRevokedSessionPeerIds(), sess.GetPeerId().String()) {
		release()
		ref = &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: a.GetProviderID(), ProviderAccountId: a.GetAccountID(), Id: ulid.NewULID()}}
		if err := a.storePairingSessionRef(ctx, ref); err != nil {
			return nil, err
		}
		sess, release, err = a.MountSession(ctx, ref, nil)
		if err != nil {
			return nil, err
		}
	}
	identity, err := a.buildPairingIdentity(ctx, offer, sess, sourcePeer, receivingPeer)
	if err != nil {
		release()
		return nil, err
	}
	return &pairing.Receiver{Transport: func(ctx context.Context) (*transport.SessionTransport, error) {
		return sess.(pairing.Session).GetPairingTransport(ctx, pairing.Relay{})
	}, Identity: identity, Release: release, Receive: func(ctx context.Context, stream *stream_packet.Session) error {
		return a.receivePairingEnrollment(ctx, stream, offer, sess, sourcePeer)
	}}, nil
}

// storePairingSessionRef replaces the resumable receiver after its prior key
// was revoked. The rejected key remains durable for audit and cannot be reused.
func (a *ProviderAccount) storePairingSessionRef(ctx context.Context, ref *session.SessionRef) error {
	release, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	store, releaseStore, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return err
	}
	defer releaseStore()
	data, err := ref.MarshalVT()
	if err != nil {
		return err
	}
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, SobjectBindingKey("pairing-session"), data)
	})
}

// EnrollPairingReceiver authorizes both receiving keys and transfers checkpoints.
func (a *ProviderAccount) EnrollPairingReceiver(ctx context.Context, stream *stream_packet.Session, enrollment *pairing.Enrollment, _ crypto.PrivKey, sourcePeer, receivingPeer peer.ID) error {
	offer, identity := enrollment.Offer, enrollment.Identity
	if err := pairing.ValidateIdentity(offer, identity, sourcePeer, receivingPeer); err != nil {
		return err
	}
	release, err := a.replicaAuth.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := a.registerPairingReplicas(ctx, enrollment, sourcePeer, receivingPeer); err != nil {
		return err
	}
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		// A merge keeps the authenticated receiving key after its temporary
		// enrollment retires. Grant it before exporting any checkpoint, so
		// that handoff never depends on background membership propagation.
		if enrollment.Choice.Merging() {
			member := &account_settings.AccountSession{PeerId: receivingPeer.String(), StoragePeerId: identity.GetStorageProof().GetResponderPeerId()}
			if _, err := a.enrollAccountMemberObject(ctx, entry, member); err != nil {
				return errors.Wrap(err, "enroll merging Session")
			}
		}
		object, err := a.enrollPairingObject(ctx, entry, identity)
		if err != nil {
			return errors.Wrap(err, "enroll SharedObject "+entry.GetRef().GetProviderResourceRef().GetId())
		}
		if err := stream.SendMsg(&pairing.Frame{Body: &pairing.Frame_Object{Object: object}}); err != nil {
			return err
		}
	}
	remotePeer, _, err := peer.ParsePeerIDWithPubKey(identity.GetSessionProof().GetResponderPeerId())
	if err != nil {
		return err
	}
	if err := a.retainPairedAccountPeer(ctx, remotePeer); err != nil {
		return err
	}
	return stream.SendMsg(&pairing.Frame{Body: &pairing.Frame_Complete{Complete: true}})
}

// receivePairingEnrollment imports checkpoints and registers the durable attachment.
func (a *ProviderAccount) receivePairingEnrollment(ctx context.Context, stream *stream_packet.Session, offer *pairing.AccountOffer, sess session.Session, sourcePeer peer.ID) error {
	// Bind the canonical settings identity before importing the replica's catalog.
	if err := a.bindPairingSettings(ctx, offer); err != nil {
		return err
	}
	settingsReceived := false
	for {
		frame, err := pairing.ReceiveFrame(stream)
		if err != nil {
			return err
		}
		if frame.GetComplete() {
			break
		}
		object := frame.GetObject()
		if err := a.installPairingObject(ctx, offer, object, sourcePeer); err != nil {
			return err
		}
		if object.GetEntry().GetRef().GetProviderResourceRef().GetId() == offer.GetSettingsId() {
			settingsReceived = true
		}
	}
	if !settingsReceived {
		return errors.New("source did not provide the canonical account settings")
	}

	// Publish the durable account attachment through the ordinary Session controller.
	if err := a.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
		return err
	}
	if err := a.retainPairedAccountPeer(ctx, sourcePeer); err != nil {
		return err
	}
	controller, releaseController, err := session.ExLookupSessionController(ctx, a.t.p.b, "", false, nil)
	if err != nil {
		return err
	}
	defer releaseController.Release()
	ref := sess.GetSessionRef()
	metadata := &session.SessionMetadata{
		ProviderId: "local", ProviderDisplayName: "Local", ProviderAccountId: a.GetAccountID(), CreatedAt: time.Now().UnixMilli(),
	}
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.GetSessionRef().EqualVT(ref) {
			metadata = nil
			break
		}
	}
	if _, err := controller.RegisterSession(ctx, ref, metadata); err != nil {
		return err
	}
	return nil
}

// preparePairingSession persists one receiving Session reference per account
// replica. Repeated pairing after interruption reuses its own durable credential.
func (a *ProviderAccount) preparePairingSession(ctx context.Context) (*session.SessionRef, error) {
	// An account already attached on this machine keeps its existing Session.
	controller, releaseController, err := session.ExLookupSessionController(ctx, a.t.p.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer releaseController.Release()
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		ref := entry.GetSessionRef()
		providerRef := ref.GetProviderResourceRef()
		if providerRef.GetProviderId() == a.GetProviderID() && providerRef.GetProviderAccountId() == a.GetAccountID() {
			return ref, nil
		}
	}

	// Serialize the binding with account-local mutations.
	release, err := a.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	store, releaseStore, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseStore()
	key := SobjectBindingKey("pairing-session")
	var ref *session.SessionRef
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, key)
		if err != nil || !found {
			return err
		}
		ref = &session.SessionRef{}
		if err := ref.UnmarshalVT(data); err != nil {
			return err
		}
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.GetProviderResourceRef().GetProviderAccountId() != a.GetAccountID() || ref.GetProviderResourceRef().GetProviderId() != a.GetProviderID() {
			return errors.New("pairing Session binding belongs to another account")
		}
		return nil
	})
	if err != nil || ref != nil {
		return ref, err
	}

	// Reserve the resumable reference before mounting once to persist its key.
	ref = &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id: ulid.NewULID(), ProviderId: a.GetProviderID(), ProviderAccountId: a.GetAccountID(),
	}}
	data, err := ref.MarshalVT()
	if err != nil {
		return nil, err
	}
	err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, key, data)
	})
	return ref, err
}

// retainPairedAccountPeer persists reconnect demand through the account lifecycle.
func (a *ProviderAccount) retainPairedAccountPeer(ctx context.Context, remotePeer peer.ID) error {
	transport := a.GetSessionTransport()
	if transport == nil {
		return ErrNoSessionTransport
	}
	if err := a.StartPersistentP2PSync(ctx, transport); err != nil {
		return err
	}
	return a.RetainP2PPeer(ctx, remotePeer)
}
