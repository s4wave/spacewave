package sobject

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
)

// newTestSOHost creates a SOHost for testing with a mutable state pointer.
// The returned statePtr can be read to observe writes.
func newTestSOHost(ctx context.Context, state *SOState) (*SOHost, **SOState) {
	statePtr := &state
	ctr := ccontainer.NewCContainer[*SOState](state)
	host := NewSOHost(ctx,
		func(_ context.Context, _ string, released func()) (ccontainer.Watchable[*SOState], func(), error) {
			return ctr, func() {}, nil
		},
		func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(*statePtr, func(_ context.Context, s *SOState) error {
				*statePtr = s
				ctr.SetValue(s)
				return nil
			}, func() {}), nil
		},
		mockSharedObjectID,
	)
	return host, statePtr
}

func TestCreateSOInviteOp(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 1)
	owner := peers[0]
	ownerPriv, err := owner.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDStr := owner.GetPeerID().String()

	state := &SOState{
		Config: &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: ownerIDStr,
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
	}

	host, statePtr := newTestSOHost(ctx, state)

	msg, err := host.CreateSOInviteOp(
		ctx,
		ownerPriv,
		SOParticipantRole_SOParticipantRole_WRITER,
		"test-provider",
		"",
		5,
		nil,
	)
	if err != nil {
		t.Fatalf("CreateSOInviteOp: %v", err)
	}

	// Verify the returned message.
	if msg.GetInviteId() == "" {
		t.Fatal("invite_id should not be empty")
	}
	if msg.GetOwnerPeerId() != ownerIDStr {
		t.Fatalf("owner_peer_id mismatch: got %s, want %s", msg.GetOwnerPeerId(), ownerIDStr)
	}
	if msg.GetProviderId() != "test-provider" {
		t.Fatal("provider_id mismatch")
	}
	if len(msg.GetToken()) != 32 {
		t.Fatalf("expected 32-byte token, got %d", len(msg.GetToken()))
	}
	if msg.GetRole() != SOParticipantRole_SOParticipantRole_WRITER {
		t.Fatal("role mismatch")
	}
	if msg.GetMaxUses() != 5 {
		t.Fatal("max_uses mismatch")
	}
	if msg.GetSignature() == nil {
		t.Fatal("signature should not be nil")
	}

	// Verify the on-chain invite.
	written := *statePtr
	if len(written.GetInvites()) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(written.GetInvites()))
	}
	inv := written.GetInvites()[0]
	if inv.GetInviteId() != msg.GetInviteId() {
		t.Fatal("on-chain invite_id should match message invite_id")
	}
	if len(inv.GetTokenHash()) != 32 {
		t.Fatalf("expected 32-byte token_hash, got %d", len(inv.GetTokenHash()))
	}
	// Token hash should NOT equal the raw token.
	if string(inv.GetTokenHash()) == string(msg.GetToken()) {
		t.Fatal("token_hash should be a hash, not the raw token")
	}
}

func TestCreateInvite(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 1)
	owner := peers[0]
	ownerPriv, err := owner.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDStr := owner.GetPeerID().String()

	state := &SOState{
		Config: &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: ownerIDStr,
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
	}

	host, statePtr := newTestSOHost(ctx, state)

	invite := &SOInvite{
		InviteId:  "inv-1",
		TokenHash: []byte("hash-1"),
		Role:      SOParticipantRole_SOParticipantRole_WRITER,
		MaxUses:   5,
	}

	if err := host.CreateInvite(ctx, ownerPriv, invite); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	written := *statePtr
	if len(written.GetInvites()) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(written.GetInvites()))
	}
	if written.GetInvites()[0].GetInviteId() != "inv-1" {
		t.Fatal("invite_id mismatch")
	}

	// Duplicate should fail.
	err = host.CreateInvite(ctx, ownerPriv, invite)
	if err == nil {
		t.Fatal("expected error for duplicate invite_id")
	}
}

func TestRevokeInvite(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 1)
	owner := peers[0]
	ownerPriv, err := owner.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDStr := owner.GetPeerID().String()

	state := &SOState{
		Config: &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: ownerIDStr,
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		Invites: []*SOInvite{{
			InviteId:  "inv-1",
			TokenHash: []byte("hash-1"),
			Role:      SOParticipantRole_SOParticipantRole_WRITER,
		}},
	}

	host, statePtr := newTestSOHost(ctx, state)

	if err := host.RevokeInvite(ctx, ownerPriv, "inv-1"); err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	written := *statePtr
	if !written.GetInvites()[0].GetRevoked() {
		t.Fatal("invite should be revoked")
	}

	// Revoking again should fail.
	err = host.RevokeInvite(ctx, ownerPriv, "inv-1")
	if err == nil {
		t.Fatal("expected error for already-revoked invite")
	}

	// Revoking nonexistent should fail.
	err = host.RevokeInvite(ctx, ownerPriv, "inv-999")
	if err == nil {
		t.Fatal("expected error for nonexistent invite")
	}
}

func TestIncrementInviteUses(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 1)
	owner := peers[0]
	ownerPriv, err := owner.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDStr := owner.GetPeerID().String()

	state := &SOState{
		Config: &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: ownerIDStr,
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		Invites: []*SOInvite{{
			InviteId:  "inv-1",
			TokenHash: []byte("hash-1"),
			Role:      SOParticipantRole_SOParticipantRole_WRITER,
			MaxUses:   2,
		}},
	}

	host, statePtr := newTestSOHost(ctx, state)

	// First use.
	if err := host.IncrementInviteUses(ctx, ownerPriv, "inv-1"); err != nil {
		t.Fatalf("IncrementInviteUses: %v", err)
	}
	if (*statePtr).GetInvites()[0].GetUses() != 1 {
		t.Fatalf("expected uses=1, got %d", (*statePtr).GetInvites()[0].GetUses())
	}

	// Second use (hits max_uses).
	if err := host.IncrementInviteUses(ctx, ownerPriv, "inv-1"); err != nil {
		t.Fatalf("IncrementInviteUses: %v", err)
	}
	if (*statePtr).GetInvites()[0].GetUses() != 2 {
		t.Fatalf("expected uses=2, got %d", (*statePtr).GetInvites()[0].GetUses())
	}

	// Third use should fail (max_uses reached).
	err = host.IncrementInviteUses(ctx, ownerPriv, "inv-1")
	if err == nil {
		t.Fatal("expected error for max_uses reached")
	}
}

func TestIncrementInviteUsesExpired(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 1)
	owner := peers[0]
	ownerPriv, err := owner.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDStr := owner.GetPeerID().String()

	state := &SOState{
		Config: &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: ownerIDStr,
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		Invites: []*SOInvite{{
			InviteId:  "inv-expired",
			TokenHash: []byte("hash"),
			Role:      SOParticipantRole_SOParticipantRole_WRITER,
			ExpiresAt: timestamppb.New(time.Now().Add(-60 * time.Second)),
		}},
	}

	host, _ := newTestSOHost(ctx, state)

	err = host.IncrementInviteUses(ctx, ownerPriv, "inv-expired")
	if err == nil {
		t.Fatal("expected error for expired invite")
	}
}
