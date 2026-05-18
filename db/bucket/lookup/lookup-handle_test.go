package bucket_lookup

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestLookupBucketGetBlockExistsBatchUsesLookupBatch(t *testing.T) {
	refs := []*block.BlockRef{
		mustLookupTestBlockRef(t, "first"),
		mustLookupTestBlockRef(t, "second"),
	}
	lk := &batchLookupTestLookup{found: []bool{true, false}}
	bkt := NewBucketFromHandle(&batchLookupTestHandle{lookup: lk})

	found, err := bkt.GetBlockExistsBatch(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || !found[0] || found[1] {
		t.Fatalf("found = %v, want [true false]", found)
	}
	if lk.existsBatchCalls != 1 {
		t.Fatalf("exists batch calls = %d, want 1", lk.existsBatchCalls)
	}
	if lk.lookupBlockCalls != 0 {
		t.Fatalf("payload lookup calls = %d, want 0", lk.lookupBlockCalls)
	}
	if !lk.localOnly {
		t.Fatal("exists batch did not use local-only lookup")
	}
}

func mustLookupTestBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

type batchLookupTestHandle struct {
	lookup Lookup
}

func (h *batchLookupTestHandle) GetDisposed() bool {
	return false
}

func (h *batchLookupTestHandle) GetBucketConfig() *bucket.Config {
	return nil
}

func (h *batchLookupTestHandle) GetLookup(context.Context) (Lookup, error) {
	return h.lookup, nil
}

type batchLookupTestLookup struct {
	found            []bool
	localOnly        bool
	lookupBlockCalls int
	existsBatchCalls int
}

func (l *batchLookupTestLookup) LookupBlock(
	context.Context,
	*block.BlockRef,
	...LookupBlockOption,
) ([]byte, bool, error) {
	l.lookupBlockCalls++
	return nil, false, nil
}

func (l *batchLookupTestLookup) LookupBlockExistsBatch(
	_ context.Context,
	refs []*block.BlockRef,
	opts ...LookupBlockOption,
) ([]bool, error) {
	l.existsBatchCalls++
	l.localOnly = NewLookupBlockOpts(opts...).LocalOnly
	out := make([]bool, len(refs))
	copy(out, l.found)
	return out, nil
}

func (l *batchLookupTestLookup) PutBlock(
	context.Context,
	[]byte,
	*block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	return nil, false, nil
}
