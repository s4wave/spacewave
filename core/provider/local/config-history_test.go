package provider_local

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// configHistoryFaultStore injects the shared commit fault only in write transactions.
type configHistoryFaultStore struct {
	// backend serves reads without consuming the write fault.
	backend kvtx.Store
	// writes injects one retryable failure before committing the real transaction.
	writes *kvtest.FaultStore
}

// NewTransaction routes writes through the fault injector and reads to the backend.
func (s *configHistoryFaultStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		return s.writes.NewTransaction(ctx, true)
	}
	return s.backend.NewTransaction(ctx, false)
}

// TestSOConfigHistoryRetainsHostChanges proves the real host persists signed
// lineage with configuration-only changes and reopens the committed state.
func TestSOConfigHistoryRetainsHostChanges(t *testing.T) {
	// Seed the genesis state in the real in-memory store.
	ctx := t.Context()
	backend := store_inmem.NewStore()
	initial, _, priv := seedTestGenesisState(t, backend)

	// Generate the reader key.
	readerKey, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := peer.IDFromPrivateKey(readerKey)
	if err != nil {
		t.Fatal(err)
	}

	// Open a host over a store whose first commit fails.
	faults := kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
	store := &configHistoryFaultStore{backend: backend, writes: faults}
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(ctx, store, "")
	host := sobject.NewSOHost(ctx, watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)

	// Add the reader through one retry.
	withReader := initial.GetConfig().CloneVT()
	withReader.Participants = append(withReader.Participants, &sobject.SOParticipantConfig{
		PeerId: reader.String(), Role: sobject.SOParticipantRole_SOParticipantRole_READER,
	})
	entry, err := sobject.BuildSOConfigChange(testSharedObjectID, initial.GetConfig(), withReader, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	if faults.Opened() != 2 || faults.DelegatedCommits() != 1 {
		t.Fatalf("write attempts = %d, commits = %d", faults.Opened(), faults.DelegatedCommits())
	}

	// The config-only change keeps the signed checkpoint.
	accepted, err := host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.GetCheckpoint().EqualVT(initial.GetCheckpoint()) {
		t.Fatal("configuration-only mutation changed the signed checkpoint")
	}

	// Read state and its supporting entries from one committed snapshot.
	read, err := backend.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	checkpointData, found, err := read.Get(ctx, SOConfigHistoryCheckpointKey(testSharedObjectID))
	if err != nil || !found {
		t.Fatalf("checkpoint found = %v, error = %v", found, err)
	}
	checkpoint := &sobject.SharedObjectConfig{}
	if err := checkpoint.UnmarshalVT(checkpointData); err != nil {
		t.Fatal(err)
	}
	if !checkpoint.EqualVT(initial.GetConfig()) {
		t.Fatal("history checkpoint is not the genesis configuration")
	}

	// The retained entry is the signed change.
	entryData, found, err := read.Get(ctx, SOConfigHistoryEntryKey(testSharedObjectID, accepted.GetConfig().GetConfigChainHash()))
	if err != nil || !found {
		t.Fatalf("entry found = %v, error = %v", found, err)
	}
	retained := &sobject.SOConfigChange{}
	if err := retained.UnmarshalVT(entryData); err != nil {
		t.Fatal(err)
	}
	if !retained.EqualVT(entry) {
		t.Fatal("retained entry differs from the signed mutation")
	}
	read.Discard()

	// A fresh provider instance must recover exactly the committed host state.
	watchAgain, lockAgain, syncAgain := NewObjectStoreSOStateFuncs(ctx, backend, "")
	reopened := sobject.NewSOHost(ctx, watchAgain, lockAgain, testSharedObjectID, syncAgain)
	t.Cleanup(reopened.ClearContext)
	got, err := reopened.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualVT(accepted) {
		t.Fatal("reopened state differs from committed state")
	}

	// Reject callback configuration tampering before publishing state or history.
	next, err := sobject.BuildSOConfigChange(testSharedObjectID, accepted.GetConfig(), accepted.GetConfig(), sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, next, func(state *sobject.SOState) error {
		state.Config.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_READER
		return nil
	}); err == nil {
		t.Fatal("accepted state that did not match signed configuration history")
	}
	got, err = host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualVT(accepted) {
		t.Fatal("failed write changed the visible host state")
	}

	// The next valid transition must extend retained history without replacing its checkpoint.
	if err := host.ApplyConfigChange(ctx, next, nil); err != nil {
		t.Fatal(err)
	}
	got, err = host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := sobject.VerifyConfigChainSuffix(testSharedObjectID, accepted.GetConfig(), got.GetConfig(), []*sobject.SOConfigChange{next}); err != nil {
		t.Fatal(err)
	}

	// The checkpoint is unchanged.
	read, err = backend.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	checkpointData, found, err = read.Get(ctx, SOConfigHistoryCheckpointKey(testSharedObjectID))
	if err != nil || !found {
		t.Fatalf("retained checkpoint found = %v, error = %v", found, err)
	}
	retainedCheckpoint := &sobject.SharedObjectConfig{}
	if err := retainedCheckpoint.UnmarshalVT(checkpointData); err != nil {
		t.Fatal(err)
	}
	if !retainedCheckpoint.EqualVT(checkpoint) {
		t.Fatal("later mutation replaced the history checkpoint")
	}

	// The new entry is retained.
	entryData, found, err = read.Get(ctx, SOConfigHistoryEntryKey(testSharedObjectID, got.GetConfig().GetConfigChainHash()))
	if err != nil || !found {
		t.Fatalf("later entry found = %v, error = %v", found, err)
	}
	retained = &sobject.SOConfigChange{}
	if err := retained.UnmarshalVT(entryData); err != nil {
		t.Fatal(err)
	}
	if !retained.EqualVT(next) {
		t.Fatal("later signed entry was not retained")
	}
	read.Discard()

	// Cloud projections may reorder membership without changing signed authority.
	ordered := got.GetConfig().CloneVT()
	projection, err := sobject.BuildSOConfigChange(testSharedObjectID, ordered, ordered, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, projection, func(state *sobject.SOState) error {
		slices.Reverse(state.Config.Participants)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	projected, err := host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := sobject.VerifyConfigChainSuffix(testSharedObjectID, ordered, projected.GetConfig(), []*sobject.SOConfigChange{projection}); err != nil {
		t.Fatal(err)
	}
}

// _ is a type assertion
var _ kvtx.Store = (*configHistoryFaultStore)(nil)
