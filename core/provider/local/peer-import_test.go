package provider_local

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestPeerImportRetainsAuthority exercises host acceptance through real local
// transactions, including failed access, retry, reopening and signed revocation.
func TestPeerImportRetainsAuthority(t *testing.T) {
	// Generate the owner key.
	ctx := t.Context()
	owner, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		t.Fatal(err)
	}

	// Generate the reader key.
	reader, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	readerID, err := peer.IDFromPrivateKey(reader)
	if err != nil {
		t.Fatal(err)
	}

	// Sign a genesis and root under an owner and a reader.
	initial := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			{PeerId: ownerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
			{PeerId: readerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_READER},
		}},
		Root: &sobject.SORoot{InnerSeqno: 1, Inner: []byte("held-root")},
	}
	genesis, err := sobject.BuildSOConfigChange(testSharedObjectID, initial.Config, initial.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	initial.Config, err = sobject.VerifyConfigChange(testSharedObjectID, initial.Config, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Root.SignInnerData(owner, testSharedObjectID, 1, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}

	// Seed the state in the real in-memory store.
	backend := store_inmem.NewStore()
	seed, err := backend.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(seed.Discard)

	// Commit the encoded state.
	data, err := initial.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Set(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID), data); err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	seed.Discard()

	// Open a host over a store whose first commit fails.
	faults := kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
	store := &configHistoryFaultStore{backend: backend, writes: faults}
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(ctx, store, readerID)
	host := sobject.NewSOHost(ctx, watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)

	// Verify an invite change into a candidate.
	change, err := sobject.BuildSOConfigChange(testSharedObjectID, initial.Config, initial.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := initial.CloneVT()
	candidate.Config, err = sobject.VerifyConfigChange(testSharedObjectID, initial.Config, change)
	if err != nil {
		t.Fatal(err)
	}

	// Reject inaccessible candidates before any durable or watched state changes.
	denied := errors.New("cannot decode root")
	if err := host.ImportPeerSnapshot(ctx, candidate, []*sobject.SOConfigChange{change}, readerID, func(context.Context, *sobject.SOState) error {
		return denied
	}); !errors.Is(err, denied) {
		t.Fatalf("access error = %v", err)
	}
	got, err := host.GetHostState(ctx)
	if err != nil || !got.EqualVT(initial) || faults.Opened() != 0 {
		t.Fatalf("failed access changed state or opened write: %v", err)
	}

	// A configuration-only import retries the atomic transaction and retains its suffix.
	if err := host.ImportPeerSnapshot(ctx, candidate, []*sobject.SOConfigChange{change}, readerID, nil); err != nil {
		t.Fatal(err)
	}
	if faults.Opened() != 2 || faults.DelegatedCommits() != 1 {
		t.Fatalf("attempts = %d, commits = %d", faults.Opened(), faults.DelegatedCommits())
	}

	// A reopened host holds the candidate and its suffix.
	watchAgain, lockAgain, syncAgain := NewObjectStoreSOStateFuncs(ctx, backend, readerID)
	reopened := sobject.NewSOHost(ctx, watchAgain, lockAgain, testSharedObjectID, syncAgain)
	t.Cleanup(reopened.ClearContext)
	got, err = reopened.GetHostState(ctx)
	if err != nil || !got.EqualVT(candidate) {
		t.Fatalf("reopened state mismatch: %v", err)
	}
	suffix, err := reopened.ReadConfigHistory(ctx, initial.Config.ConfigChainHash, got.Config.ConfigChainHash)
	if err != nil || len(suffix) != 1 || !suffix[0].EqualVT(change) {
		t.Fatalf("reopened suffix = %v: %v", suffix, err)
	}
	if err := sobject.VerifyConfigChainSuffix(testSharedObjectID, initial.Config, got.Config, suffix); err != nil {
		t.Fatal(err)
	}

	// A proven local removal commits without requiring a new root or decryptable grant.
	nextConfig := candidate.Config.CloneVT()
	nextConfig.Participants = nextConfig.Participants[:1]
	removal, err := sobject.BuildSOConfigChange(testSharedObjectID, candidate.Config, nextConfig, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	revoked := candidate.CloneVT()
	revoked.Config, err = sobject.VerifyConfigChange(testSharedObjectID, candidate.Config, removal)
	if err != nil {
		t.Fatal(err)
	}

	// Import the removal without a root; it reports the revocation.
	revoked.Root = nil
	if err := host.ImportPeerSnapshot(ctx, revoked, []*sobject.SOConfigChange{removal}, readerID, func(context.Context, *sobject.SOState) error {
		t.Error("revocation requested decryption")
		return denied
	}); !errors.Is(err, sobject.ErrParticipantRevoked) {
		t.Fatalf("revocation result = %v", err)
	}
	got, err = host.GetHostState(ctx)
	if err != nil || !got.Config.EqualVT(revoked.Config) || !got.Root.EqualVT(initial.Root) {
		t.Fatalf("revocation did not preserve root and commit authority: %v", err)
	}

	// A fresh storage read recovers only the last readable configuration and root.
	archived, err := backend.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	archiveData, found, err := archived.Get(ctx, readCheckpointKey(testSharedObjectID))
	archived.Discard()
	if err != nil || !found {
		t.Fatalf("read checkpoint missing after removal: %v", err)
	}
	checkpointState := &sobject.SOState{}
	if err := checkpointState.UnmarshalVT(archiveData); err != nil {
		t.Fatal(err)
	}
	if !checkpointState.Config.EqualVT(candidate.Config) || !checkpointState.Root.EqualVT(candidate.Root) {
		t.Fatal("read checkpoint did not retain the last readable snapshot")
	}

	// Explicit invitation authority replaces the upgrade checkpoint with its accepted head.
	rejoined := got.CloneVT()
	rejoined.Config = initial.Config.CloneVT()
	reauthorization, err := sobject.BuildSOConfigChange(testSharedObjectID, got.Config, rejoined.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	rejoined.Config, err = sobject.VerifyConfigChange(testSharedObjectID, got.Config, reauthorization)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.InstallInviteSnapshot(ctx, rejoined); err != nil {
		t.Fatal(err)
	}

	// The read checkpoint is gone.
	read, err := backend.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	if _, found, err := read.Get(ctx, readCheckpointKey(testSharedObjectID)); err != nil || found {
		t.Fatalf("readmission retained obsolete read checkpoint: found=%v err=%v", found, err)
	}

	// The invitation head is the new checkpoint.
	checkpointData, found, err := read.Get(ctx, SOConfigHistoryCheckpointKey(testSharedObjectID))
	if err != nil || !found {
		t.Fatalf("invitation checkpoint missing: %v", err)
	}
	checkpoint := &sobject.SharedObjectConfig{}
	if err := checkpoint.UnmarshalVT(checkpointData); err != nil {
		t.Fatal(err)
	}
	if !checkpoint.EqualVT(rejoined.Config) {
		t.Fatal("invitation state retained the previous trust checkpoint")
	}

	// The stored state matches the invitation.
	stateData, found, err := read.Get(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID))
	if err != nil || !found {
		t.Fatalf("invitation state missing: %v", err)
	}
	stored := &sobject.SOState{}
	if err := stored.UnmarshalVT(stateData); err != nil || !stored.EqualVT(rejoined) {
		t.Fatalf("invitation state and checkpoint disagree: %v", err)
	}
}
