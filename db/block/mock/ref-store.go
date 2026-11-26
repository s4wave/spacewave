package block_mock

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// RefStore keeps the references each put declares and serves them with the
// bytes, so graph walks such as DiffGraph can read it. A block put without
// references reads as byte-only.
type RefStore struct {
	block.StoreOps

	mtx  sync.Mutex
	refs map[string][]*block.BlockRef
}

// NewRefStore wraps inner, or a new in-memory mock store when inner is nil.
func NewRefStore(inner block.StoreOps) *RefStore {
	if inner == nil {
		inner = NewMockStore(hash.HashType_HashType_UNKNOWN)
	}
	return &RefStore{StoreOps: inner, refs: make(map[string][]*block.BlockRef)}
}

// GetRefs returns the references the put of ref declared, and whether it
// declared any.
func (s *RefStore) GetRefs(ref *block.BlockRef) ([]*block.BlockRef, bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	refs, known := s.refs[ref.MarshalString()]
	return refs, known
}

// PutBlock stores the bytes and records the references the put declared.
func (s *RefStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	if opts.GetRefs() != nil || opts.GetRefsKnown() {
		s.mtx.Lock()
		s.refs[ref.MarshalString()] = opts.GetRefs()
		s.mtx.Unlock()
	}
	return ref, existed, nil
}

// PutBlockBatch puts each entry through PutBlock.
func (s *RefStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed := make([]bool, len(entries))
	for i, entry := range entries {
		_, exists, err := s.PutBlock(ctx, entry.Data, &block.PutOpts{ForceBlockRef: entry.Ref, Refs: entry.Refs, RefsKnown: entry.RefsKnown})
		if err != nil {
			return nil, err
		}
		existed[i] = exists
	}
	return existed, nil
}

// GetStoredBlock reads the block with its recorded references.
func (s *RefStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := block.GetBlockWithoutRefs(ctx, s, ref)
	if stored != nil {
		var known bool
		stored.Refs, known = s.GetRefs(ref)
		stored.RefsSource = block.AssertedIf(known)
	}
	return stored, err
}

// _ is a type assertion
var _ block.StoreOps = ((*RefStore)(nil))
