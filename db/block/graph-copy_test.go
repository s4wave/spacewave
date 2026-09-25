package block_test

import (
	"context"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// graphTestStore records the refs of each put and serves them with the bytes.
// Blocks put without refs read as byte-only.
type graphTestStore struct {
	block.StoreOps

	mu   sync.Mutex
	refs map[string][]*block.BlockRef
	// reads counts GetStoredBlock calls.
	reads int
}

func newGraphTestStore() *graphTestStore {
	return &graphTestStore{
		StoreOps: newOverlayMemoryStore(),
		refs:     make(map[string][]*block.BlockRef),
	}
}

func (s *graphTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if _, _, err := s.PutBlock(ctx, entry.Data, &block.PutOpts{ForceBlockRef: entry.Ref, Refs: entry.Refs}); err != nil {
			return err
		}
	}
	return nil
}

func (s *graphTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	if opts.GetRefs() != nil {
		s.mu.Lock()
		s.refs[ref.MarshalString()] = opts.GetRefs()
		s.mu.Unlock()
	}
	return ref, existed, nil
}

func (s *graphTestStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.mu.Lock()
	s.reads++
	refs, known := s.refs[ref.MarshalString()]
	s.mu.Unlock()
	stored, err := block.GetBlockWithoutRefs(ctx, s, ref)
	if stored != nil {
		stored.Refs, stored.RefsKnown = refs, known
	}
	return stored, err
}

// put writes a block with refs, marking a leaf with an empty list.
func (s *graphTestStore) put(t *testing.T, data string, refs ...*block.BlockRef) *block.BlockRef {
	t.Helper()
	if refs == nil {
		refs = []*block.BlockRef{}
	}
	ref, _, err := s.PutBlock(t.Context(), []byte(data), &block.PutOpts{Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestCopyGraphRebuildsGraphInPostOrder(t *testing.T) {
	ctx := t.Context()
	src := newGraphTestStore()
	shared := src.put(t, "shared")
	left := src.put(t, "left", shared)
	right := src.put(t, "right", shared, shared)
	skipped := src.put(t, "skipped")
	root := src.put(t, "root", left, right, skipped)

	dst := newGraphTestStore()
	var completed []string
	err := block.CopyGraph(ctx, src, dst, root, &block.GraphCopyOptions{
		Known: func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
			known := make([]bool, len(refs))
			for i, ref := range refs {
				known[i] = ref.EqualsRef(skipped)
			}
			return known, nil
		},
		Complete: func(_ context.Context, ref *block.BlockRef) error {
			completed = append(completed, ref.MarshalString())
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, ref := range []*block.BlockRef{root, left, right, shared} {
		want := src.refs[ref.MarshalString()]
		got, ok := dst.refs[ref.MarshalString()]
		if !ok || len(got) != len(want) {
			t.Fatalf("dst refs of %s = %v, want %v", ref.MarshalString(), got, want)
		}
	}
	if _, ok := dst.refs[skipped.MarshalString()]; ok {
		t.Fatal("known subtree was copied")
	}
	if src.reads != 4 {
		t.Fatalf("source reads = %d, want 4", src.reads)
	}
	index := func(ref *block.BlockRef) int {
		for i, key := range completed {
			if key == ref.MarshalString() {
				return i
			}
		}
		t.Fatalf("%s did not complete", ref.MarshalString())
		return -1
	}
	if len(completed) != 4 {
		t.Fatalf("completed %d blocks, want 4", len(completed))
	}
	if index(shared) > index(left) || index(shared) > index(right) ||
		index(left) > index(root) || index(right) > index(root) {
		t.Fatalf("completion order %v is not post-order", completed)
	}
}

func TestCopyGraphReportsMissingAndUnknownBlocks(t *testing.T) {
	ctx := t.Context()
	src := newGraphTestStore()
	opaque, _, err := src.StoreOps.PutBlock(ctx, []byte("opaque"), nil)
	if err != nil {
		t.Fatal(err)
	}
	missing := src.put(t, "missing")
	if err := src.RmBlock(ctx, missing); err != nil {
		t.Fatal(err)
	}

	var completed int
	opts := &block.GraphCopyOptions{
		Complete: func(context.Context, *block.BlockRef) error {
			completed++
			return nil
		},
	}
	if err := block.CopyGraph(ctx, src, newGraphTestStore(), src.put(t, "a", opaque), opts); !errors.Is(err, block.ErrRefsUnknown) {
		t.Fatalf("byte-only child err = %v, want ErrRefsUnknown", err)
	}
	if err := block.CopyGraph(ctx, src, newGraphTestStore(), src.put(t, "b", missing), opts); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("missing child err = %v, want ErrNotFound", err)
	}
	if completed != 0 {
		t.Fatalf("completed %d blocks over a failed subtree", completed)
	}
}
