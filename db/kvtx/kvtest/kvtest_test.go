package kvtx_kvtest

import (
	"context"
	"testing"

	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestKVTest(t *testing.T) {
	ctx := context.Background()
	store := sinmem.NewStore()
	if err := TestAll(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
}
func TestKVTestRetriesThroughFaultStore(t *testing.T) {
	ctx := context.Background()
	store := NewFaultStore(sinmem.NewStore(), FaultBeforeCommit)

	if err := TestAll(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
	if got := store.Opened(); got < 2 {
		t.Fatalf("opened transactions = %d, want at least 2", got)
	}
	if got := store.DiscardedAttempts(); len(got) == 0 || got[0] != 1 {
		t.Fatalf("discarded attempts = %v, want first attempt discarded", got)
	}
	if got := store.DelegatedCommits(); got == 0 {
		t.Fatal("delegated commits = 0, want a successful commit")
	}
}
