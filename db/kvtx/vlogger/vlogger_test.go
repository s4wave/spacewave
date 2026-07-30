package kvtx_vlogger

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestVlogger(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	var underlyingStore kvtx.Store = sinmem.NewStore()
	vstore := NewVLogger(le, underlyingStore)
	if err := kvtx_kvtest.TestAll(ctx, vstore); err != nil {
		t.Fatal(err.Error())
	}
}

func TestKeyForLoggingRedactsKeyMaterial(t *testing.T) {
	const secret = "password=correct-horse-battery-staple"

	logBuf := bytes.NewBuffer(nil)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(logBuf)
	le := logrus.NewEntry(log)

	var underlyingStore kvtx.Store = sinmem.NewStore()
	vstore := NewVLogger(le, underlyingStore)
	tx, err := vstore.NewTransaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	if err := tx.Set(context.Background(), []byte(secret), []byte("value")); err != nil {
		t.Fatal(err)
	}

	output := logBuf.String()
	if strings.Contains(output, secret) {
		t.Fatalf("vlogger exposed key material in logs: %s", output)
	}
	if !strings.Contains(output, "len=") {
		t.Fatalf("vlogger did not include structural key summary: %s", output)
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
	return errors.Join(errors.New("wrapped logger commit conflict"), kvtx.ErrInvalidSnapshot)
}

func TestVloggerPreservesInvalidSnapshot(t *testing.T) {
	store := &typedCommitStore{Store: sinmem.NewStore()}
	log := logrus.New()
	vstore := NewVLogger(logrus.NewEntry(log), store)
	tx, err := vstore.NewTransaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	err = tx.Commit(context.Background())
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("commit error = %v, want ErrInvalidSnapshot", err)
	}
}
