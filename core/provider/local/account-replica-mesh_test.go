package provider_local

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/pairing"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// TestAccountReplicaMesh verifies that the first machine has no special role
// after a replica has a durable copy: B can enroll C, and A catches up later.
func TestAccountReplicaMesh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	network := inproc.NewNetwork()
	var originals []*ProviderAccount
	var sessions []*Session
	for range 3 {
		_, _, account, sess, release := setupProviderAndSessionInternal(ctx, t)
		defer release()
		if err := account.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
			t.Fatal(err)
		}
		account.StopSessionTransport()
		account.t.p.localNetwork = transport.WithInprocNetwork(network)
		if err := account.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
			t.Fatal(err)
		}
		originals = append(originals, account)
		sessions = append(sessions, sess)
	}
	a := originals[0]
	spaceRef, err := a.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, a, spaceRef)
	b, bSession := enrollMeshReplica(ctx, t, a, sessions[0], originals[1], sessions[1])
	waitReplicaCopy(ctx, t, b, spaceRef)

	// Recreate the receiver's workers while the original account is offline.
	a.StopP2PSync()
	a.StopSessionTransport()
	b.StopP2PSync()
	if err := b.StartPersistentP2PSync(ctx, b.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	waitReplicaCopy(ctx, t, b, spaceRef)
	c, cSession := enrollMeshReplica(ctx, t, b, bSession, originals[2], sessions[2])
	waitReplicaCopy(ctx, t, c, spaceRef)
	so, releaseSO, err := c.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()
	data, found, err := so.GetBlockStore().(*BlockStore).store.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("third replica lacks the original payload: found=%v err=%v", found, err)
	}
	source, releaseSource, err := a.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSource()
	assertSameBlockGraph(ctx, t, source.GetBlockStore(), so.GetBlockStore().(*BlockStore).store, replicaWorldHead(ctx, t, source).GetRootRef())

	// A Space created on the third replica reaches B without A or new pairing.
	later, err := c.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	waitReplicaObject(ctx, t, b, later)
	if err := a.EnsureConfiguredSessionTransport(ctx, sessions[0].GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if err := a.StartPersistentP2PSync(ctx, a.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	waitReplicaObject(ctx, t, a, later)
	settings, err := a.readAccountSettings(ctx)
	if err != nil || len(settings.GetSessions()) != 3 {
		t.Fatalf("returning original did not learn all three Sessions: %v, %v", settings, err)
	}
	if err := b.UnlinkDevice(ctx, cSession.GetPeerId()); err != nil {
		t.Fatal(err)
	}
	// Remaining replicas must share one verified revocation lineage, including
	// for a Space originally created by the removed Session.
	for _, ref := range []*sobject.SharedObjectRef{spaceRef, later} {
		ownerState := getSOState(ctx, t, b, ref, ref.GetProviderResourceRef().GetId())
		replicaSO, release, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		states, releaseStates, err := replicaSO.(*SharedObject).GetSOHost().GetSOStateCtr(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseStates()
		if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
			return bytes.Equal(state.GetConfig().GetConfigChainHash(), ownerState.GetConfig().GetConfigChainHash()), nil
		}, nil); err != nil {
			t.Fatalf("revocation did not converge to one configuration lineage: %v", err)
		}
	}
}

