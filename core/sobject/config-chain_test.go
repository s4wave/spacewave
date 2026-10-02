package sobject

import (
	"bytes"
	"context"
	"testing"

	"github.com/pkg/errors"
)

func TestVerifyConfigChain(t *testing.T) {
	t.Run("rejects unsigned genesis", func(t *testing.T) {
		peers := createMockPeers(t, 1)
		ownerID := peers[0].GetPeerID().String()

		err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{{
			ConfigSeqno: 0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{{
					PeerId: ownerID,
					Role:   SOParticipantRole_SOParticipantRole_OWNER,
				}},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}})
		if err == nil {
			t.Fatal("VerifyConfigChain accepted an unsigned genesis")
		}
	})

	t.Run("rejects unsigned non-genesis change", func(t *testing.T) {
		peers := createMockPeers(t, 1)
		ownerID := peers[0].GetPeerID().String()

		err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{
			{
				ConfigSeqno: 0,
				Config: &SharedObjectConfig{
					Participants: []*SOParticipantConfig{{
						PeerId: ownerID,
						Role:   SOParticipantRole_SOParticipantRole_OWNER,
					}},
				},
				ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
			},
			{
				ConfigSeqno: 1,
				Config: &SharedObjectConfig{
					Participants: []*SOParticipantConfig{{
						PeerId: ownerID,
						Role:   SOParticipantRole_SOParticipantRole_OWNER,
					}},
				},
				ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_UNKNOWN,
			},
		})
		if err == nil {
			t.Fatal("expected VerifyConfigChain to reject unsigned non-genesis entry")
		}
	})

	t.Run("rejects unsigned first entry without genesis change type", func(t *testing.T) {
		peers := createMockPeers(t, 1)
		ownerID := peers[0].GetPeerID().String()

		err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{{
			ConfigSeqno: 0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{{
					PeerId: ownerID,
					Role:   SOParticipantRole_SOParticipantRole_OWNER,
				}},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
		}})
		if err == nil {
			t.Fatal("expected VerifyConfigChain to reject non-genesis first entry")
		}
	})

	t.Run("rejects signed first entry without genesis change type", func(t *testing.T) {
		// Sign as the only owner.
		ctx := context.Background()
		peers := createMockPeers(t, 1)
		ownerPriv, err := peers[0].GetPrivKey(ctx)
		if err != nil {
			t.Fatalf("get owner private key: %v", err)
		}

		// Sign an add-participant change as the first entry.
		cfg := &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: peers[0].GetPeerID().String(),
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		}
		entry, err := BuildSOConfigChange(
			mockSharedObjectID,
			&SharedObjectConfig{},
			cfg,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
			ownerPriv,
			nil,
		)
		if err != nil {
			t.Fatalf("build first entry: %v", err)
		}

		// A chain must open with a genesis change.
		if err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{entry}); err == nil {
			t.Fatal("expected VerifyConfigChain to reject signed non-genesis first entry")
		}
	})

	t.Run("accepts add-participant followed by self-enroll peer", func(t *testing.T) {
		// Create the owner, an added reader and the reader's rejoining device.
		ctx := context.Background()
		peers := createMockPeers(t, 3)
		ownerPriv, err := peers[0].GetPrivKey(ctx)
		if err != nil {
			t.Fatalf("get owner private key: %v", err)
		}
		rejoinPriv, err := peers[2].GetPrivKey(ctx)
		if err != nil {
			t.Fatalf("get rejoin private key: %v", err)
		}

		// Sign a genesis that holds only the owner.
		genesisConfig := &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId:   peers[0].GetPeerID().String(),
				Role:     SOParticipantRole_SOParticipantRole_OWNER,
				EntityId: "acct-1",
			}},
		}
		genesisEntry, err := BuildSOConfigChange(
			mockSharedObjectID,
			&SharedObjectConfig{},
			genesisConfig,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
			ownerPriv,
			nil,
		)
		if err != nil {
			t.Fatalf("build genesis entry: %v", err)
		}
		genesisHash, err := HashSOConfigChange(genesisEntry)
		if err != nil {
			t.Fatalf("hash genesis entry: %v", err)
		}

		// Advance the config to the genesis head.
		currentCfg := genesisConfig.CloneVT()
		currentCfg.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
		currentCfg.ConfigChainHash = genesisHash

		// Have the owner add the reader with its recorded username.
		nextCfg := currentCfg.CloneVT()
		nextCfg.Participants = append(nextCfg.GetParticipants(), &SOParticipantConfig{
			PeerId:   peers[1].GetPeerID().String(),
			Role:     SOParticipantRole_SOParticipantRole_READER,
			EntityId: "acct-2",
			Username: "bob",
		})
		addParticipantEntry, err := BuildSOConfigChange(
			mockSharedObjectID,
			currentCfg,
			nextCfg,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
			ownerPriv,
			nil,
		)
		if err != nil {
			t.Fatalf("build add participant entry: %v", err)
		}
		addParticipantHash, err := HashSOConfigChange(addParticipantEntry)
		if err != nil {
			t.Fatalf("hash add participant entry: %v", err)
		}

		// Self-enroll the reader's second device on the same entity.
		rejoinCfg := nextCfg.CloneVT()
		rejoinCfg.ConfigChainSeqno = addParticipantEntry.GetConfigSeqno()
		rejoinCfg.ConfigChainHash = addParticipantHash
		selfEnrollEntry, err := BuildSelfEnrollPeerConfigChange(
			mockSharedObjectID,
			rejoinCfg,
			rejoinPriv,
			peers[2].GetPeerID().String(),
			"acct-2",
			SOParticipantRole_SOParticipantRole_READER,
		)
		if err != nil {
			t.Fatalf("build self-enroll entry: %v", err)
		}

		// Check that the self-enroll extends the chain and keeps the username.
		if selfEnrollEntry.GetConfigSeqno() != 2 {
			t.Fatalf("expected self-enroll seqno 2, got %d", selfEnrollEntry.GetConfigSeqno())
		}
		if selfEnrollEntry.GetConfig().GetConfigChainSeqno() != addParticipantEntry.GetConfigSeqno() {
			t.Fatalf(
				"expected self-enroll config seqno %d, got %d",
				addParticipantEntry.GetConfigSeqno(),
				selfEnrollEntry.GetConfig().GetConfigChainSeqno(),
			)
		}
		if !bytes.Equal(selfEnrollEntry.GetConfig().GetConfigChainHash(), addParticipantHash) {
			t.Fatalf("expected self-enroll config hash to preserve prior head")
		}
		enrolled := selfEnrollEntry.GetConfig().GetParticipants()[2]
		if enrolled.GetUsername() != "bob" {
			t.Fatalf("self-enrolled username = %q, want bob", enrolled.GetUsername())
		}

		// A self-enrolling peer may not rename its entity.
		renamedCfg := rejoinCfg.CloneVT()
		renamedCfg.Participants = append(renamedCfg.GetParticipants(), &SOParticipantConfig{
			PeerId:   peers[2].GetPeerID().String(),
			Role:     SOParticipantRole_SOParticipantRole_READER,
			EntityId: "acct-2",
			Username: "mallory",
		})
		renamedEntry, err := BuildSOConfigChange(
			mockSharedObjectID,
			rejoinCfg,
			renamedCfg,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER,
			rejoinPriv,
			nil,
		)
		if err != nil {
			t.Fatalf("build renamed self-enroll entry: %v", err)
		}
		if err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{
			genesisEntry,
			addParticipantEntry,
			renamedEntry,
		}); err == nil {
			t.Fatal("VerifyConfigChain accepted a self-enroll rename")
		}

		// Verify the honest chain end to end.
		if err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{
			genesisEntry,
			addParticipantEntry,
			selfEnrollEntry,
		}); err != nil {
			t.Fatalf("VerifyConfigChain returned error: %v", err)
		}
	})
}

