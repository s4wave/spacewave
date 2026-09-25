//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"os"
	"path"
	"testing"
	"time"

	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/sirupsen/logrus"
)

// TestBolt tests all tests on top of bolt.
func TestBolt(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	dir, err := os.MkdirTemp("", "hydra-test-bolt-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)

	tp := path.Join(dir, "database.boltdb")

	db, err := Open(tp, 0o644, nil, []byte("test-bucket"))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, db),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBoltPanicIsInvalidSnapshot(t *testing.T) {
	err := func() (err error) {
		defer recoverBoltTxPanic(&err)
		panic("page 2 already freed")
	}()
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("panic error = %v, want ErrInvalidSnapshot", err)
	}
}

// TestExecuteClosesWhenDatabaseRemoved tests that Execute keeps running
// through commits and returns ErrLockFileChanged once the database file is
// removed.
func TestExecuteClosesWhenDatabaseRemoved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbPath := path.Join(t.TempDir(), "database.boltdb")
	store, err := Open(dbPath, 0o644, nil, []byte("test-bucket"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- store.Execute(ctx) }()

	for i := range 10 {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("key"), []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-errCh:
		t.Fatalf("execute returned during commits: %v", err)
	default:
	}

	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; !errors.Is(err, bdberrors.ErrLockFileChanged) {
		t.Fatalf("execute error = %v, want ErrLockFileChanged", err)
	}
}
