//go:build !tinygo

package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestTargetedInviteRedeemedAgain proves the named peer of a used targeted
// invite redeems it again for the same grant without counting a second use.
func TestTargetedInviteRedeemedAgain(t *testing.T) {
	// Mount the owner account and an independent reader account and session.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	rawProvider, providerRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	local := rawProvider.(*provider_local.Provider)
	readerRef, err := local.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	rawReader, releaseReader, err := local.AccessProviderAccount(ctx, readerRef.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseReader)
	reader := rawReader.(*provider_local.ProviderAccount)
	readerSession, releaseSession, err := reader.MountSession(ctx, readerRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)

	// Invite the reader's session to the owner's shared object for one use.
	ref, err := owner.CreateSharedObject(ctx, "redeem-again", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	host := object.(sobject.InviteHost)
	invite, err := host.CreateSOInviteOp(ctx, host.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, TargetPeerId: readerSession.GetPeerId().String(), MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), host.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}

	// Redeem the invite, then redeem it again as a canceled enrollment would.
	first, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatalf("redeem a used targeted invite again: %v", err)
	}
	if again.Grant == nil || again.SharedObjectID != first.SharedObjectID {
		t.Fatal("repeat redemption did not return the grant of the first")
	}
	if again.Grant.GetPeerId() != first.Grant.GetPeerId() {
		t.Fatalf("repeat grant peer = %q, want %q", again.Grant.GetPeerId(), first.Grant.GetPeerId())
	}

	// The repeat leaves the use count at the first redemption.
	state, err := host.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if uses := sobject.FindInvite(state, invite.GetInviteId()).GetUses(); uses != 1 {
		t.Fatalf("invite uses = %d, want 1", uses)
	}
}