func TestVerifyConfigChangeRequiresOwner(t *testing.T) {
	// Load the owner key.
	ctx := context.Background()
	peers := createMockPeers(t, 2)
	ownerPriv, err := peers[0].GetPrivKey(ctx)
	if err != nil {
		t.Fatalf("get owner private key: %v", err)
	}

	// Start from a signed genesis held by one owner and one writer.
	owner := &SOParticipantConfig{
		PeerId: peers[0].GetPeerID().String(),
		Role:   SOParticipantRole_SOParticipantRole_OWNER,
	}
	writer := &SOParticipantConfig{
		PeerId: peers[1].GetPeerID().String(),
		Role:   SOParticipantRole_SOParticipantRole_WRITER,
	}
	genesisConfig := &SharedObjectConfig{Participants: []*SOParticipantConfig{owner, writer}}
	genesis, err := BuildSOConfigChange(
		mockSharedObjectID,
		&SharedObjectConfig{},
		genesisConfig,
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		ownerPriv,
		nil,
	)
	if err != nil {
		t.Fatalf("build genesis entry: %v", err)
	}
	genesisHash, err := HashSOConfigChange(genesis)
	if err != nil {
		t.Fatalf("hash genesis entry: %v", err)
	}
	current := configWithAppliedConfigChainHead(genesisConfig, 0, genesisHash)

	// buildRemoval signs a REMOVE_PARTICIPANT change leaving the given participants.
	buildRemoval := func(remaining ...*SOParticipantConfig) *SOConfigChange {
		// Remove every participant not kept.
		next := current.CloneVT()
		next.Participants = remaining
		entry, err := BuildSOConfigChange(
			mockSharedObjectID,
			current,
			next,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
			ownerPriv,
			nil,
		)
		if err != nil {
			t.Fatalf("build removal entry: %v", err)
		}
		return entry
	}

	// The last owner may not leave while others remain.
	t.Run("rejects removing the last owner while others remain", func(t *testing.T) {
		entry := buildRemoval(writer)
		if _, err := VerifyConfigChange(mockSharedObjectID, current, entry); !errors.Is(err, ErrNoOwner) {
			t.Fatalf("expected ErrNoOwner, got %v", err)
		}
		if err := VerifyConfigChain(mockSharedObjectID, []*SOConfigChange{genesis, entry}); !errors.Is(err, ErrNoOwner) {
			t.Fatalf("expected chain to reject ownerless result, got %v", err)
		}
	})

	// Everyone may leave together.
	t.Run("accepts a terminal empty configuration", func(t *testing.T) {
		if _, err := VerifyConfigChange(mockSharedObjectID, current, buildRemoval()); err != nil {
			t.Fatalf("expected terminal departure to verify: %v", err)
		}
	})
}
