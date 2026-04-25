package sobject

import (
	"bytes"
	"context"
	"testing"
)

func TestVerifyConfigChain(t *testing.T) {
	t.Run("accepts unsigned legacy genesis", func(t *testing.T) {
		peers := createMockPeers(t, 1)
		ownerID := peers[0].GetPeerID().String()

		err := VerifyConfigChain([]*SOConfigChange{{
			ConfigSeqno: 0,
			Config: &SharedObjectConfig{
				Participants: []*SOParticipantConfig{{
					PeerId: ownerID,
					Role:   SOParticipantRole_SOParticipantRole_OWNER,
				}},
			},
			ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		}})
		if err != nil {
			t.Fatalf("VerifyConfigChain returned error: %v", err)
		}
	})

	t.Run("rejects unsigned non-genesis change", func(t *testing.T) {
		peers := createMockPeers(t, 1)
		ownerID := peers[0].GetPeerID().String()

		err := VerifyConfigChain([]*SOConfigChange{
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

	t.Run("accepts add-participant followed by self-enroll peer", func(t *testing.T) {
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

		genesisConfig := &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId:   peers[0].GetPeerID().String(),
				Role:     SOParticipantRole_SOParticipantRole_OWNER,
				EntityId: "acct-1",
			}},
		}
		genesisEntry, err := BuildSOConfigChange(
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

		currentCfg := genesisConfig.CloneVT()
		currentCfg.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
		currentCfg.ConfigChainHash = genesisHash

		nextCfg := currentCfg.CloneVT()
		nextCfg.Participants = append(nextCfg.GetParticipants(), &SOParticipantConfig{
			PeerId:   peers[1].GetPeerID().String(),
			Role:     SOParticipantRole_SOParticipantRole_READER,
			EntityId: "acct-2",
		})
		addParticipantEntry, err := BuildSOConfigChange(
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

		rejoinCfg := nextCfg.CloneVT()
		rejoinCfg.ConfigChainSeqno = addParticipantEntry.GetConfigSeqno()
		rejoinCfg.ConfigChainHash = addParticipantHash
		selfEnrollEntry, err := BuildSelfEnrollPeerConfigChange(
			rejoinCfg,
			rejoinPriv,
			peers[2].GetPeerID().String(),
			"acct-2",
			SOParticipantRole_SOParticipantRole_READER,
		)
		if err != nil {
			t.Fatalf("build self-enroll entry: %v", err)
		}
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

		if err := VerifyConfigChain([]*SOConfigChange{
			genesisEntry,
			addParticipantEntry,
			selfEnrollEntry,
		}); err != nil {
			t.Fatalf("VerifyConfigChain returned error: %v", err)
		}
	})
}
