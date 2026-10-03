package kvtx_kvtest

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestFaultStoreReplaysFreshTransactions(t *testing.T) {
	tests := []struct {
		name  string
		store func() kvtx.Store
	}{
		{
			name:  "native in-memory",
			store: func() kvtx.Store { return sinmem.NewStore() },
		},
		{
			name:  "hashmap adapter",
			store: func() kvtx.Store { return hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Wrap the selected backend with a commit failure and track body attempts.
			ctx := context.Background()
			backend := test.store()
			store := NewFaultStore(backend, FaultBeforeCommit)
			var bodyAttempts []int

			// Run a write transaction that must replay after its first commit failure.
			err := kvtx.RunTransaction(ctx, true,
				func(ctx context.Context) (kvtx.Tx, error) {
					return store.NewTransaction(ctx, true)
				},
				func(ctx context.Context, tx kvtx.Tx) error {
					// Require the fault transaction wrapper and record the current attempt.
					faultTx, ok := tx.(*faultTx)
					if !ok {
						t.Fatalf("transaction type = %T, want *faultTx", tx)
					}
					attempt := faultTx.Attempt()
					bodyAttempts = append(bodyAttempts, attempt)

					// Write a distinct value for the successful replay attempt.
					value := []byte("failed attempt")
					if attempt == 2 {
						value = []byte("successful attempt")
					}
					return tx.Set(ctx, []byte("key"), value)
				})
			if err != nil {
				t.Fatal(err)
			}

			// Verify fresh transaction attempts, cleanup, and one delegated commit.
			if got, want := bodyAttempts, []int{1, 2}; !equalInts(got, want) {
				t.Fatalf("body attempts = %v, want %v", got, want)
			}
			if got, want := store.Opened(), 2; got != want {
				t.Fatalf("opened transactions = %d, want %d", got, want)
			}
			if got, want := store.DiscardedAttempts(), []int{1, 2}; !equalInts(got, want) {
				t.Fatalf("discarded attempts = %v, want %v", got, want)
			}
			if got, want := store.DelegatedCommits(), 1; got != want {
				t.Fatalf("delegated commits = %d, want %d", got, want)
			}

			// Open a read transaction over the backend after replay succeeds.
			tx, err := backend.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()

			// Verify the backend holds the successful attempt value.
			value, found, err := tx.Get(ctx, []byte("key"))
			if err != nil {
				t.Fatal(err)
			}
			if !found || string(value) != "successful attempt" {
				t.Fatalf("committed value = %q, found = %t", value, found)
			}
		})
	}
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
