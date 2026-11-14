package provider_local

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestMergeInviteSnapshotKeepsHeldOperations checks that a replica holding
// the object under an older config takes a newer invitation snapshot without
// losing its unshared operations, and that a stale invitation or one outside
// the held config chain leaves the held state alone.
func TestMergeInviteSnapshotKeepsHeldOperations(t *testing.T) {
	// Hold the genesis state with an operation the replica has not shared.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	backend := store_inmem.NewStore()
	held, genesis, owner := seedTestGenesisState(t, backend)
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(ctx, backend, "", nil)
	host := sobject.NewSOHost(ctx, watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)
	if _, _, err := host.AddLocalOperation(ctx, le, testStepFactorySet(), owner, []byte("unshared")); err != nil {
		t.Fatal(err)
	}

	// Name a writer the invitation owner adds.
	writer, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	writerID, err := peer.IDFromPrivateKey(writer)
	if err != nil {
		t.Fatal(err)
	}

	// The change puts the invitation owner one config change ahead.
	withWriter := held.GetConfig().CloneVT()
	withWriter.Participants = append(withWriter.Participants, &sobject.SOParticipantConfig{
		PeerId: writerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_WRITER,
	})
	addWriter, err := sobject.BuildSOConfigChange(testSharedObjectID, held.GetConfig(), withWriter, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Its snapshot records the invite this replica redeemed.
	candidate := held.CloneVT()
	candidate.Config, err = sobject.VerifyConfigChange(testSharedObjectID, held.GetConfig(), addWriter)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Invites = []*sobject.SOInvite{{InviteId: "fresh", MaxUses: 1, Uses: 1}}

	// Installing it adopts the newer config and invite and keeps the operation.
	if err := host.MergeInviteSnapshot(ctx, candidate, []*sobject.SOConfigChange{genesis, addWriter}); err != nil {
		t.Fatal(err)
	}
	got, err := host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetConfig().EqualVT(candidate.GetConfig()) {
		t.Fatal("installed state did not adopt the invitation config")
	}
	if sobject.FindInvite(got, "fresh") == nil {
		t.Fatal("installed state did not record the invite")
	}
	if len(got.GetOps()) != 1 {
		t.Fatalf("installed state holds %d operations, want the unshared one", len(got.GetOps()))
	}

	// An invitation behind the held config is refused.
	if err := host.MergeInviteSnapshot(ctx, held, []*sobject.SOConfigChange{genesis}); err == nil {
		t.Fatal("stale invitation was installed")
	}

	// An invitation from an unrelated config chain is refused.
	other, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	foreign, foreignGenesis, err := sobject.BuildGenesisSOState(le, testStepFactorySet(), testSharedObjectID, other, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.MergeInviteSnapshot(ctx, foreign, []*sobject.SOConfigChange{foreignGenesis}); err == nil {
		t.Fatal("foreign invitation was installed")
	}

	// Neither refusal changed the held state.
	unchanged, err := host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged.EqualVT(got) {
		t.Fatal("refused invitation changed the held state")
	}
}
