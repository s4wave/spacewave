package psecho

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

func TestFilterLocalExistingBlocksUsesBatchWithoutPayloadReads(t *testing.T) {
	refs := []*block.BlockRef{
		mustTestBlockRef(t, "first"),
		mustTestBlockRef(t, "second"),
		mustTestBlockRef(t, "third"),
	}
	lk := &queueTestLookup{found: []bool{true, false, true}}

	queued, err := filterLocalExistingBlocks(context.Background(), lk, refs)
	if err != nil {
		t.Fatal(err)
	}
	if lk.existsBatchCalls != 1 {
		t.Fatalf("exists batch calls = %d, want 1", lk.existsBatchCalls)
	}
	if lk.lookupBlockCalls != 0 {
		t.Fatalf("payload lookup calls = %d, want 0", lk.lookupBlockCalls)
	}
	if !lk.localOnly {
		t.Fatal("existence batch did not use local-only lookup")
	}
	if len(queued) != 2 || queued[0] != refs[0] || queued[1] != refs[2] {
		t.Fatalf("queued refs = %#v, want first and third refs", queued)
	}
}

func TestFilterLocalExistingBlocksFallsBackOnBatchError(t *testing.T) {
	refs := []*block.BlockRef{
		mustTestBlockRef(t, "first"),
		mustTestBlockRef(t, "second"),
	}
	lk := &queueTestLookup{
		batchErr:    errors.New("batch failed"),
		lookupFound: []bool{false, true},
	}

	queued, err := filterLocalExistingBlocks(context.Background(), lk, refs)
	if err != nil {
		t.Fatal(err)
	}
	if lk.existsBatchCalls != 1 {
		t.Fatalf("exists batch calls = %d, want 1", lk.existsBatchCalls)
	}
	if lk.lookupBlockCalls != 2 {
		t.Fatalf("payload lookup calls = %d, want 2 fallback calls", lk.lookupBlockCalls)
	}
	if len(queued) != 1 || queued[0] != refs[1] {
		t.Fatalf("queued refs = %#v, want second ref", queued)
	}
}

func mustTestBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

type queueTestLookup struct {
	found            []bool
	lookupFound      []bool
	batchErr         error
	localOnly        bool
	lookupBlockCalls int
	existsBatchCalls int
}

func (l *queueTestLookup) LookupBlock(
	context.Context,
	*block.BlockRef,
	...bucket_lookup.LookupBlockOption,
) ([]byte, bool, error) {
	call := l.lookupBlockCalls
	l.lookupBlockCalls++
	if call < len(l.lookupFound) {
		return nil, l.lookupFound[call], nil
	}
	return nil, false, nil
}

func (l *queueTestLookup) LookupBlockExistsBatch(
	_ context.Context,
	refs []*block.BlockRef,
	opts ...bucket_lookup.LookupBlockOption,
) ([]bool, error) {
	l.existsBatchCalls++
	l.localOnly = bucket_lookup.NewLookupBlockOpts(opts...).LocalOnly
	if l.batchErr != nil {
		return nil, l.batchErr
	}
	out := make([]bool, len(refs))
	copy(out, l.found)
	return out, nil
}

func (l *queueTestLookup) PutBlock(
	context.Context,
	[]byte,
	*block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	return nil, false, nil
}
