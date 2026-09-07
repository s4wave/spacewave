package provider_local_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

// TestPrepareDirectInviteUsesSessionTransport verifies an unpaired owner can
// receive its first invitation after the creation RPC ends and after sync restarts.
func TestPrepareDirectInviteUsesSessionTransport(t *testing.T) {
	ctx := t.Context()
	_, _, account, session, release := setupProviderAndSession(ctx, t)
	defer release()
	ref, err := account.CreateSharedObject(ctx, "invitation-owner", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObject()
	host := object.(sobject.InviteHost)
	invite, err := host.CreateSOInviteOp(ctx, host.GetPrivKey(), sobject.SOParticipantRole_SOParticipantRole_WRITER, "local", "", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	if err := account.PrepareDirectInvite(requestCtx, session.GetPrivKey(), host.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	cancelRequest()
	endpoint, err := invite.VerifyTransportPeer()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != session.GetPeerId() {
		t.Fatalf("invite targets %s instead of session %s", endpoint, session.GetPeerId())
	}
	if endpoint.String() == invite.GetOwnerPeerId() {
		t.Fatal("fixture does not distinguish session and storage identities")
	}
	if !account.IsP2PSyncRunning() {
		t.Fatal("invitation service stopped with its creation request")
	}

	// Pending invitations restore the existing account-owned sync service.
	account.StopP2PSync()
	if err := account.AutoStartP2PSyncIfNeeded(ctx, account.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	defer account.StopP2PSync()
	if !account.IsP2PSyncRunning() {
		t.Fatal("pending invitation did not restore sync service")
	}
}
