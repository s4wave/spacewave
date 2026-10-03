package kvtx_txcache

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestTXCache_Store(t *testing.T) {
	// Create the context and logger for the transaction-cache store checks.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Exercise the cache store through the shared key-value contract checks.
	var underlyingStore kvtx.Store = sinmem.NewStore()
	underlyingStore = kvtx_vlogger.NewVLogger(le, underlyingStore)
	tstore := NewStore(underlyingStore)
	if err := kvtx_kvtest.TestAll(ctx, tstore); err != nil {
		t.Fatal(err.Error())
	}
}

type invalidSnapshotCommitTx struct {
	kvtx.Tx
}

func (t *invalidSnapshotCommitTx) Commit(ctx context.Context) error {
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	return errors.Join(errors.New("rebuilt write transaction conflict"), kvtx.ErrInvalidSnapshot)
}

func TestTxCacheCommitPreservesInvalidSnapshot(t *testing.T) {
	// Open the underlying read transaction for the cache commit test.
	ctx := context.Background()
	store := sinmem.NewStore()
	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}

	// Construct a cache transaction whose rebuilt writer returns a snapshot conflict.
	tx, err := NewTxWithCbs(
		readTx,
		true,
		readTx.Discard,
		func() (kvtx.Tx, error) {
			writeTx, err := store.NewTransaction(ctx, true)
			if err != nil {
				return nil, err
			}
			return &invalidSnapshotCommitTx{Tx: writeTx}, nil
		},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Buffer a transaction value and attempt the conflicting commit.
	if err := tx.Set(ctx, []byte("key"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit(ctx)

	// Verify the cache commit preserves the invalid snapshot error.
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("commit error = %v, want ErrInvalidSnapshot", err)
	}
}
