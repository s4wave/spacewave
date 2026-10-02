package sobject

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestVerifyConfigChainSuffix exercises trust anchored in a receiver's held state.
func TestVerifyConfigChainSuffix(t *testing.T) {
	// Load three keys so every signature is real.
	peers := createMockPeers(t, 3)
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := peers[1].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	enrolling, err := peers[2].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// Sign the genesis of an owner and a reader.
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_READER, EntityId: "reader-account"},
	}}
	build := func(current, next *SharedObjectConfig, kind SOConfigChangeType, signer crypto.PrivKey) *SOConfigChange {
		t.Helper()
		entry, err := BuildSOConfigChange(mockSharedObjectID, current, next, kind, signer, nil)
		if err != nil {
			t.Fatal(err)
		}
		return entry
	}
	genesis := build(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner)
	checkpoint, err := VerifyConfigChange(mockSharedObjectID, initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	before := checkpoint.CloneVT()

	// Transport projections may reorder members without changing signed authority.
	reordered := checkpoint.CloneVT()
	slices.Reverse(reordered.Participants)
	if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, reordered, nil); err != nil {
		t.Fatalf("reordered checkpoint: %v", err)
	}

	// Transfer authority, then remove the previous owner under the new authority.
	promoted := checkpoint.CloneVT()
	promoted.Participants[1].Role = SOParticipantRole_SOParticipantRole_OWNER
	promotion := build(checkpoint, promoted, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner)
	intermediate, err := VerifyConfigChange(mockSharedObjectID, checkpoint, promotion)
	if err != nil {
		t.Fatal(err)
	}

	// The final projection may also differ in order from a signed suffix entry.
	reordered = intermediate.CloneVT()
	slices.Reverse(reordered.Participants)
	encodedPromotion, err := promotion.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, reordered, []*SOConfigChange{promotion}); err != nil {
		t.Fatalf("reordered suffix target: %v", err)
	}
	afterPromotion, err := promotion.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(encodedPromotion, afterPromotion) {
		t.Fatal("verification mutated the signed suffix")
	}

	// Continue the verified history under the promoted participant's authority.
	removed := intermediate.CloneVT()
	removed.Participants = removed.Participants[1:]
	removal := build(intermediate, removed, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, reader)
	candidate, err := VerifyConfigChange(mockSharedObjectID, intermediate, removal)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, candidate, []*SOConfigChange{promotion, removal}); err != nil {
		t.Fatal(err)
	}
	if !checkpoint.EqualVT(before) {
		t.Fatal("verification mutated the held checkpoint")
	}

	// A matching head authenticates the entire held configuration, not its hash alone.
	t.Run("same head", func(t *testing.T) {
		if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, checkpoint.CloneVT(), nil); err != nil {
			t.Fatal(err)
		}
		forged := checkpoint.CloneVT()
		forged.Participants[1].Role = SOParticipantRole_SOParticipantRole_OWNER
		if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, forged, nil); err == nil {
			t.Fatal("accepted forged same-head configuration")
		}
	})

	// Reject histories that cannot prove every transition from the held checkpoint.
	tests := []struct {
		// name identifies the rejected history.
		name string
		// change produces an independently signed invalid transition.
		change func() *SOConfigChange
	}{
		{"reader promotion", func() *SOConfigChange {
			return build(checkpoint, promoted, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, reader)
		}},
		{"wrong anchor", func() *SOConfigChange {
			entry := promotion.CloneVT()
			entry.PreviousHash[0] ^= 1
			signConfigChange(t, entry, owner)
			return entry
		}},
		{"sequence gap", func() *SOConfigChange {
			entry := promotion.CloneVT()
			entry.ConfigSeqno++
			signConfigChange(t, entry, owner)
			return entry
		}},
		{"self enrollment", func() *SOConfigChange {
			entry, err := BuildSelfEnrollPeerConfigChange(mockSharedObjectID, checkpoint, enrolling, peers[2].GetPeerID().String(), "reader-account", SOParticipantRole_SOParticipantRole_READER)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyConfigChange(mockSharedObjectID, checkpoint, entry); err != nil {
				t.Fatal(err)
			}
			return entry
		}},
		{"genesis", func() *SOConfigChange { return genesis }},
		{"missing entry", func() *SOConfigChange { return nil }},
		{"invalid signature", func() *SOConfigChange {
			entry := promotion.CloneVT()
			entry.Config.Participants[1].Role = SOParticipantRole_SOParticipantRole_WRITER
			return entry
		}},
		{"duplicate participant", func() *SOConfigChange {
			next := promoted.CloneVT()
			next.Participants = append(next.Participants, next.Participants[0].CloneVT())
			return build(checkpoint, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Match the claimed target so rejection must come from transition verification.
			entry := tt.change()
			var claimed *SharedObjectConfig
			if entry != nil {
				head, err := HashSOConfigChange(entry)
				if err != nil {
					t.Fatal(err)
				}
				claimed = configWithAppliedConfigChainHead(entry.GetConfig(), entry.GetConfigSeqno(), head)
			}
			if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, claimed, []*SOConfigChange{entry}); err == nil {
				t.Fatal("accepted invalid suffix")
			}
		})
	}

	// Replays and unsigned legacy anchors cannot establish a new trust epoch.
	if err := VerifyConfigChainSuffix(mockSharedObjectID, candidate, candidate, []*SOConfigChange{removal}); err == nil {
		t.Fatal("accepted replay")
	}
	if err := VerifyConfigChainSuffix(mockSharedObjectID, initial, checkpoint, []*SOConfigChange{genesis}); err == nil {
		t.Fatal("accepted empty checkpoint")
	}

	// A removed owner cannot sign.
	unauthorized := build(candidate, candidate, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner)
	if _, err := VerifyConfigChange(mockSharedObjectID, candidate, unauthorized); err == nil {
		t.Fatal("accepted a removed owner's signature")
	}

	// The sequence cannot wrap.
	exhausted := candidate.CloneVT()
	exhausted.ConfigChainSeqno = ^uint64(0)
	wrapped := build(exhausted, exhausted, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, reader)
	if _, err := VerifyConfigChange(mockSharedObjectID, exhausted, wrapped); err == nil {
		t.Fatal("accepted wrapped configuration sequence")
	}

	// The suffix must reach the target head.
	wrongTarget := candidate.CloneVT()
	wrongTarget.ConfigChainHash[0] ^= 1
	if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, wrongTarget, []*SOConfigChange{promotion, removal}); err == nil {
		t.Fatal("accepted wrong target head")
	}
}
