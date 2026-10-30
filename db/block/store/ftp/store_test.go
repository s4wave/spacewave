//go:build !tinygo

package block_store_ftp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
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

// testConfig returns the server BLOCK_STORE_FTP_TEST_HOST names, such as a
// ProFTPD container, and skips the test without one. BLOCK_STORE_FTP_TEST_CA
// names the PEM file of the CA that signed its certificate.
func testConfig(t *testing.T) *Config {
	// Read the server from the environment, skipping without one.
	t.Helper()
	host := os.Getenv("BLOCK_STORE_FTP_TEST_HOST")
	if host == "" {
		t.Skip("BLOCK_STORE_FTP_TEST_HOST names no FTPS server")
	}
	port, _ := strconv.ParseUint(os.Getenv("BLOCK_STORE_FTP_TEST_PORT"), 10, 32)
	conf := &Config{
		Host:     host,
		Port:     uint32(port),
		User:     os.Getenv("BLOCK_STORE_FTP_TEST_USER"),
		Password: os.Getenv("BLOCK_STORE_FTP_TEST_PASSWORD"),
		Root:     "test-" + strconv.FormatInt(time.Now().UnixNano(), 36),
	}
	if caFile := os.Getenv("BLOCK_STORE_FTP_TEST_CA"); caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(pem)
		conf.TLSConfig = &tls.Config{RootCAs: roots}
	}
	return conf
}

// openTestStore opens the store of storeID on the test server.
func openTestStore(t *testing.T, conf *Config, storeID string) *Store {
	// Open the store and close it when the test ends.
	t.Helper()
	store, err := NewStore(logrus.NewEntry(logrus.New()), NewClient(conf), storeID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

// TestStore writes a batch, reads each block with its refs, and checks that
// files only add: a repeated write keeps the file, a delete is refused, another
// store's files stay apart, a batch wider than the pool completes, and an
// upload cut off midway leaves no block behind.
func TestStore(t *testing.T) {
	// Open a store on a pool smaller than the wide batch below.
	ctx := context.Background()
	conf := testConfig(t)
	conf.MaxConns = 3
	store := openTestStore(t, conf, "store-a")

	// Write a leaf and a parent naming it, and refuse a block whose refs are
	// unknown.
	leafData, parentData := []byte("leaf"), []byte("parent")
	leaf, _ := block.BuildBlockRef(leafData, nil)
	parent, _ := block.BuildBlockRef(parentData, nil)
	_, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: leaf, Data: leafData, RefsKnown: true},
		{Ref: parent, Data: parentData, Refs: []*block.BlockRef{leaf}, RefsKnown: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PutBlock(ctx, []byte("loose"), nil); !errors.Is(err, block.ErrRefsUnknown) {
		t.Fatalf("loose block returned %v, want ErrRefsUnknown", err)
	}

	// Read each back with its refs.
	stored, err := store.GetStoredBlock(ctx, parent)
	if err != nil || stored == nil {
		t.Fatalf("parent: %v %v", stored, err)
	}
	if !bytes.Equal(stored.Data, parentData) || !stored.RefsKnown || len(stored.Refs) != 1 || !stored.Refs[0].EqualVT(leaf) {
		t.Fatalf("parent read back as %+v", stored)
	}
	stored, err = store.GetStoredBlock(ctx, leaf)
	if err != nil || stored == nil || !stored.RefsKnown || len(stored.Refs) != 0 {
		t.Fatalf("leaf read back as %+v, %v", stored, err)
	}
	stat, err := store.StatBlock(ctx, parent)
	if err != nil || stat == nil || stat.Size != int64(len(parentData)) {
		t.Fatalf("stat: %+v %v", stat, err)
	}

	// A repeated write keeps the file, and a delete is refused.
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: leaf, Data: leafData, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	if err := store.RmBlock(ctx, leaf); !errors.Is(err, ErrAppendOnly) {
		t.Fatalf("rm returned %v, want ErrAppendOnly", err)
	}
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: leaf, Tombstone: true}}); !errors.Is(err, ErrAppendOnly) {
		t.Fatalf("tombstone returned %v, want ErrAppendOnly", err)
	}

	// Existence probes report each position, including repeats and misses,
	// and another store's files are apart.
	missing, _ := block.BuildBlockRef([]byte("missing"), nil)
	exists, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{leaf, missing, parent, leaf})
	if err != nil {
		t.Fatal(err)
	}
	if want := []bool{true, false, true, true}; !slices.Equal(exists, want) {
		t.Fatalf("exists=%v want %v", exists, want)
	}
	if stored, err := store.GetStoredBlock(ctx, missing); err != nil || stored != nil {
		t.Fatalf("missing block read as %v, %v", stored, err)
	}
	other := openTestStore(t, conf, "store-b")
	if found, err := other.GetBlockExists(ctx, leaf); err != nil || found {
		t.Fatalf("other store found the block: %v %v", found, err)
	}

	// A batch wider than the pool stores every block.
	var entries []*block.PutBatchEntry
	var refs []*block.BlockRef
	for i := range 40 {
		data := bytes.Repeat([]byte{byte(i)}, 1000+i)
		ref, _ := block.BuildBlockRef(data, nil)
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		refs = append(refs, ref)
	}
	if _, err := store.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	exists, err = store.GetBlockExistsBatch(ctx, refs)
	if err != nil || slices.Contains(exists, false) {
		t.Fatalf("wide batch exists=%v %v", exists, err)
	}

	// Cut off the upload of a large block midway.
	big := make([]byte, 64<<20)
	_, _ = rand.Read(big)
	bigRef, _ := block.BuildBlockRef(big, nil)
	bigEntry := []*block.PutBatchEntry{{Ref: bigRef, Data: big, RefsKnown: true}}
	cutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = store.PutBlockBatch(cutCtx, bigEntry)
	cancel()
	if err == nil {
		t.Fatal("upload finished before the cut; raise the block size")
	}

	// The cut upload left no block under its name, and the next attempt
	// stores it.
	if found, err := store.GetBlockExists(ctx, bigRef); err != nil || found {
		t.Fatalf("cut upload left the block: %v %v", found, err)
	}
	if _, err := store.PutBlockBatch(ctx, bigEntry); err != nil {
		t.Fatal(err)
	}
	if data, found, err := store.GetBlock(ctx, bigRef); err != nil || !found || !bytes.Equal(data, big) {
		t.Fatalf("big block read back: found=%v %v", found, err)
	}
}

// TestCheck passes against a reachable server and reports refused
// credentials.
func TestCheck(t *testing.T) {
	conf := testConfig(t)
	if result := Check(t.Context(), conf); result.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("check: %v %s", result.GetOutcome(), result.GetDetail())
	}
	conf.Password += "-wrong"
	if result := Check(t.Context(), conf); result.GetOutcome() != block_store_s3.CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED {
		t.Fatalf("wrong password: %v %s", result.GetOutcome(), result.GetDetail())
	}
}
