package hydra_git

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v6/storage/memory"
)

func TestNewFuncEngine(t *testing.T) {
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

func TestNewFuncTxDiscardOnce(t *testing.T) {
	var discards int

	tx := NewFuncTx(
		&testStorer{Storage: memory.NewStorage()},
		nil,
		func() { discards++ },
	)
	tx.Discard()
	tx.Discard()
	if discards != 1 {
		t.Fatalf("expected single discard, got %d", discards)
	}
}

type testStorer struct {
	*memory.Storage
}

func (t *testStorer) GetReadOnly() bool { return false }
