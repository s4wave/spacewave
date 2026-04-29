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
	t.Helper()

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
	ctx := context.Background()

	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	tp := newTempDBPath(t, "hydra-test-sqlite-*.sqlite")

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

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Now test with kvtx wrapper
	ktx := store_kvtx.NewKVTx(kvkey, db, nil).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSQLiteWithMode tests SQLite with file mode specification.
func TestSQLiteWithMode(t *testing.T) {
	ctx := context.Background()

	tp := newTempDBPath(t, "hydra-test-sqlite-mode-*.sqlite")

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

	if err := tx.Set(ctx, []byte("mode_test"), []byte("works")); err != nil {
		t.Fatal(err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSQLiteIterator tests specific iterator functionality.
func TestSQLiteIterator(t *testing.T) {
	ctx := context.Background()

	tp := newTempDBPath(t, "hydra-test-sqlite-iter-*.sqlite")

	db, err := Open(ctx, tp, "test_iter")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	// Create transaction and add test data
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	// Add test data with known prefix
	testData := map[string]string{
		"prefix:key1": "value1",
		"prefix:key2": "value2",
		"prefix:key3": "value3",
		"other:key1":  "other1",
	}

	for k, v := range testData {
		if err := tx.Set(ctx, []byte(k), []byte(v)); err != nil {
			t.Fatal(err.Error())
		}
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Test prefix iteration
	tx, err = db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	iter := tx.Iterate(ctx, []byte("prefix:"), true, false)
	defer iter.Close()

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

	if count != 3 {
		t.Fatalf("Expected 3 keys with prefix, got %d", count)
	}

	if err := iter.Err(); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSQLitePragmas verifies that tunable pragmas supplied via OpenWithPragmas
// are applied to the underlying database connection.
func TestSQLitePragmas(t *testing.T) {
	ctx := context.Background()

	tp := newTempDBPath(t, "hydra-test-sqlite-pragmas-*.sqlite")

	const wantCacheSize int32 = -8000
	db, err := OpenWithPragmas(ctx, tp, "test_pragmas", Pragmas{CacheSize: wantCacheSize})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	var got int32
	if err := db.GetDB().QueryRowContext(ctx, "PRAGMA cache_size").Scan(&got); err != nil {
		t.Fatal(err.Error())
	}
	if got != wantCacheSize {
		t.Fatalf("expected cache_size=%d, got %d", wantCacheSize, got)
	}
}

// TestSQLiteReadHandle ensures read-only sqlite transactions are lightweight
// handles over sql.DB and still honor Commit/Discard lifecycle semantics.
func TestSQLiteReadHandle(t *testing.T) {
	ctx := context.Background()

	tp := newTempDBPath(t, "hydra-test-sqlite-read-handle-*.sqlite")

	db, err := Open(ctx, tp, "test_read_handle")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	writeTx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer writeTx.Discard()

	if err := writeTx.Set(ctx, []byte("k"), []byte("v")); err != nil {
		t.Fatal(err.Error())
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	readTx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	val, found, err := readTx.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || string(val) != "v" {
		t.Fatalf("unexpected read result: found=%v val=%q", found, string(val))
	}

	if err := readTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	if _, _, err := readTx.Get(ctx, []byte("k")); err != kvtx.ErrDiscarded {
		t.Fatalf("expected ErrDiscarded after read commit, got %v", err)
	}
}
