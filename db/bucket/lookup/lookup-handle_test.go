package bucket_lookup

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// TestLookupBucketExistenceUsesLookupBatch rejects payload reads for existence probes.
func TestLookupBucketExistenceUsesLookupBatch(t *testing.T) {
	refs := []*block.BlockRef{
		mustLookupTestBlockRef(t, "first"),
		mustLookupTestBlockRef(t, "second"),
	}
	lk := &batchLookupTestLookup{found: []bool{true, false}}
	bkt := NewBucketFromHandle(&batchLookupTestHandle{lookup: lk})

	found, err := bkt.GetBlockExistsBatch(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || !found[0] || found[1] {
		t.Fatalf("found = %v, want [true false]", found)
	}
	// Single-block probes use the same metadata-only lookup for hits and misses.
	for _, expected := range []bool{true, false} {
		lk.found = []bool{expected}
		found, err := bkt.GetBlockExists(t.Context(), refs[0])
		if err != nil {
			t.Fatal(err)
		}
		if found != expected {
			t.Fatalf("single block found = %t, want %t", found, expected)
		}
	}
	if lk.existsBatchCalls != 3 {
		t.Fatalf("exists batch calls = %d, want 3", lk.existsBatchCalls)
	}
	if lk.lookupBlockCalls != 0 {
		t.Fatalf("payload lookup calls = %d, want 0", lk.lookupBlockCalls)
	}
	if !lk.localOnly {
		t.Fatal("exists batch did not use local-only lookup")
	}
}

// mustLookupTestBlockRef builds a valid reference for a fixture payload.
func mustLookupTestBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// batchLookupTestHandle exposes one lookup service to the bucket adapter.
type batchLookupTestHandle struct {
	// lookup supplies the fixture lookup operations.
	lookup Lookup
}

// GetDisposed keeps the fixture handle available.
func (h *batchLookupTestHandle) GetDisposed() bool {
	return false
}

// GetBucketConfig omits configuration unused by the existence probes.
func (h *batchLookupTestHandle) GetBucketConfig() *bucket.Config {
	return nil
}

// GetLookup returns the fixture service.
func (h *batchLookupTestHandle) GetLookup(context.Context) (Lookup, error) {
	return h.lookup, nil
}

// batchLookupTestLookup records whether the adapter reads metadata or payloads.
type batchLookupTestLookup struct {
	// found supplies the configured existence results.
	found []bool
	// localOnly records the lookup's discovery policy.
	localOnly bool
	// lookupBlockCalls counts payload reads.
	lookupBlockCalls int
	// existsBatchCalls counts metadata probes.
	existsBatchCalls int
}

// BeginReadOperation retains the fixture lookup for a bounded read.
func (l *batchLookupTestLookup) BeginReadOperation(context.Context) (Lookup, func(), error) {
	return l, func() {}, nil
}

// LookupBlock records an unwanted payload read.
func (l *batchLookupTestLookup) LookupBlock(
	context.Context,
	*block.BlockRef,
	...LookupBlockOption,
) ([]byte, bool, error) {
	l.lookupBlockCalls++
	return nil, false, nil
}

// LookupStoredBlock reads the block without refs because this test lookup
// keeps no ref graph.
func (l *batchLookupTestLookup) LookupStoredBlock(ctx context.Context, ref *block.BlockRef, opts ...LookupBlockOption) (*block.StoredBlock, error) {
	data, found, err := l.LookupBlock(ctx, ref, opts...)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

// LookupBlockExistsBatch returns configured metadata results in input order.
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

// PutBlock satisfies the unused write contract.
func (l *batchLookupTestLookup) PutBlock(
	context.Context,
	[]byte,
	*block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	return nil, false, nil
}
