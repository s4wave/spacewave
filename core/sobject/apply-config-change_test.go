package sobject

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// signConfigChange replaces the signatures of entry with one by privKey.
func signConfigChange(t *testing.T, entry *SOConfigChange, privKey crypto.PrivKey) {
	t.Helper()
	entry.Signatures = nil
	if err := signSOConfigChange(entry, privKey); err != nil {
		t.Fatalf("sign config change: %v", err)
	}
}

func TestApplyConfigChange(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 2)
	owner := peers[0]
	newPeer := peers[1]
	ownerPriv, err := owner.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerIDStr := owner.GetPeerID().String()
	newPeerIDStr := newPeer.GetPeerID().String()

	t.Run("genesis config change", func(t *testing.T) {
		// Start with an empty config (no chain hash).
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{{
					PeerId: ownerIDStr,
					Role:   SOParticipantRole_SOParticipantRole_OWNER,
				}},
			},
		}

		// Host the state and capture the written state.
		var written *SOState
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, s *SOState, _ ...*SOConfigChange) error {
				written = s
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// Build genesis config change (seqno 0, empty previous_hash).
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}
		signConfigChange(t, entry, ownerPriv)

		// The host applies the change and writes the new config.
		if err := host.ApplyConfigChange(ctx, entry, nil); err != nil {
			t.Fatalf("ApplyConfigChange: %v", err)
		}
		if written == nil {
			t.Fatal("state was not written")
		}
		if len(written.GetConfig().GetConfigChainHash()) == 0 {
			t.Fatal("config_chain_hash not set after genesis")
		}
		if len(written.GetConfig().GetParticipants()) != 1 {
			t.Fatalf("expected 1 participant, got %d", len(written.GetConfig().GetParticipants()))
		}
	})

	t.Run("add participant via config change", func(t *testing.T) {
		// State with an existing chain hash from a prior genesis.
		genesisEntry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}
		signConfigChange(t, genesisEntry, ownerPriv)
		genesisHash, err := HashSOConfigChange(genesisEntry)
		if err != nil {
			t.Fatal(err)
		}

		// Start from an owner-only config.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
				ConfigChainHash: genesisHash,
			},
		}

		// Host the state and capture the written state.
		var written *SOState
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, s *SOState, _ ...*SOConfigChange) error {
				written = s
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// Build entry that adds a second participant.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    1,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
					{PeerId: newPeerIDStr, Role: SOParticipantRole_SOParticipantRole_WRITER},
				},
			},
			ChangeType:   SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
			PreviousHash: genesisHash,
		}
		signConfigChange(t, entry, ownerPriv)

		// The host applies the change and adds the participant.
		if err := host.ApplyConfigChange(ctx, entry, nil); err != nil {
			t.Fatalf("ApplyConfigChange: %v", err)
		}
		if written == nil {
			t.Fatal("state was not written")
		}
		if len(written.GetConfig().GetParticipants()) != 2 {
			t.Fatalf("expected 2 participants, got %d", len(written.GetConfig().GetParticipants()))
		}

		// Chain hash should advance.
		entryHash, _ := HashSOConfigChange(entry)
		if string(written.GetConfig().GetConfigChainHash()) != string(entryHash) {
			t.Fatal("config_chain_hash not updated to entry hash")
		}
	})

	t.Run("reject wrong previous_hash", func(t *testing.T) {
		// Start from an owner-only config.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
				ConfigChainHash: []byte("some-existing-hash"),
			},
		}

		// Host the state and fail on any write.
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, _ *SOState, _ ...*SOConfigChange) error {
				t.Fatal("should not write on rejection")
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// Build a change that names no previous hash.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    1,
			Config:         state.GetConfig().CloneVT(),
			ChangeType:     SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_UNKNOWN,
			PreviousHash:   []byte("wrong-hash"),
		}
		signConfigChange(t, entry, ownerPriv)

		// The host refuses it.
		err := host.ApplyConfigChange(ctx, entry, nil)
		if err == nil {
			t.Fatal("expected error for wrong previous_hash")
		}
	})

	t.Run("reject wrong seqno", func(t *testing.T) {
		// Build a genesis entry so we have a real chain hash.
		genesisEntry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}
		signConfigChange(t, genesisEntry, ownerPriv)
		genesisHash, err := HashSOConfigChange(genesisEntry)
		if err != nil {
			t.Fatal(err)
		}

		// Start from an owner-only config.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
				ConfigChainHash:  genesisHash,
				ConfigChainSeqno: 0,
			},
		}

		// Host the state and fail on any write.
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, _ *SOState, _ ...*SOConfigChange) error {
				t.Fatal("should not write on seqno rejection")
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// Entry with seqno 5 when expected is 1.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    5,
			Config:         state.GetConfig().CloneVT(),
			ChangeType:     SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_UNKNOWN,
			PreviousHash:   genesisHash,
		}
		signConfigChange(t, entry, ownerPriv)

		// The host refuses the skipped seqno.
		err = host.ApplyConfigChange(ctx, entry, nil)
		if err == nil {
			t.Fatal("expected error for wrong seqno")
		}
	})

	t.Run("seqno advances after apply", func(t *testing.T) {
		// Start from an owner-only config.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
		}

		// Host the state and keep each written state.
		var written *SOState
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, s *SOState, _ ...*SOConfigChange) error {
				written = s
				state = s
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// Genesis: seqno 0.
		entry0 := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}
		signConfigChange(t, entry0, ownerPriv)
		if err := host.ApplyConfigChange(ctx, entry0, nil); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		if written.GetConfig().GetConfigChainSeqno() != 0 {
			t.Fatalf("expected seqno 0 after genesis, got %d", written.GetConfig().GetConfigChainSeqno())
		}

		// Entry 1: seqno 1.
		entry1, err := BuildSOConfigChange(
			mockSharedObjectID,
			written.GetConfig(),
			written.GetConfig(),
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_UNKNOWN,
			ownerPriv,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if entry1.GetConfigSeqno() != 1 {
			t.Fatalf("BuildSOConfigChange should set seqno 1, got %d", entry1.GetConfigSeqno())
		}
		if err := host.ApplyConfigChange(ctx, entry1, nil); err != nil {
			t.Fatalf("entry1: %v", err)
		}
		if written.GetConfig().GetConfigChainSeqno() != 1 {
			t.Fatalf("expected seqno 1 after entry1, got %d", written.GetConfig().GetConfigChainSeqno())
		}
	})

	t.Run("reject non-owner signer", func(t *testing.T) {
		// Sign as the writer.
		newPeerPriv, err := newPeer.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Start from an owner and a writer.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{PeerId: ownerIDStr, Role: SOParticipantRole_SOParticipantRole_OWNER},
					{PeerId: newPeerIDStr, Role: SOParticipantRole_SOParticipantRole_WRITER},
				},
			},
		}

		// Host the state and fail on any write.
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, _ *SOState, _ ...*SOConfigChange) error {
				t.Fatal("should not write when signer is not OWNER")
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// Build a change.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config:         state.GetConfig().CloneVT(),
			ChangeType:     SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_UNKNOWN,
		}

		// Sign with WRITER, not OWNER.
		signConfigChange(t, entry, newPeerPriv)

		// The host refuses the writer signature.
		err = host.ApplyConfigChange(ctx, entry, nil)
		if err == nil {
			t.Fatal("expected error for non-owner signer")
		}
	})

	t.Run("allow self-enroll peer for same entity", func(t *testing.T) {
		// Sign as the new peer of the same entity.
		newPeerPriv, err := newPeer.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Record the owner genesis.
		genesisEntry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}
		signConfigChange(t, genesisEntry, ownerPriv)
		genesisHash, err := HashSOConfigChange(genesisEntry)
		if err != nil {
			t.Fatal(err)
		}

		// Start from the genesis config.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
				},
				ConfigChainHash:  genesisHash,
				ConfigChainSeqno: 0,
			},
		}

		// Host the state and capture the written state.
		var written *SOState
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, s *SOState, _ ...*SOConfigChange) error {
				written = s
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// The new peer enrolls itself as a reader.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    1,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
					{
						PeerId:   newPeerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
				},
				ConfigChainHash:  genesisHash,
				ConfigChainSeqno: 0,
			},
			ChangeType:   SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER,
			PreviousHash: genesisHash,
		}
		signConfigChange(t, entry, newPeerPriv)

		// The host applies the enrollment.
		if err := host.ApplyConfigChange(ctx, entry, nil); err != nil {
			t.Fatalf("ApplyConfigChange: %v", err)
		}
		if written == nil {
			t.Fatal("state was not written")
		}
		if len(written.GetConfig().GetParticipants()) != 2 {
			t.Fatalf("expected 2 participants, got %d", len(written.GetConfig().GetParticipants()))
		}
	})

	t.Run("reject self-enroll role escalation", func(t *testing.T) {
		// Sign as the new peer of the same entity.
		newPeerPriv, err := newPeer.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Start with a writer of the entity.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_WRITER,
						EntityId: "acct-1",
					},
				},
			},
		}

		// Host the state and fail on any write.
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, _ *SOState, _ ...*SOConfigChange) error {
				t.Fatal("should not write on self-enroll escalation")
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// The new peer enrolls itself above its entity role.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_WRITER,
						EntityId: "acct-1",
					},
					{
						PeerId:   newPeerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER,
		}
		signConfigChange(t, entry, newPeerPriv)

		// The host refuses the escalation.
		err = host.ApplyConfigChange(ctx, entry, nil)
		if err == nil {
			t.Fatal("expected self-enroll escalation error")
		}
	})

	t.Run("reject self-enroll cross entity", func(t *testing.T) {
		// Sign as a peer of another entity.
		newPeerPriv, err := newPeer.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Start from an owner-only config.
		state := &SOState{
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
				},
			},
		}

		// Host the state and fail on any write.
		host := NewSOHost(nil, nil, func(_ context.Context, _ string) (SOStateLock, error) {
			return NewSOStateLock(state, func(_ context.Context, _ *SOState, _ ...*SOConfigChange) error {
				t.Fatal("should not write on cross-entity self-enroll")
				return nil
			}, func() {}), nil
		}, mockSharedObjectID)

		// The peer enrolls itself under the owner entity.
		entry := &SOConfigChange{
			SharedObjectId: mockSharedObjectID,
			ConfigSeqno:    0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{
					{
						PeerId:   ownerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_OWNER,
						EntityId: "acct-1",
					},
					{
						PeerId:   newPeerIDStr,
						Role:     SOParticipantRole_SOParticipantRole_READER,
						EntityId: "acct-2",
					},
				},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER,
		}
		signConfigChange(t, entry, newPeerPriv)

		// The host refuses the enrollment.
		err = host.ApplyConfigChange(ctx, entry, nil)
		if err == nil {
			t.Fatal("expected cross-entity self-enroll error")
		}
	})
}
