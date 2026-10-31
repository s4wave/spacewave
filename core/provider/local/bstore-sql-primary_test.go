package provider_local

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// readCountStore counts the block reads that reach its store.
type readCountStore struct {
	block.StoreOps
	reads atomic.Int32
}

func (s *readCountStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.reads.Add(1)
	return s.StoreOps.GetBlock(ctx, ref)
}

func (s *readCountStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.reads.Add(1)
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// TestSQLPrimaryStores checks that a store with a primary SQL backend writes
// to the backend and not the local bucket, reads a bucket-only block from the
// bucket without reading the backend, reads a backend-only block through, and
// fills neither store from the other.
func TestSQLPrimaryStores(t *testing.T) {
	// Build the chain over an in-memory bucket and backend.
	ctx := context.Background()
	local := newBatchForwardTestStore()
	sql := &readCountStore{StoreOps: newBatchForwardTestStore()}
	write, reads := sqlPrimaryStores("primary", local, sql, func() block.StoreOps { return nil })
	has := func(store block.StoreOps, ref *block.BlockRef) bool {
		t.Helper()
		found, err := store.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}

	// A batch write lands in the backend and not the bucket.
	data := []byte("written")
	ref, err := block.BuildBlockRef(data, &block.PutOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}}); err != nil {
		t.Fatal(err)
	}
	if !has(sql, ref) || has(local, ref) {
		t.Fatal("the write did not land in the backend alone")
	}

	// The bucket is read only through the chain.
	if _, _, err := block.NewReadOnlyStore(local).PutBlock(ctx, []byte("refused"), nil); err == nil {
		t.Fatal("the read-only bucket accepted a write")
	}

	// A bucket-only block reads from the bucket without a backend read.
	old := []byte("bucket only")
	oldRef, _, err := local.PutBlock(ctx, old, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := sql.reads.Load()
	got, found, err := reads.GetBlock(ctx, oldRef)
	if err != nil || !found || string(got) != string(old) {
		t.Fatalf("bucket-only read = %q, %v, %v", got, found, err)
	}
	if sql.reads.Load() != before {
		t.Fatal("a bucket hit read the backend")
	}

	// A backend-only block reads through and is not filled into the bucket.
	got, found, err = reads.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("backend-only read = %q, %v, %v", got, found, err)
	}
	if has(local, ref) || has(sql, oldRef) {
		t.Fatal("a read filled a store")
	}
}
