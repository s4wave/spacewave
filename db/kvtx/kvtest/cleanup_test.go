package kvtx_kvtest

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestFaultStoreCleanupOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := NewFaultStore(sinmem.NewStore(), FaultBeforeCommit)
	bodyCalls := 0

	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			bodyCalls++
			cancel()
			return tx.Set(ctx, []byte("key"), []byte("value"))
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if got, want := bodyCalls, 1; got != want {
		t.Fatalf("body calls = %d, want %d", got, want)
	}
	assertAttemptCleanup(t, store, 1, []int{1}, 0)
}

func TestFaultStoreCleanupOnBodyError(t *testing.T) {
	ctx := context.Background()
	store := NewFaultStore(sinmem.NewStore(), FaultBeforeCommit)
	bodyErr := errors.New("body failed")
	bodyCalls := 0

	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		},
		func(context.Context, kvtx.Tx) error {
			bodyCalls++
			return bodyErr
		})
	if !errors.Is(err, bodyErr) {
		t.Fatalf("error = %v, want body error", err)
	}
	if got, want := bodyCalls, 1; got != want {
		t.Fatalf("body calls = %d, want %d", got, want)
	}
	assertAttemptCleanup(t, store, 1, []int{1}, 0)
}

func assertAttemptCleanup(t *testing.T, store *FaultStore, opened int, discarded []int, delegatedCommits int) {
	t.Helper()
	if got := store.Opened(); got != opened {
		t.Fatalf("opened transactions = %d, want %d", got, opened)
	}
	if got := store.DiscardedAttempts(); !equalInts(got, discarded) {
		t.Fatalf("discarded attempts = %v, want %v", got, discarded)
	}
	if got := store.DelegatedCommits(); got != delegatedCommits {
		t.Fatalf("delegated commits = %d, want %d", got, delegatedCommits)
	}
}
