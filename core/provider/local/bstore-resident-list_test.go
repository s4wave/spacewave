package provider_local

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// TestWarmResidentSkipsLocalBlocks reads four listed blocks through
// skipLocal, two of which the local volume holds: the bucket is asked for the
// other two alone, since a placed store's reads serve the first two from the
// volume and would never reach copies the warm-up filled.
func TestWarmResidentSkipsLocalBlocks(t *testing.T) {
	ctx := t.Context()
	newStore := func() block.StoreOps {
		return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	}
	bucket, volume := newStore(), newStore()

	// Put four blocks in the bucket and the first two in the volume too.
	var refs []*block.BlockRef
	for i := range 4 {
		data := []byte{'b', byte(i)}
		ref, _, err := bucket.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if _, _, err := volume.PutBlock(ctx, data, nil); err != nil {
				t.Fatal(err)
			}
		}
		refs = append(refs, ref)
	}

	// Read the list through skipLocal, recording what the bucket is asked for.
	var asked []*block.BlockRef
	read := func(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error {
		for _, ref := range refs {
			asked = append(asked, ref)
			data, _, err := bucket.GetBlock(ctx, ref)
			if err != nil {
				return err
			}
			if err := fn(ref, &block.StoredBlock{Data: data}); err != nil {
				return err
			}
		}
		return nil
	}
	var skipped, fetched int
	err := skipLocal(volume, read, &skipped)(ctx, refs, func(*block.BlockRef, *block.StoredBlock) error {
		fetched++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Only the blocks the volume lacks came from the bucket.
	if skipped != 2 || fetched != 2 || len(asked) != 2 {
		t.Fatalf("skipped %d, fetched %d, asked %d; want 2, 2, 2", skipped, fetched, len(asked))
	}
	for _, ref := range asked {
		if !ref.EqualVT(refs[2]) && !ref.EqualVT(refs[3]) {
			t.Fatalf("the bucket was asked for %s, which the volume holds", ref.MarshalString())
		}
	}
}

// TestWarmResidentRetriesThrottledReads has the bucket answer 503 SlowDown
// after the first of four blocks, as a throttled account does: the warm-up
// waits, asks again for the three it lacks alone, and passes on each block
// once. A failure that is not transient still ends the read.
func TestWarmResidentRetriesThrottledReads(t *testing.T) {
	// Build the four refs the warm-up will ask the bucket for.
	ctx := t.Context()
	refs := make([]*block.BlockRef, 4)
	for i := range refs {
		ref, err := block.BuildBlockRef([]byte{'w', byte(i)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	slowDown := &block_store_s3.StatusError{StatusCode: http.StatusServiceUnavailable, Code: "SlowDown"}

	// The bucket fails its first read after one block.
	var asked [][]*block.BlockRef
	read := func(_ context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error {
		asked = append(asked, refs)
		for i, ref := range refs {
			if len(asked) == 1 && i == 1 {
				return errors.Wrap(slowDown, "read pack range")
			}
			if err := fn(ref, &block.StoredBlock{}); err != nil {
				return err
			}
		}
		return nil
	}
	var retries int
	passed := make(map[string]int)
	err := retryTransient(logrus.NewEntry(logrus.New()), read, time.Millisecond, &retries)(ctx, refs, func(ref *block.BlockRef, _ *block.StoredBlock) error {
		passed[ref.MarshalString()]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if retries != 1 || len(asked) != 2 || len(asked[1]) != 3 {
		t.Fatalf("retries %d, reads %d, second read asked %d refs", retries, len(asked), len(asked[len(asked)-1]))
	}
	for _, ref := range refs {
		if passed[ref.MarshalString()] != 1 {
			t.Fatalf("block %s passed on %d times", ref.MarshalString(), passed[ref.MarshalString()])
		}
	}

	// A refused read is not retried.
	denied := &block_store_s3.StatusError{StatusCode: http.StatusForbidden, Code: "AccessDenied"}
	err = retryTransient(logrus.NewEntry(logrus.New()), func(context.Context, []*block.BlockRef, func(*block.BlockRef, *block.StoredBlock) error) error {
		return denied
	}, time.Millisecond, &retries)(ctx, refs, func(*block.BlockRef, *block.StoredBlock) error { return nil })
	if !errors.Is(err, denied) || retries != 1 {
		t.Fatalf("a refused read returned %v after %d retries", err, retries)
	}
}
