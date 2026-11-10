//go:build !js

package store_kvtx_sqlite

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
	store_test "github.com/s4wave/spacewave/db/store/test"
)

func newTempDBPath(t *testing.T, pattern string) string {
	// Create a temporary SQLite file and remove its database sidecars afterward.
	t.Helper()

	// Create a temporary file to use as the SQLite database path.
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		t.Fatal(err.Error())
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		_ = os.Remove(name)
		_ = os.Remove(name + "-wal")
		_ = os.Remove(name + "-shm")
	})
	return name
}

// TestSQLite tests all tests on top of SQLite.
func TestSQLite(t *testing.T) {
	// Set up the context for the SQLite store integration test.
	ctx := context.Background()

	// Construct the key codec used by the kvtx wrapper.
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Allocate a temporary on-disk database path for this test.
	tp := newTempDBPath(t, "hydra-test-sqlite-*.sqlite")

	// Open the SQLite store and arrange to close its database handle.
	db, err := Open(ctx, tp, "test_table")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Test basic functionality first
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Test basic operations
	if err := tx.Set(ctx, []byte("test"), []byte("value")); err != nil {
		t.Fatal(err.Error())
	}

	// Read the value just written through the SQLite transaction.
	val, found, err := tx.Get(ctx, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("Expected to find test key")
	}
	if string(val) != "value" {
		t.Fatalf("Expected 'value', got '%s'", string(val))
	}

	// Commit the basic key-value operation to the SQLite store.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Now test with kvtx wrapper

	// Exercise the generic kvtx adapter against the SQLite store.
	ktx := store_kvtx.NewKVTx(kvkey, db, nil).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSQLiteWithMode tests SQLite with file mode specification.
func TestSQLiteWithMode(t *testing.T) {
	// Set up the context for the SQLite file-mode test.
	ctx := context.Background()

	// Allocate a temporary database path for the mode-specific store.
	tp := newTempDBPath(t, "hydra-test-sqlite-mode-*.sqlite")

	// Open the store with the requested file permissions.
	db, err := OpenWithMode(ctx, tp, 0o644, "test_table_mode")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Test that we can create and use the database
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Write a key through the mode-configured SQLite transaction.
	if err := tx.Set(ctx, []byte("mode_test"), []byte("works")); err != nil {
		t.Fatal(err.Error())
	}

	// Commit the mode-specific write to SQLite.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSQLiteIterator tests specific iterator functionality.
func TestSQLiteIterator(t *testing.T) {
	// Set up the context for the SQLite iterator test.
	ctx := context.Background()

	// Allocate a temporary database path for prefix iteration.
	tp := newTempDBPath(t, "hydra-test-sqlite-iter-*.sqlite")

	// Open the SQLite store used by the iterator test.
	db, err := Open(ctx, tp, "test_iter")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Create transaction and add test data

	// Begin a write transaction for the iterator fixture records.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Add test data with known prefix

	// Define records with matching and nonmatching key prefixes.
	testData := map[string]string{
		"prefix:key1": "value1",
		"prefix:key2": "value2",
		"prefix:key3": "value3",
		"other:key1":  "other1",
	}

	// Write every fixture record to the SQLite transaction.
	for k, v := range testData {
		if err := tx.Set(ctx, []byte(k), []byte(v)); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Commit the fixture records before opening the read transaction.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Test prefix iteration

	// Open a read transaction for prefix iteration.
	tx, err = db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Create an iterator for keys under the requested prefix.
	iter := tx.Iterate(ctx, []byte("prefix:"), true, false)
	defer iter.Close()

	// Count and record every key returned by the prefix iterator.
	count := 0
	for iter.Next() {
		key := iter.Key()
		value, err := iter.Value()
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("Found key: %s, value: %s", string(key), string(value))
		count++
	}

	// Require the iterator to return exactly the three matching keys.
	if count != 3 {
		t.Fatalf("Expected 3 keys with prefix, got %d", count)
	}

	// Check whether iteration ended with a storage error.
	if err := iter.Err(); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSQLitePragmas verifies that tunable pragmas supplied via OpenWithPragmas
// are applied to the underlying database connection.
func TestSQLitePragmas(t *testing.T) {
	// Set up the context for checking SQLite pragmas.
	ctx := context.Background()

	// Allocate the database path for the pragma configuration test.
	tp := newTempDBPath(t, "hydra-test-sqlite-pragmas-*.sqlite")

	// Apply a configured cache size when opening the SQLite database.
	const wantCacheSize int32 = -8000
	db, err := OpenWithPragmas(ctx, tp, "test_pragmas", Pragmas{CacheSize: wantCacheSize})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Read the cache-size pragma on two connections held at once, so each is
	// its own connection of the pool.
	for range 2 {
		conn, err := db.GetDB().Conn(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer conn.Close()
		var got int32
		if err := conn.QueryRowContext(ctx, "PRAGMA cache_size").Scan(&got); err != nil {
			t.Fatal(err.Error())
		}
		if got != wantCacheSize {
			t.Fatalf("expected cache_size=%d, got %d", wantCacheSize, got)
		}
	}
}

// TestSQLiteWriterContract walks the commit contract the kvtx Volume relies
// on: ordered commits wait for a flush, full commits are durable, the deadline
// flushes a pending commit, writers queue in arrival order, a read transaction
// keeps one snapshot, and the background checkpoint moves the WAL into the
// database file.
func TestSQLiteWriterContract(t *testing.T) {
	// Open a store and its contract views.
	ctx := t.Context()
	tp := newTempDBPath(t, "hydra-test-sqlite-contract-*.sqlite")
	db, err := Open(ctx, tp, "test_contract")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ordered := db.(kvtx.OrderedCommitStore)
	queue := db.(kvtx.ForegroundQueueStore)
	if !db.(kvtx.AtomicCommitStore).SupportsAtomicCommit() {
		t.Fatal("local store does not support atomic commit")
	}

	// Probe durability with a done context, which WaitDurable answers at once
	// when nothing is pending.
	done, cancel := context.WithCancel(ctx)
	cancel()
	durable := func() bool {
		return ordered.WaitDurable(done) == nil
	}

	// Write one key, committing it as the caller chooses.
	write := func(key, value string, commit func(kvtx.Tx) error) {
		// Begin the write and set the key.
		t.Helper()
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
			t.Fatal(err)
		}

		// Commit it.
		if err := commit(tx); err != nil {
			t.Fatal(err)
		}
	}
	commitOrdered := func(tx kvtx.Tx) error { return kvtx.CommitOrdered(ctx, tx) }
	commitFull := func(tx kvtx.Tx) error { return tx.Commit(ctx) }

	// An ordered commit stays pending until Sync; a full commit is durable.
	write("k", "1", commitOrdered)
	if durable() {
		t.Fatal("ordered commit durable before a flush")
	}
	if err := ordered.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if !durable() {
		t.Fatal("ordered commit pending after Sync")
	}
	write("k", "2", commitOrdered)
	write("k", "3", commitFull)
	if !durable() {
		t.Fatal("ordered commit before a full commit still pending")
	}

	// The deadline flushes a pending ordered commit.
	write("k", "4", commitOrdered)
	start := time.Now()
	if err := ordered.WaitDurable(ctx); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > 2*common.SyncDeadline {
		t.Fatalf("deadline flush took %v", waited)
	}

	// A background writer queues behind the first without counting as a
	// foreground one, and a foreground writer queued behind both does.
	first, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	queued := func(ctx context.Context, value string) chan error {
		done := make(chan error, 1)
		go func() {
			tx, err := db.NewTransaction(ctx, true)
			if err == nil {
				err = tx.Set(ctx, []byte("k"), []byte(value))
			}
			if err == nil {
				err = tx.Commit(ctx)
			}
			done <- err
		}()
		return done
	}
	background := queued(kvtx.WithBackgroundWriteContext(ctx), "6")
	for !queue.WritersWaiting() {
		time.Sleep(time.Millisecond)
	}
	if queue.ForegroundWaiting() {
		t.Fatal("background writer counted as a foreground one")
	}
	foreground := queued(ctx, "7")
	for !queue.ForegroundWaiting() {
		time.Sleep(time.Millisecond)
	}

	// The first writer commits, and the queued ones follow it.
	if err := first.Set(ctx, []byte("k"), []byte("5")); err != nil {
		t.Fatal(err)
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, done := range []chan error{foreground, background} {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if queue.WritersWaiting() || queue.ForegroundWaiting() {
		t.Fatal("writer still waiting after all committed")
	}

	// A read transaction keeps its snapshot across a later commit, and ends
	// with Discard.
	read, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}

	// Read the key through a transaction.
	get := func(tx kvtx.Tx) string {
		t.Helper()
		v, _, err := tx.Get(ctx, []byte("k"))
		if err != nil {
			t.Fatal(err)
		}
		return string(v)
	}
	if got := get(read); got != "6" {
		t.Fatalf("read %q, want 6", got)
	}

	// A commit after the first read leaves the snapshot unchanged.
	write("k", "7", commitFull)
	if got := get(read); got != "6" {
		t.Fatalf("snapshot read %q after a commit, want 6", got)
	}

	// Discard ends the read, and a new read sees the commit.
	read.Discard()
	if _, _, err := read.Get(ctx, []byte("k")); err != kvtx.ErrDiscarded {
		t.Fatalf("read after Discard: %v, want ErrDiscarded", err)
	}
	fresh, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := get(fresh); got != "7" {
		t.Fatalf("new read %q, want 7", got)
	}
	fresh.Discard()

	// The background checkpoint copies the WAL into the database file.
	before, err := os.Stat(tp)
	if err != nil {
		t.Fatal(err)
	}
	write("big", strings.Repeat("x", 1<<20), commitFull)
	deadline := time.Now().Add(3 * common.CheckpointInterval)
	for {
		after, err := os.Stat(tp)
		if err != nil {
			t.Fatal(err)
		}
		if after.Size() >= before.Size()+(1<<19) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("database file %d bytes after checkpoint interval, was %d", after.Size(), before.Size())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSQLiteWALBound writes far past a small write-ahead log bound while a
// reader keeps reading, and checks that the checkpointer brings the log back
// near the bound instead of letting it grow with the writes.
func TestSQLiteWALBound(t *testing.T) {
	// Open a database whose log is bounded at 64 KiB.
	ctx := context.Background()
	tp := newTempDBPath(t, "hydra-test-sqlite-wal-*.sqlite")
	const bound = 64 << 10
	db, err := OpenWithPragmas(ctx, tp, "test_wal", Pragmas{WALCheckpointBytes: bound})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Read constantly while the writes run, as lookups do.
	stop := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if tx, err := db.NewTransaction(ctx, false); err == nil {
				_, _, _ = tx.Get(ctx, []byte("key-0"))
				tx.Discard()
			}
		}
	}()

	// Write 2,000 rows of 1 KiB, each in its own transaction.
	value := make([]byte, 1024)
	for i := range 2000 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := tx.Set(ctx, []byte("key-"+strconv.Itoa(i)), value); err != nil {
			t.Fatal(err.Error())
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}
	}
	close(stop)
	<-readDone

	// The log comes back near its bound once a commit lets the checkpointer
	// run with no reader left.
	deadline := time.Now().Add(5 * time.Second)
	for {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := tx.Set(ctx, []byte("tick"), value); err != nil {
			t.Fatal(err.Error())
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}
		info, err := os.Stat(tp + "-wal")
		if err != nil {
			t.Fatal(err.Error())
		}
		if info.Size() <= 4*bound {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("write-ahead log is %d bytes, want at most %d", info.Size(), 4*bound)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
