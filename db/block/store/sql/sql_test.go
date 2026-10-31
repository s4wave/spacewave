//go:build !tinygo

package block_store_sql

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// testConfig returns the database BLOCK_STORE_SQL_TEST_HOST names, such as a
// MariaDB container, and skips the test without one.
func testConfig(t *testing.T) *Config {
	// Read the server from the environment, with a fresh table per test.
	t.Helper()
	host := os.Getenv("BLOCK_STORE_SQL_TEST_HOST")
	if host == "" {
		t.Skip("BLOCK_STORE_SQL_TEST_HOST names no MySQL-compatible server")
	}
	port, _ := strconv.ParseUint(os.Getenv("BLOCK_STORE_SQL_TEST_PORT"), 10, 32)
	return &Config{
		Host:     host,
		Port:     uint32(port),
		Database: os.Getenv("BLOCK_STORE_SQL_TEST_DATABASE"),
		User:     os.Getenv("BLOCK_STORE_SQL_TEST_USER"),
		Password: os.Getenv("BLOCK_STORE_SQL_TEST_PASSWORD"),
		Table:    "test_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_blocks",
	}
}

// openTestStore opens the store of storeID in the test database.
func openTestStore(t *testing.T, conf *Config, storeID string) *Store {
	// Open the pool and table, and close the store when the test ends.
	t.Helper()
	db, err := Open(conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureTable(t.Context(), db, conf.Table); err != nil {
		t.Fatal(err)
	}
	store := NewStore(logrus.NewEntry(logrus.New()), db, conf.Table, storeID)
	t.Cleanup(store.Close)
	return store
}

// TestStore writes a batch, reads each block with its refs, and checks that
// rows only add: a repeated write keeps the row, a delete is refused, and
// another store's rows stay apart.
func TestStore(t *testing.T) {
	// Open a store in a fresh table.
	ctx := context.Background()
	conf := testConfig(t)
	store := openTestStore(t, conf, "store-a")

	// Write a leaf, a parent naming it, and a block whose refs are unknown.
	leafData, parentData, looseData := []byte("leaf"), []byte("parent"), []byte("loose")
	leaf, _ := block.BuildBlockRef(leafData, nil)
	parent, _ := block.BuildBlockRef(parentData, nil)
	loose, _ := block.BuildBlockRef(looseData, nil)
	_, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: leaf, Data: leafData, RefsKnown: true},
		{Ref: parent, Data: parentData, Refs: []*block.BlockRef{leaf}, RefsKnown: true},
		{Ref: loose, Data: looseData},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read the parent back with its refs.
	stored, err := store.GetStoredBlock(ctx, parent)
	if err != nil || stored == nil {
		t.Fatalf("parent: %v %v", stored, err)
	}
	if !bytes.Equal(stored.Data, parentData) || !stored.RefsKnown || len(stored.Refs) != 1 || !stored.Refs[0].EqualVT(leaf) {
		t.Fatalf("parent read back as %+v", stored)
	}

	// The leaf has known empty refs, the loose block unknown refs.
	stored, err = store.GetStoredBlock(ctx, leaf)
	if err != nil || stored == nil || !stored.RefsKnown || len(stored.Refs) != 0 {
		t.Fatalf("leaf read back as %+v, %v", stored, err)
	}
	stored, err = store.GetStoredBlock(ctx, loose)
	if err != nil || stored == nil || stored.RefsKnown {
		t.Fatalf("loose block read back as %+v, %v", stored, err)
	}

	// Stat reports the data's size.
	stat, err := store.StatBlock(ctx, parent)
	if err != nil || stat == nil || stat.Size != int64(len(parentData)) {
		t.Fatalf("stat: %+v %v", stat, err)
	}

	// A repeated write keeps the row, and a delete is refused.
	if _, _, err := store.PutBlock(ctx, leafData, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.RmBlock(ctx, leaf); !errors.Is(err, ErrAppendOnly) {
		t.Fatalf("rm returned %v, want ErrAppendOnly", err)
	}
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: leaf, Tombstone: true}}); !errors.Is(err, ErrAppendOnly) {
		t.Fatalf("tombstone returned %v, want ErrAppendOnly", err)
	}

	// Existence probes report each position, including repeats and misses,
	// and another store's rows are apart.
	missing, _ := block.BuildBlockRef([]byte("missing"), nil)
	exists, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{leaf, missing, parent, leaf})
	if err != nil {
		t.Fatal(err)
	}
	if want := []bool{true, false, true, true}; !slices.Equal(exists, want) {
		t.Fatalf("exists=%v want %v", exists, want)
	}
	other := openTestStore(t, conf, "store-b")
	if found, err := other.GetBlockExists(ctx, leaf); err != nil || found {
		t.Fatalf("other store found the block: %v %v", found, err)
	}
	if stored, err := other.GetStoredBlock(ctx, leaf); err != nil || stored != nil {
		t.Fatalf("other store read the block: %v %v", stored, err)
	}
}

// TestStoreSplitsBatches writes a batch past the statement budget, which the
// store splits into several inserts.
func TestStoreSplitsBatches(t *testing.T) {
	// Shrink the statement budget to a few blocks.
	ctx := context.Background()
	store := openTestStore(t, testConfig(t), "store-split")
	store.packetOnce.Do(func() { store.packetBytes = 4096 })

	// Write twenty 1000 byte blocks and find each stored.
	var entries []*block.PutBatchEntry
	var refs []*block.BlockRef
	for i := range 20 {
		data := bytes.Repeat([]byte{byte(i)}, 1000)
		ref, _ := block.BuildBlockRef(data, nil)
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		refs = append(refs, ref)
	}
	if _, err := store.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	exists, err := store.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range exists {
		if !ok {
			t.Fatalf("block %d missing", i)
		}
	}
}

// TestCheck passes against a reachable database and reports refused
// credentials.
func TestCheck(t *testing.T) {
	// Check with the right password, then a wrong one.
	conf := testConfig(t)
	conf.Table = conf.Table + "_check"
	if result := Check(t.Context(), conf); result.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("check: %v %s", result.GetOutcome(), result.GetDetail())
	}
	conf.Password += "-wrong"
	if result := Check(t.Context(), conf); result.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED {
		t.Fatalf("wrong password: %v %s", result.GetOutcome(), result.GetDetail())
	}
}

// TestReadSkipsPrepare checks that a block read sends its arguments inline,
// one round trip without a server-side prepared statement, and that binary
// data and keys survive the inline encoding.
func TestReadSkipsPrepare(t *testing.T) {
	// Write a block holding every byte value.
	ctx := context.Background()
	store := openTestStore(t, testConfig(t), "store-inline")
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	ref, _ := block.BuildBlockRef(data, nil)
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}

	// Count the server's prepared statements across ten reads.
	prepares := func() int64 {
		var name string
		var count int64
		if err := store.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Com_stmt_prepare'").Scan(&name, &count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	before := prepares()
	for range 10 {
		got, found, err := store.GetBlock(ctx, ref)
		if err != nil || !found || !bytes.Equal(got, data) {
			t.Fatalf("read back %d bytes, found=%v err=%v", len(got), found, err)
		}
	}

	// The reads prepared nothing.
	if after := prepares(); after != before {
		t.Fatalf("ten reads prepared %d statements, want 0", after-before)
	}
}
