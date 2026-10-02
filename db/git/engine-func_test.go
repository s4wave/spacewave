package hydra_git

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v6/storage/memory"
)

// TestNewFuncEngine checks that the engine opens transactions through its
// function with the requested write flag.
func TestNewFuncEngine(t *testing.T) {
	// Build an engine that records each write flag.
	ctx := context.Background()
	var writes []bool
	eng := NewFuncEngine(func(ctx context.Context, write bool) (Tx, error) {
		writes = append(writes, write)
		return NewFuncTx(
			&testStorer{Storage: memory.NewStorage()},
			nil,
			nil,
		), nil
	})

	// Open a write transaction and check the recorded flag.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if tx == nil {
		t.Fatal("expected tx")
	}
	if len(writes) != 1 || !writes[0] {
		t.Fatalf("unexpected write flags %v", writes)
	}
}

// TestNewFuncTxDiscardOnce checks that Discard runs the discard function once.
func TestNewFuncTxDiscardOnce(t *testing.T) {
	// Build a transaction that counts discards.
	var discards int
	tx := NewFuncTx(
		&testStorer{Storage: memory.NewStorage()},
		nil,
		func() { discards++ },
	)

	// Discard twice and expect one call.
	tx.Discard()
	tx.Discard()
	if discards != 1 {
		t.Fatalf("expected single discard, got %d", discards)
	}
}

// testStorer is a writable in-memory Git storer.
type testStorer struct {
	*memory.Storage
}

// GetReadOnly reports that the storer accepts writes.
func (t *testStorer) GetReadOnly() bool { return false }
