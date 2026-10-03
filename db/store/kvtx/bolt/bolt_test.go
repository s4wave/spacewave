//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"os"
	"path"
	"strconv"
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
	// Prepare the context and logger for the Bolt store contract tests.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the key encoding used by the transaction store.
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Allocate a temporary directory for the Bolt database.
	dir, err := os.MkdirTemp("", "hydra-test-bolt-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)

	// Open the Bolt store in the temporary directory and close it after the contract tests.
	tp := path.Join(dir, "database.boltdb")
	db, err := Open(tp, 0o644, nil, []byte("test-bucket"))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	// Run the shared storage contracts through the logged transaction adapter.
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
	// Bound the Store execution test with a cancelable context.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Open the temporary Bolt store for database removal checks.
	dbPath := path.Join(t.TempDir(), "database.boltdb")
	store, err := Open(dbPath, 0o644, nil, []byte("test-bucket"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()

	// Run the Store directory watcher alongside the test.
	errCh := make(chan error, 1)
	go func() { errCh <- store.Execute(ctx) }()

	// Verify repeated commits leave the Store watcher running.
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

	// Remove the database and verify the Store reports its changed lock file.
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; !errors.Is(err, bdberrors.ErrLockFileChanged) {
		t.Fatalf("execute error = %v, want ErrLockFileChanged", err)
	}
}

// TestExecuteManyStores tests that more executing stores than the default
// Linux inotify instance limit keep running, and that removing one database
// stops only its Store.
func TestExecuteManyStores(t *testing.T) {
	// Bound the whole test.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Open more stores than the default limit of 128 inotify instances, split
	// across two directories.
	const count = 200
	dirs := []string{t.TempDir(), t.TempDir()}
	stores := make([]*Store, count)
	dbPaths := make([]string, count)
	for i := range stores {
		dbPaths[i] = path.Join(dirs[i%len(dirs)], strconv.Itoa(i)+".boltdb")
		store, err := Open(dbPaths[i], 0o644, nil, []byte("test-bucket"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.db.Close()
		stores[i] = store
	}

	// Execute every store until the context ends.
	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()
	errChs := make([]chan error, count)
	for i, store := range stores {
		errChs[i] = make(chan error, 1)
		go func() { errChs[i] <- store.Execute(execCtx) }()
	}

	// Removing the first database stops only its Store.
	if err := os.Remove(dbPaths[0]); err != nil {
		t.Fatal(err)
	}
	if err := <-errChs[0]; !errors.Is(err, bdberrors.ErrLockFileChanged) {
		t.Fatalf("execute error = %v, want ErrLockFileChanged", err)
	}

	// Canceling the context stops the remaining stores without a watcher
	// error.
	execCancel()
	for i, errCh := range errChs[1:] {
		if err := <-errCh; !errors.Is(err, context.Canceled) {
			t.Fatalf("store %d execute error = %v, want context.Canceled", i+1, err)
		}
	}
}
