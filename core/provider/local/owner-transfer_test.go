package provider_local_test

import (
	"context"
	"slices"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestNativeOwnerTransfer proves that a Space outlives its hosting owner: the
// first invitee's account commits the departure and hosts, and the other
// remaining account routes to it and has its operations admitted there.
func TestNativeOwnerTransfer(t *testing.T) {
	// Full peer sync runs natively under one deadline.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Three accounts: the owner, the first invitee, and a later invitee.
	_, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	_, _, first, firstSession, releaseFirst := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseFirst)
	_, _, second, secondSession, releaseSecond := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseSecond)

	// The owner hosts an object with a body that the test validates by hand.
	ref, err := owner.CreateSharedObject(ctx, "transfer-object", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	object := mounted.(*provider_local.SharedObject)
	id := object.GetSharedObjectID()

	// The owner serves a direct invitation over its session transport.
	invite, err := object.CreateSOInviteOp(ctx, object.GetPrivKey(), sobject.SOParticipantRole_SOParticipantRole_WRITER, "local", "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), object.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.StopSessionTransport)
	t.Cleanup(owner.StopP2PSync)

	// Every account runs a session transport reachable from the other two.
	for _, joiner := range []struct {
		account *provider_local.ProviderAccount
		session *provider_local.Session
	}{{first, firstSession}, {second, secondSession}} {
		if err := joiner.account.EnsureConfiguredSessionTransport(ctx, joiner.session.GetPrivKey()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(joiner.account.StopSessionTransport)
		t.Cleanup(joiner.account.StopP2PSync)
	}
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), first.GetSessionTransport())
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), second.GetSessionTransport())
	prepareSessionTransportBackends(ctx, t, first.GetSessionTransport(), second.GetSessionTransport())

	// Both invitees join in order, so the first one's session is the default successor.
	if _, err := first.JoinViaInvite(ctx, firstSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := second.JoinViaInvite(ctx, secondSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	successor := mountJoined(ctx, t, first, id)
	remaining := mountJoined(ctx, t, second, id)

	// The owner leaves while both invitees remain.
	before, err := object.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.LeaveSharedObject(ctx, ownerSession.GetPrivKey(), id, ""); err != nil {
		t.Fatal(err)
	}

	// The departed owner retains the last root it could read, before the transfer.
	checkpoint, err := object.GetSharedObjectReadCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint == nil {
		t.Fatal("departed owner retained no read checkpoint")
	}
	if seqno := checkpoint.Config.GetConfigChainSeqno(); seqno != before.GetConfig().GetConfigChainSeqno() {
		t.Fatalf("read checkpoint config seqno = %d, want %d", seqno, before.GetConfig().GetConfigChainSeqno())
	}

	// The successor's account commits the departure.
	states, releaseStates, err := successor.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	departed, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return !slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == object.GetPeerID().String()
		}), nil
	}, nil)
	if err != nil {
		t.Fatalf("successor did not commit the departure: %v", err)
	}

	// The same change makes the successor's storage identity the owning host.
	if !slices.ContainsFunc(departed.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
		return p.GetPeerId() == successor.GetPeerID().String() && sobject.IsOwner(p.GetRole())
	}) {
		t.Fatal("departure committed before the successor's storage identity became owner")
	}
	waitSOEndpoint(ctx, t, first, id, "")
	waitSOEndpoint(ctx, t, second, id, firstSession.GetPeerId().String())

	// The remaining writer's operation is admitted by the new host.
	localID, err := remaining.QueueOperation(ctx, []byte("after transfer"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return len(state.GetOps()) != 0, nil
	}, nil); err != nil {
		t.Fatalf("new host did not receive the operation: %v", err)
	}
	if err := successor.ProcessOperations(ctx, false, func(_ context.Context, _ sobject.SharedObjectStateSnapshot, _ []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
		data := []byte("accepted")
		results := make([]*sobject.SOOperationResult, len(ops))
		for i, op := range ops {
			results[i] = sobject.BuildSOOperationResult(op.GetPeerId(), op.GetNonce(), true, nil)
		}
		return &data, results, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := remaining.WaitOperation(ctx, localID); err != nil {
		t.Fatalf("operation was not admitted after the transfer: %v", err)
	}
}

// mountJoined mounts the object id that account joined.
func mountJoined(ctx context.Context, t *testing.T, account *provider_local.ProviderAccount, id string) *provider_local.SharedObject {
	// Find the joined entry in the account's list.
	t.Helper()
	entries := account.GetSOListCtr().GetValue().GetSharedObjects()
	index := slices.IndexFunc(entries, func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == id
	})
	if index == -1 {
		t.Fatal("joined object is absent from the provider list")
	}

	// Mount it for the rest of the test.
	mounted, release, err := account.MountSharedObject(ctx, entries[index].GetRef(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return mounted.(*provider_local.SharedObject)
}

// waitSOEndpoint waits until account routes object id to endpoint.
func waitSOEndpoint(ctx context.Context, t *testing.T, account *provider_local.ProviderAccount, id, endpoint string) {
	t.Helper()
	if _, err := account.GetSOListCtr().WaitValueWithValidator(ctx, func(list *sobject.SharedObjectList) (bool, error) {
		return slices.ContainsFunc(list.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
			return entry.GetRef().GetProviderResourceRef().GetId() == id && entry.GetTransportPeerId() == endpoint
		}), nil
	}, nil); err != nil {
		t.Fatalf("object endpoint did not become %q: %v", endpoint, err)
	}
}
