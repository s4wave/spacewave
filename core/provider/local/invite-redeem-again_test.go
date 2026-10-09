//go:build !tinygo

package provider_local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
)

// targetedInvite is an owner's shared object holding a one-use writer invite
// that names an independent reader session.
type targetedInvite struct {
	reader        *provider_local.ProviderAccount
	readerSession session.Session
	host          sobject.InviteHost
	invite        *sobject.SOInviteMessage
}

// newTargetedInvite mounts the owner account and an independent reader account
// and session, then invites the reader's session for one use.
func newTargetedInvite(ctx context.Context, t *testing.T) *targetedInvite {
	t.Helper()
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
	return &targetedInvite{reader: reader, readerSession: readerSession, host: host, invite: invite}
}

// TestTargetedInviteRedeemedAgain proves the named peer of a used targeted
// invite redeems it again for the same grant without counting a second use.
func TestTargetedInviteRedeemedAgain(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	f := newTargetedInvite(ctx, t)

	// Redeem the invite, then redeem it again as a canceled enrollment would.
	first, err := f.reader.JoinViaInvite(ctx, f.readerSession.GetPrivKey(), f.invite, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.reader.JoinViaInvite(ctx, f.readerSession.GetPrivKey(), f.invite, "")
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
	state, err := f.host.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if uses := sobject.FindInvite(state, f.invite.GetInviteId()).GetUses(); uses != 1 {
		t.Fatalf("invite uses = %d, want 1", uses)
	}
}

// TestTargetedInviteRefusedAfterRemoval proves removing the named peer closes
// its targeted invite, so it cannot redeem the invite to re-enroll.
func TestTargetedInviteRefusedAfterRemoval(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	f := newTargetedInvite(ctx, t)
	if _, err := f.reader.JoinViaInvite(ctx, f.readerSession.GetPrivKey(), f.invite, ""); err != nil {
		t.Fatal(err)
	}

	// The owner removes the named peer from the shared object.
	removed, err := sobject.RemoveSOParticipant(ctx, f.host.GetSOHost(), f.readerSession.GetPeerId().String(), f.host.GetPrivKey(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("named peer was not a participant")
	}
	state, err := f.host.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !sobject.FindInvite(state, f.invite.GetInviteId()).GetRevoked() {
		t.Fatal("removal left the named peer's invite live")
	}

	// The repeat redemption is refused.
	_, err = f.reader.JoinViaInvite(ctx, f.readerSession.GetPrivKey(), f.invite, "")
	if err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("repeat redemption error = %v, want a revoked invite", err)
	}
}
