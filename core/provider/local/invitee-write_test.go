package provider_local_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestInviteeWriteReachesOwner proves that an operation a joined writer queues
// reaches the owner's copy of the shared object and decodes there.
func TestInviteeWriteReachesOwner(t *testing.T) {
	// Start an owner and an invitee provider.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	defer releaseOwner()
	_, _, invitee, inviteeSession, releaseInvitee := setupProviderAndSession(ctx, t)
	defer releaseInvitee()

	// The owner creates and mounts an object.
	ref, err := owner.CreateSharedObject(ctx, "invitee-write", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObject()
	ownerObject := mounted.(*provider_local.SharedObject)

	// The owner invites a writer over its session transport.
	invite, err := ownerObject.CreateSOInviteOp(ctx, ownerObject.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), ownerObject.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionTransport()
	defer owner.StopP2PSync()
	if err := invitee.EnsureConfiguredSessionTransport(ctx, inviteeSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	defer invitee.StopSessionTransport()
	defer invitee.StopP2PSync()
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), invitee.GetSessionTransport())
	joined, err := invitee.JoinViaInvite(ctx, inviteeSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatal(err)
	}
	inviteeObject := mountJoined(ctx, t, invitee, joined.SharedObjectID)

	// The invitee writes an operation.
	want := []byte("written on the joined device")
	localID, err := inviteeObject.QueueOperation(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	writer := inviteeObject.GetPeerID().String()

	// The owner holds the operation and decodes its content.
	states, releaseStates, err := ownerObject.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		set, err := snapshot.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		h := set.Find(writer, localID)
		if h == nil {
			return false, nil
		}
		content, err := snapshot.DecodeOperation(ctx, set.Get(h))
		return bytes.Equal(content, want), err
	}, nil); err != nil {
		t.Fatalf("owner did not receive the invitee's operation: %v", err)
	}
}
