package kvtx_kvtest

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestFaultStoreInjectsOneCommitFailure(t *testing.T) {
	// Wrap an in-memory store with one commit failure.
	ctx := context.Background()
	store := NewFaultStore(sinmem.NewStore(), FaultBeforeCommit)

	// Write the first transaction and verify its injected commit failure.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("key"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("first commit error = %v, want invalid snapshot", err)
	}
	tx.Discard()

	// Verify the next transaction commits without another injected failure.
	tx, err = store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()

	// Verify both transaction attempts were opened and discarded.
	if got := store.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := store.Discarded(); got != 2 {
		t.Fatalf("discarded transactions = %d, want 2", got)
	}
	if got := store.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}
}
