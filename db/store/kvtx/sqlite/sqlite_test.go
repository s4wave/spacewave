//go:build !js

package store_kvtx_sqlite

import (
	"context"
	"os"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
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

	// Read the cache-size pragma from the open database connection.
	var got int32
	if err := db.GetDB().QueryRowContext(ctx, "PRAGMA cache_size").Scan(&got); err != nil {
		t.Fatal(err.Error())
	}

	// Verify SQLite retained the configured cache size.
	if got != wantCacheSize {
		t.Fatalf("expected cache_size=%d, got %d", wantCacheSize, got)
	}
}

// TestSQLiteReadHandle ensures read-only sqlite transactions are lightweight
// handles over sql.DB and still honor Commit/Discard lifecycle semantics.
func TestSQLiteReadHandle(t *testing.T) {
	// Set up the context for the read-handle lifecycle test.
	ctx := context.Background()

	// Allocate a temporary database path for the read-handle test.
	tp := newTempDBPath(t, "hydra-test-sqlite-read-handle-*.sqlite")

	// Open the SQLite store used for read and write transactions.
	db, err := Open(ctx, tp, "test_read_handle")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Begin a write transaction to seed the read-handle fixture.
	writeTx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer writeTx.Discard()

	// Write the fixture key and value before opening a read handle.
	if err := writeTx.Set(ctx, []byte("k"), []byte("v")); err != nil {
		t.Fatal(err.Error())
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open a read-only transaction over the committed fixture.
	readTx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the fixture value through the read-only transaction.
	val, found, err := readTx.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || string(val) != "v" {
		t.Fatalf("unexpected read result: found=%v val=%q", found, string(val))
	}

	// Commit the read handle and exercise its lifecycle boundary.
	if err := readTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify a committed read handle rejects subsequent reads.
	if _, _, err := readTx.Get(ctx, []byte("k")); err != kvtx.ErrDiscarded {
		t.Fatalf("expected ErrDiscarded after read commit, got %v", err)
	}
}
