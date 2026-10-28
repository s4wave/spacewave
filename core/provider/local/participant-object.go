package provider_local

import (
	"context"
	stderrors "errors"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
)

// participantObjectProtocol carries checkpoint requests authorized by object participation.
const participantObjectProtocol = protocol.ID("alpha/participant-object/1")

// participantObjectBindingContext separates storage binding proofs from all
// other peer signatures.
const participantObjectBindingContext = "spacewave participant object storage binding v1"

// participantObjectFetchTimeout bounds one peer's answer to a checkpoint request.
const participantObjectFetchTimeout = 15 * time.Second

// FetchParticipantObject exports a checkpoint of a held object to a Session
// that participates in it. It adds the Session's proven storage identity at
// the Session's role, as an invite join adds both identities, so the
// requesting volume can hold and sync the object.
func (a *ProviderAccount) FetchParticipantObject(ctx context.Context, request *ParticipantObjectRequest) (*pairing.SharedObject, error) {
	// Resolve the calling Session and the listed object it names.
	stream, err := link.MustGetMountedStreamContext(ctx)
	if err != nil {
		return nil, err
	}
	session := stream.GetPeerID()
	id := request.GetObjectId()
	entries := a.soListCtr.GetValue().GetSharedObjects()
	idx := slices.IndexFunc(entries, func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == id
	})
	if id == "" || idx == -1 {
		return nil, sobject.ErrSharedObjectNotFound
	}
	entry := entries[idx]

	// Verify the storage identity signed its binding to the Session.
	storage, storagePub, err := verifyParticipantObjectBinding(id, session, request.GetStorageProof())
	if err != nil {
		return nil, err
	}

	// Require the Session to read the object under its current config.
	so, release, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
	if err != nil {
		return nil, err
	}
	defer release()
	local := so.(*SharedObject)
	state, err := local.soHost.GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	role := participantRole(state.GetConfig(), session.String())
	if !sobject.CanReadState(role) {
		return nil, errors.New("calling Session is not a reader of the object")
	}

	// Add the storage identity at the Session's role.
	if participantRole(state.GetConfig(), storage.String()) == sobject.SOParticipantRole_SOParticipantRole_UNKNOWN {
		owner, err := a.vol.GetPeer(ctx, true)
		if err != nil {
			return nil, err
		}
		key, err := owner.GetPrivKey(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := sobject.AddSOParticipant(ctx, local.soHost, id, key, owner.GetPeerID().String(), storage.String(), storagePub, role, "", ""); err != nil {
			return nil, errors.Wrap(err, "add storage identity")
		}
	}

	// Export the durable state and its config lineage.
	state, err = local.soHost.GetHostState(ctx)
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
	return &pairing.SharedObject{Entry: entry.CloneVT(), State: state.Clone(), ConfigLineage: lineage}, nil
}

// EnrollParticipantObject returns this account's ref to the object id. When
// the account does not hold the object, it asks each linked peer for a
// checkpoint and installs the first one a writer of via created. Only a writer
// of via could have created the object, so a peer cannot substitute an object
// of its own under the same ID.
func (a *ProviderAccount) EnrollParticipantObject(ctx context.Context, id string, meta *sobject.SharedObjectMeta, via string) (*sobject.SharedObjectRef, error) {
	// A held object needs no enrollment.
	ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), id, SobjectBlockStoreID(id))
	if listsSharedObject(a.soListCtr.GetValue(), id) {
		return ref, nil
	}

	// Read the writers of via, whose genesis signature the object must carry.
	writers, err := a.readObjectWriters(ctx, via)
	if err != nil {
		return nil, errors.Wrap(err, "read the writers of the enclosing object")
	}

	// Sign the storage identity's binding to this Session.
	st := a.GetSessionTransport()
	if st == nil || st.GetChildBus() == nil {
		return nil, errors.New("session transport is not running")
	}
	proof, err := a.signParticipantObjectBinding(ctx, id, st.GetPeerID())
	if err != nil {
		return nil, err
	}

	// Ask each known peer until one returns a checkpoint to install.
	var errs []error
	for _, remote := range a.knownP2PPeers(st) {
		object, err := a.fetchParticipantObject(ctx, st.GetChildBus(), st.GetPeerID(), remote, id, proof)
		if err == nil {
			err = verifyParticipantObjectOrigin(id, object, writers)
		}
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "peer %s", remote.String()))
			continue
		}

		// Install the checkpoint like an invite join, then sync it.
		if err := a.mountEnrolledSO(ctx, id, meta, "shared", object.GetState(), object.GetConfigLineage()); err != nil {
			return nil, err
		}
		a.RetrySharedObjectSync(id)
		return ref, nil
	}
	if len(errs) == 0 {
		return nil, errors.New("no known peer holds the object")
	}
	return nil, stderrors.Join(errs...)
}

