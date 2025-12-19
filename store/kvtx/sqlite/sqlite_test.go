//go:build !js && !wasip1

package store_kvtx_sqlite

import (
	"context"
	"os"
	"path"
	"testing"

	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_test "github.com/aperturerobotics/hydra/store/test"
)

// TestSQLite tests all tests on top of SQLite.
func TestSQLite(t *testing.T) {
	ctx := context.Background()

	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	dir, err := os.MkdirTemp("", "hydra-test-sqlite-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)

	tp := path.Join(dir, "database.sqlite")

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

	dir, err := os.MkdirTemp("", "hydra-test-sqlite-mode-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)

	tp := path.Join(dir, "database_mode.sqlite")

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

	dir, err := os.MkdirTemp("", "hydra-test-sqlite-iter-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)

	tp := path.Join(dir, "database_iter.sqlite")

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
