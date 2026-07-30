package kvtx_prefixer

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestPrefixer(t *testing.T) {
	ctx := context.Background()
	store := sinmem.NewStore()
	if err := kvtx_kvtest.TestAll(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
	prefixed := NewPrefixer(store, []byte("testing-prefix/"))
	if err := kvtx_kvtest.TestAll(ctx, prefixed); err != nil {
		t.Fatal(err.Error())
	}
}

type typedCommitStore struct {
	kvtx.Store
}

func (s *typedCommitStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &typedCommitTx{Tx: tx}, nil
}

type typedCommitTx struct {
	kvtx.Tx
}

func (t *typedCommitTx) Commit(ctx context.Context) error {
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	return errors.Join(errors.New("wrapped adapter commit conflict"), kvtx.ErrInvalidSnapshot)
}

func TestPrefixerPreservesInvalidSnapshot(t *testing.T) {
	store := &typedCommitStore{Store: sinmem.NewStore()}
	prefixed := NewPrefixer(store, []byte("prefix/"))
	tx, err := prefixed.NewTransaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	err = tx.Commit(context.Background())
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("commit error = %v, want ErrInvalidSnapshot", err)
	}
}
