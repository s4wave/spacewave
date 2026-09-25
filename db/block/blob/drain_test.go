package blob

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// batchCountStore counts the batches written to the wrapped store.
type batchCountStore struct {
	block.StoreOps
	batches int
}

// PutBlockBatch counts the batch and forwards it.
func (s *batchCountStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.batches++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

// TestBuildBlobWriteDrainsOnce checks that a chunked blob's staged chunks and
// the root that references them reach the store in one batch.
func TestBuildBlobWriteDrainsOnce(t *testing.T) {
	ctx := t.Context()
	store := &batchCountStore{StoreOps: block_mock.NewMockStore(0)}
	data := make([]byte, 1<<20)
	if _, err := io.ReadFull(prng.BuildSeededReader([]byte("drain once")), data); err != nil {
		t.Fatal(err)
	}

	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	if _, err := BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), bcs, nil); err != nil {
		t.Fatal(err)
	}
	if staged := btx.GetStagedStore(); staged == nil {
		t.Fatal("chunked blob did not stage its chunks")
	}
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if store.batches != 1 {
		t.Fatalf("store batches: got %d want 1", store.batches)
	}

	// The written blob reads back from the store alone.
	_, rcs := block.NewTransaction(store, nil, ref, nil)
	rdr, err := NewReader(ctx, rcs)
	if err != nil {
		t.Fatal(err)
	}
	defer rdr.Close()
	got, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("blob contents differ after write")
	}
}
