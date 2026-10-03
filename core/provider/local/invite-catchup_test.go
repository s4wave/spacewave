package provider_local_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestInvitedProviderCatchup proves invitation checkpoint installation and
// encrypted operation convergence through the account-owned sync compositions.
func TestInvitedProviderCatchup(t *testing.T) {
	// Start an owner and a reader provider.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	defer releaseOwner()
	_, _, reader, readerSession, releaseReader := setupProviderAndSession(ctx, t)
	defer releaseReader()

	// The owner creates and mounts an object.
	ref, err := owner.CreateSharedObject(ctx, "invited-catchup", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObject()
	ownerObject := object.(*provider_local.SharedObject)

	// The owner prepares an invitation for the reader.
	invite, err := ownerObject.CreateSOInviteOp(ctx, ownerObject.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_READER, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), ownerObject.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionTransport()
	defer owner.StopP2PSync()
	if err := reader.EnsureConfiguredSessionTransport(ctx, readerSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	defer reader.StopSessionTransport()
	defer reader.StopP2PSync()

	// The invitation establishes the link. Preparing both backends avoids two
	// competing dials replacing the stream during the enrollment RPC.
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), reader.GetSessionTransport())
	joined, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatal(err)
	}

	// Find the joined object in the reader's list.
	var readerRef *sobject.SharedObjectRef
	for _, entry := range reader.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == joined.SharedObjectID {
			readerRef = entry.GetRef()
		}
	}
	if readerRef == nil {
		t.Fatal("invitation did not persist the shared object")
	}

	// Mount the replica and watch its state.
	replica, releaseReplica, err := reader.MountSharedObject(ctx, readerRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReplica()
	readerObject := replica.(*provider_local.SharedObject)
	states, releaseStates, err := readerObject.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()

	// Retire sync while the owner makes a signed configuration change.
	reader.StopP2PSync()
	checkpoint := states.GetValue().GetConfig().CloneVT()
	current, err := ownerObject.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	change, err := sobject.BuildSOConfigChange(joined.SharedObjectID, current.Config, current.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, ownerObject.GetPrivKey(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownerObject.GetSOHost().ApplyConfigChange(ctx, change, nil); err != nil {
		t.Fatal(err)
	}

	// Write new content as an operation while the reader is offline.
	want := []byte("readable content written while the other device is offline")
	localID, err := ownerObject.QueueOperation(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	ownerID := ownerObject.GetPeerID().String()

	// Restart sync; the reader accepts the operation and the config change.
	if err := reader.StartPersistentP2PSync(ctx, reader.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	accepted, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		set, err := state.OperationSet(joined.SharedObjectID)
		if err != nil {
			return false, err
		}
		return set.Find(ownerID, localID) != nil && state.GetConfig().GetConfigChainSeqno() == checkpoint.GetConfigChainSeqno()+1, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readerObject.GetSOHost().ReadConfigHistory(ctx, checkpoint.GetConfigChainHash(), accepted.GetConfig().GetConfigChainHash()); err != nil {
		t.Fatal(err)
	}

	// The new content becomes readable.
	readableStates, releaseReadable, err := readerObject.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReadable()
	if _, err := readableStates.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		// Wait for the owner's operation to decode to want.
		if snapshot == nil {
			return false, nil
		}
		set, err := snapshot.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		h := set.Find(ownerID, localID)
		if h == nil {
			return false, nil
		}
		content, err := snapshot.DecodeOperation(ctx, set.Get(h))
		return bytes.Equal(content, want), err
	}, nil); err != nil {
		t.Fatalf("readable content did not converge: %v", err)
	}
}