// knownP2PPeers returns the peers linked to the session transport, then the
// other peers retained for P2P sync, without duplicates.
func (a *ProviderAccount) knownP2PPeers(st *transport.SessionTransport) []peer.ID {
	// Order the linked peers first, since they answer without a dial.
	var peers []peer.ID
	links, _ := st.GetLinkSnapshotsWithWait()
	for _, linked := range links {
		if !slices.Contains(peers, linked.RemotePeerID) {
			peers = append(peers, linked.RemotePeerID)
		}
	}

	// Append the retained peers that are not linked.
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, id := range a.p2pPeerIDs {
			if !slices.Contains(peers, id) {
				peers = append(peers, id)
			}
		}
	})
	return slices.DeleteFunc(peers, func(id peer.ID) bool { return id == st.GetPeerID() })
}

// fetchParticipantObject requests one peer's checkpoint of the object.
func (a *ProviderAccount) fetchParticipantObject(ctx context.Context, b bus.Bus, local, remote peer.ID, id string, proof *peer.Signature) (*pairing.SharedObject, error) {
	ctx, cancel := context.WithTimeout(ctx, participantObjectFetchTimeout)
	defer cancel()
	open := stream_srpc.NewOpenStreamFunc(b, participantObjectProtocol, local, remote, 0)
	return NewSRPCParticipantObjectServiceClient(srpc.NewClient(open)).FetchParticipantObject(ctx, &ParticipantObjectRequest{
		ObjectId: id, StorageProof: proof,
	})
}

// readObjectWriters returns the peers that may write the held object id.
func (a *ProviderAccount) readObjectWriters(ctx context.Context, id string) (map[string]bool, error) {
	// Mount the held object.
	if !listsSharedObject(a.soListCtr.GetValue(), id) {
		return nil, sobject.ErrSharedObjectNotFound
	}
	ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), id, SobjectBlockStoreID(id))
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	defer release()

	// Collect the participants whose role writes operations.
	state, err := so.(*SharedObject).soHost.GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	writers := make(map[string]bool)
	for _, participant := range state.GetConfig().GetParticipants() {
		if sobject.CanWriteOps(participant.GetRole()) {
			writers[participant.GetPeerId()] = true
		}
	}
	return writers, nil
}

// signParticipantObjectBinding signs the binding of this volume's storage
// identity to the Session for the object.
func (a *ProviderAccount) signParticipantObjectBinding(ctx context.Context, id string, session peer.ID) (*peer.Signature, error) {
	// Load the volume's storage key.
	storage, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	key, err := storage.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}

	// Sign the binding of this object to the Session.
	data, err := (&ParticipantObjectBinding{ObjectId: id, SessionPeerId: session.String()}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return peer.NewSignature(participantObjectBindingContext, key, hash.RecommendedHashType, data, true)
}

// verifyParticipantObjectBinding verifies a storage identity's signed binding
// to the Session for the object and returns that identity.
func verifyParticipantObjectBinding(id string, session peer.ID, proof *peer.Signature) (peer.ID, crypto.PubKey, error) {
	// Resolve the proof's public key.
	if proof == nil {
		return "", nil, errors.New("storage proof is required")
	}
	pub, err := proof.ParsePubKey()
	if err != nil || pub == nil {
		return "", nil, errors.New("storage proof public key is invalid")
	}

	// Verify the signature over the binding and return its signer.
	data, err := (&ParticipantObjectBinding{ObjectId: id, SessionPeerId: session.String()}).MarshalVT()
	if err != nil {
		return "", nil, err
	}
	valid, err := proof.VerifyWithPublic(participantObjectBindingContext, pub, data)
	if err != nil || !valid {
		return "", nil, errors.New("storage proof signature is invalid")
	}
	storage, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return "", nil, err
	}
	if storage == session {
		return "", nil, errors.New("storage proof names the Session itself")
	}
	return storage, pub, nil
}

// verifyParticipantObjectOrigin checks that the checkpoint's config history
// starts at a genesis that a writer in writers signed as an owner.
func verifyParticipantObjectOrigin(id string, object *pairing.SharedObject, writers map[string]bool) error {
	// Verify the lineage leads from its first change to the held config.
	lineage := object.GetConfigLineage()
	if object.GetState().GetConfig() == nil {
		return errors.New("checkpoint has no config")
	}
	if _, err := sobject.VerifyConfigLineage(id, object.GetState().GetConfig(), lineage); err != nil {
		return err
	}

	// Require the history to reach genesis, whose owners signed it.
	if len(lineage) == 0 || lineage[0].GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS {
		return errors.New("checkpoint history does not reach the object's genesis")
	}
	for _, participant := range lineage[0].GetConfig().GetParticipants() {
		if sobject.IsOwner(participant.GetRole()) && writers[participant.GetPeerId()] {
			return nil
		}
	}
	return errors.New("object was not created by a writer of the enclosing object")
}

// participantRole returns the role of peerID in cfg, or UNKNOWN when absent.
func participantRole(cfg *sobject.SharedObjectConfig, peerID string) sobject.SOParticipantRole {
	for _, participant := range cfg.GetParticipants() {
		if participant.GetPeerId() == peerID {
			return participant.GetRole()
		}
	}
	return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
}

// _ is a type assertion.
var (
	_ SRPCParticipantObjectServiceServer = (*ProviderAccount)(nil)
	_ sobject.ParticipantObjectEnroller  = (*ProviderAccount)(nil)
)
