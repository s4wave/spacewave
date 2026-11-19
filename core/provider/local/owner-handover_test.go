package provider_local_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestNativeOwnerHandoverRestart proves that an owner restarted after its
// transfer, before it syncs the successor's commit, sees itself departing: its
// ownership reads handing over to the successor, and a second leave is refused
// rather than forking the config chain with another transfer.
func TestNativeOwnerHandoverRestart(t *testing.T) {
	// Native providers retain independent storage and device identities.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	reader, readerSession := setupPeerAccount(ctx, t, tb)

	// The owner hosts an object.
	ref, err := owner.CreateSharedObject(ctx, "handover-object", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject := mountOwned(ctx, t, owner, ref)
	id := object.GetSharedObjectID()

	// The owner serves a direct invitation over its session transport.
	invite, err := object.CreateSOInviteOp(ctx, object.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), object.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.StopSessionTransport)
	t.Cleanup(owner.StopP2PSync)

	// The reader joins, then goes offline so it cannot commit the departure.
	if err := reader.EnsureConfiguredSessionTransport(ctx, readerSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	reader.StopP2PSync()
	reader.StopSessionTransport()

	// The owner transfers to the reader's Session with its departure.
	successor := readerSession.GetPeerId().String()
	if err := owner.LeaveSharedObject(ctx, ownerSession.GetPrivKey(), id, successor); err != nil {
		t.Fatal(err)
	}

	// The owner restarts the object before the successor commits, and forgets
	// the peers it retained, as a process restart does.
	left, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	releaseObject()
	owner.ForgetP2PPeers()
	object, _ = mountOwned(ctx, t, owner, ref)

	// It reads handing over, with itself departing and the successor owning.
	// The state snapshot is published after the host state it follows, so a
	// read right after the remount can still hold the config before the leave.
	snap := waitSnapshotSeqno(ctx, t, object, left.GetConfig().GetConfigChainSeqno())
	own, err := snap.GetOwnership(ctx)
	if err != nil {
		t.Fatal(err)
	}
	self := object.GetPeerID().String()
	if own.State() != sobject.SOOwnershipHandingOver || !own.Departing(self) || own.Owns(self) || !own.Owns(successor) {
		t.Fatalf("restarted owner %s reads ownership %+v; want handing over to %s", self, own, successor)
	}
	if !slices.Contains(own.Owners(), successor) || !slices.Contains(own.SigningOwners(), self) {
		t.Fatalf("restarted owner reads owners %v signing %v", own.Owners(), own.SigningOwners())
	}

	// The restarted departing owner demands no link to its successor. The
	// successor commits the departure without it and holds no link back, so a
	// demand here closes after the link hold-open and redials forever.
	if err := owner.AutoStartP2PSyncIfNeeded(ctx, owner.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	if owner.RetainsP2PPeer(successor) {
		t.Fatalf("departing owner retains a link to its successor %s", successor)
	}

	// A second leave is refused instead of transferring again.
	before, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = owner.LeaveSharedObject(ctx, ownerSession.GetPrivKey(), id, "")
	if !errors.Is(err, sobject.ErrHandingOver) {
		t.Fatalf("second leave = %v; want ErrHandingOver", err)
	}
	after, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.GetConfig().GetConfigChainSeqno() != before.GetConfig().GetConfigChainSeqno() {
		t.Fatal("refused leave changed the config")
	}
}

// waitSnapshotSeqno waits for the state snapshot of object whose config is at
// seqno or later.
func waitSnapshotSeqno(ctx context.Context, t *testing.T, object *provider_local.SharedObject, seqno uint64) sobject.SharedObjectStateSnapshot {
	// Watch the state snapshots.
	t.Helper()
	snaps, release, err := object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Wait for one at seqno.
	snap, err := snaps.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		config, err := snap.GetConfig(ctx)
		return err == nil && config.GetConfigChainSeqno() >= seqno, err
	}, nil)
	if err != nil {
		t.Fatalf("state snapshot did not reach config %d: %v", seqno, err)
	}
	return snap
}

// mountOwned mounts ref on account and returns the object and a release that
// runs once, also at cleanup.
func mountOwned(ctx context.Context, t *testing.T, account *provider_local.ProviderAccount, ref *sobject.SharedObjectRef) (*provider_local.SharedObject, func()) {
	// Mount the object.
	t.Helper()
	mounted, release, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Release it once, whichever comes first.
	var released bool
	releaseOnce := func() {
		if !released {
			released = true
			release()
		}
	}
	t.Cleanup(releaseOnce)
	return mounted.(*provider_local.SharedObject), releaseOnce
}
