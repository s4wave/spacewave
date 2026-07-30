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
			ctx := context.Background()
			backend := test.store()
			store := NewFaultStore(backend, FaultBeforeCommit)
			var bodyAttempts []int

			err := kvtx.RunTransaction(ctx, true,
				func(ctx context.Context) (kvtx.Tx, error) {
					return store.NewTransaction(ctx, true)
				},
				func(ctx context.Context, tx kvtx.Tx) error {
					faultTx, ok := tx.(*faultTx)
					if !ok {
						t.Fatalf("transaction type = %T, want *faultTx", tx)
					}
					bodyAttempts = append(bodyAttempts, faultTx.Attempt())
					return tx.Set(ctx, []byte("key"), []byte("successful attempt"))
				})
			if err != nil {
				t.Fatal(err)
			}

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

			tx, err := backend.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
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