// enrollMeshReplica exercises approved provider enrollment directly; bilateral
// rejection and durable UI Session registration are covered by the duplex test.
func enrollMeshReplica(ctx context.Context, t *testing.T, source *ProviderAccount, sourceSession session.Session, receiving *ProviderAccount, receivingSession session.Session) (*ProviderAccount, *Session) {
	t.Helper()
	settings, err := source.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := source.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	offer := &pairing.AccountOffer{AccountId: source.GetAccountID(), SettingsId: settings.GetProviderResourceRef().GetId(), StoragePeerId: storage.GetPeerID().String(), OperationId: ulid.NewULID()}
	account, releaseAccount, err := receiving.t.p.AccessProviderAccount(ctx, source.GetAccountID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAccount)
	replica := account.(*ProviderAccount)
	ref := sourceSession.GetSessionRef().CloneVT()
	ref.ProviderResourceRef.Id = ulid.NewULID()
	sess, releaseSession, err := replica.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)
	identity, err := replica.buildPairingIdentity(ctx, offer, sess, sourceSession.GetPeerId(), receivingSession.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.ValidateIdentity(offer, identity, sourceSession.GetPeerId(), receivingSession.GetPeerId()); err != nil {
		t.Fatal(err)
	}
	if err := replica.bindPairingSettings(ctx, offer); err != nil {
		t.Fatal(err)
	}
	if err := source.registerPairingReplicas(ctx, &pairing.Enrollment{Offer: offer, Identity: identity}, sourceSession.GetPeerId(), receivingSession.GetPeerId()); err != nil {
		t.Fatal(err)
	}
	for _, entry := range source.soListCtr.GetValue().GetSharedObjects() {
		object, err := source.enrollPairingObject(ctx, entry, identity)
		if err != nil {
			t.Fatal(err)
		}
		if err := replica.installPairingObject(ctx, offer, object, sourceSession.GetPeerId()); err != nil {
			t.Fatal(err)
		}
	}
	if err := replica.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if err := source.retainPairedAccountPeer(ctx, sess.GetPeerId()); err != nil {
		t.Fatal(err)
	}
	if err := replica.retainPairedAccountPeer(ctx, sourceSession.GetPeerId()); err != nil {
		t.Fatal(err)
	}
	return replica, sess.(*Session)
}

func waitReplicaCopy(ctx context.Context, t *testing.T, account *ProviderAccount, ref *sobject.SharedObjectRef) {
	t.Helper()
	for {
		progress, changed := account.GetAccountCopyProgress()
		for _, item := range progress {
			if item.GetObjectId() == ref.GetProviderResourceRef().GetId() && item.GetComplete() {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("replica copy did not complete: %v", progress)
		case <-changed:
		}
	}
}

func waitReplicaObject(ctx context.Context, t *testing.T, account *ProviderAccount, ref *sobject.SharedObjectRef) {
	t.Helper()
	if _, err := account.soListCtr.WaitValueWithValidator(ctx, func(list *sobject.SharedObjectList) (bool, error) {
		for _, entry := range list.GetSharedObjects() {
			if entry.GetRef().EqualVT(ref) {
				return true, nil
			}
		}
		return false, nil
	}, nil); err != nil {
		t.Fatalf("replica did not discover the Space: %v", err)
	}
}

// replicaWorldHead returns the World head of a mounted Space.
func replicaWorldHead(ctx context.Context, t *testing.T, so sobject.SharedObject) *bucket.ObjectRef {
	t.Helper()
	states, release, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	inner, err := states.GetValue().GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := &sobject_world_engine.InnerState{}
	if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatal(err)
	}
	return head.GetHeadRef()
}

// assertSameBlockGraph checks that dst holds every block reachable from root in
// src with the same bytes and edges.
func assertSameBlockGraph(ctx context.Context, t *testing.T, src, dst block.StoreOps, root *block.BlockRef) {
	t.Helper()
	seen := map[string]bool{root.MarshalString(): true}
	queue := []*block.BlockRef{root}
	for len(queue) != 0 {
		ref := queue[0]
		queue = queue[1:]
		want, err := src.GetStoredBlock(ctx, ref)
		if err != nil || want == nil || !want.GetRefsKnown() {
			t.Fatalf("source block %s: stored=%v err=%v", ref.MarshalString(), want, err)
		}
		got, err := dst.GetStoredBlock(ctx, ref)
		if err != nil || got == nil || !got.GetRefsKnown() {
			t.Fatalf("replica block %s: stored=%v err=%v", ref.MarshalString(), got, err)
		}
		if !bytes.Equal(got.GetData(), want.GetData()) || !sameRefs(got.GetRefs(), want.GetRefs()) {
			t.Fatalf("replica block %s differs from its source", ref.MarshalString())
		}
		for _, child := range want.GetRefs() {
			if key := child.MarshalString(); !seen[key] {
				seen[key] = true
				queue = append(queue, child)
			}
		}
	}
	if len(seen) < 3 {
		t.Fatalf("source graph has only %d blocks", len(seen))
	}
}

// sameRefs compares two edge lists as sets.
func sameRefs(a, b []*block.BlockRef) bool {
	return slices.Equal(refKeys(a), refKeys(b))
}

// refKeys returns the sorted, distinct keys of refs.
func refKeys(refs []*block.BlockRef) []string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, ref.MarshalString())
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}
