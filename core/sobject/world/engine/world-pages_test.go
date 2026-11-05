package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// pinCountStore is a test block store with root ownership that counts the
// reader pins it holds. Like a volume, it pins only a block it holds.
type pinCountStore struct {
	*testBlockStore
	// pins counts the held pins by ref.
	pins map[string]int
	// complete holds the roots marked complete.
	complete map[string]bool
}

func (s *pinCountStore) SupportsRootRetention() bool { return true }

// KeepsHoldCounts reports that the test store does not count shared holds.
func (s *pinCountStore) KeepsHoldCounts() bool { return false }

func (s *pinCountStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	if found, err := s.GetBlockExists(ctx, ref); err != nil || !found {
		return nil, block.ErrNotFound
	}
	key := ref.MarshalString()
	s.pins[key]++
	return func() { s.pins[key]-- }, nil
}

func (s *pinCountStore) OpenStage(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *pinCountStore) ReleaseRoots(context.Context, []*block.BlockRef) error { return nil }

func (s *pinCountStore) MarkRootsComplete(_ context.Context, refs []*block.BlockRef) error {
	for _, ref := range refs {
		s.complete[ref.MarshalString()] = true
	}
	return nil
}

func (s *pinCountStore) RootsComplete(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		out[i] = s.complete[ref.MarshalString()]
	}
	return out, nil
}

// testPage returns a World page holding data with the child refs.
func testPage(t *testing.T, data string, refs ...*block.BlockRef) *block.PutBatchEntry {
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.PutBatchEntry{Ref: ref, Data: []byte(data), Refs: refs, RefsKnown: true}
}

// TestWorldPagesKeepsUntilStored keeps a World's pages unwritten and tries to
// make the view lose one: an orphan sweep, a reader pin across a failed and a
// successful store, and completion proofs on kept roots.
func TestWorldPagesKeepsUntilStored(t *testing.T) {
	// Keep a three-page World, an orphan, and two entries that are not pages.
	ctx := t.Context()
	inner := &pinCountStore{
		testBlockStore: newTestBlockStore("pages", block_mock.NewMockStore(0)),
		pins:           make(map[string]int),
		complete:       make(map[string]bool),
	}
	pages := newWorldPages(inner)

	// Build the pages: a World of three, an orphan, and two non-pages.
	leaf := testPage(t, "leaf")
	mid := testPage(t, "mid", leaf.Ref)
	root := testPage(t, "root", mid.Ref)
	orphan := testPage(t, "orphan")
	refsUnknown := testPage(t, "refs-unknown")
	refsUnknown.RefsKnown = false
	tomb := &block.PutBatchEntry{Ref: testPage(t, "gone").Ref, Tombstone: true}

	// Keep them all; the two non-pages come back.
	rest := pages.keep([]*block.PutBatchEntry{root, mid, leaf, orphan, refsUnknown, tomb})
	if len(rest) != 2 || rest[0] != refsUnknown || rest[1] != tomb {
		t.Fatalf("keep returned %v, want the entry without refs and the tombstone", rest)
	}

	// The store lacks the kept pages.
	if found, err := inner.GetBlockExists(ctx, root.Ref); err != nil || found {
		t.Fatalf("store holds the kept root: found=%v err=%v", found, err)
	}

	// The view reads them with their child lists.
	stored, err := pages.GetStoredBlock(ctx, root.Ref)
	if err != nil || !stored.GetRefsKnown() || len(stored.Refs) != 1 || !stored.Refs[0].EqualVT(mid.Ref) {
		t.Fatalf("stored root: %v err=%v", stored, err)
	}
	exists, err := pages.GetBlockExistsBatch(ctx, []*block.BlockRef{leaf.Ref, refsUnknown.Ref, mid.Ref})
	if err != nil || !exists[0] || exists[1] || !exists[2] {
		t.Fatalf("exists batch: %v err=%v", exists, err)
	}

	// A read scope and a stage see them too.
	readOps, releaseRead, err := pages.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if data, found, err := readOps.GetBlock(ctx, leaf.Ref); err != nil || !found || string(data) != "leaf" {
		t.Fatalf("read scope leaf: %q found=%v err=%v", data, found, err)
	}
	releaseRead()
	stage, releaseStage, err := pages.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := stage.GetBlockExists(ctx, mid.Ref); err != nil || !found {
		t.Fatalf("stage misses the kept mid page: found=%v err=%v", found, err)
	}
	releaseStage()

	// A kept root gets no completion proof.
	if err := pages.MarkRootsComplete(ctx, []*block.BlockRef{root.Ref}); err != nil {
		t.Fatal(err)
	}
	if complete, err := pages.RootsComplete(ctx, []*block.BlockRef{root.Ref}); err != nil || complete[0] {
		t.Fatalf("kept root proved complete: %v err=%v", complete, err)
	}

	// The sweep drops the orphan and keeps what the live root reaches.
	pages.sweep([]*block.BlockRef{root.Ref})
	if found, _ := pages.GetBlockExists(ctx, orphan.Ref); found {
		t.Fatal("sweep kept the orphan")
	}
	if want := len("root") + len("mid") + len("leaf"); pages.keptBytes() != want {
		t.Fatalf("kept bytes %d want %d", pages.keptBytes(), want)
	}

	// A reader pin on a kept page waits here and holds it through a sweep
	// with no live root. One released early never reaches the store.
	releaseMid, err := block.PinRoot(ctx, pages, mid.Ref)
	if err != nil {
		t.Fatal(err)
	}
	releaseEarly, err := block.PinRoot(ctx, pages, mid.Ref)
	if err != nil {
		t.Fatal(err)
	}
	releaseEarly()
	pages.sweep(nil)
	if found, _ := pages.GetBlockExists(ctx, root.Ref); found {
		t.Fatal("sweep kept the unpinned root")
	}
	if found, _ := pages.GetBlockExists(ctx, leaf.Ref); !found {
		t.Fatal("sweep dropped a page under a pin")
	}

	// A store the volume did not take keeps the pages and the pin waiting.
	if err := pages.stored(ctx, []*block.BlockRef{mid.Ref}); err == nil {
		t.Fatal("stored succeeded though the store lacks the pinned page")
	}
	if found, _ := pages.GetBlockExists(ctx, mid.Ref); !found {
		t.Fatal("failed store dropped the pinned page")
	}
	pages.sweep(nil)
	if found, _ := pages.GetBlockExists(ctx, leaf.Ref); !found {
		t.Fatal("failed store left the pin uncovered")
	}

	// Once the store holds them, the pages leave and the pin moves there.
	if _, err := inner.PutBlockBatch(ctx, []*block.PutBatchEntry{mid, leaf}); err != nil {
		t.Fatal(err)
	}
	if err := pages.stored(ctx, []*block.BlockRef{mid.Ref}); err != nil {
		t.Fatal(err)
	}
	if pages.keptBytes() != 0 {
		t.Fatalf("kept bytes after store: %d", pages.keptBytes())
	}
	if got := inner.pins[mid.Ref.MarshalString()]; got != 1 {
		t.Fatalf("store pins on mid: %d want 1", got)
	}
	if data, found, err := pages.GetBlock(ctx, leaf.Ref); err != nil || !found || string(data) != "leaf" {
		t.Fatalf("stored leaf: %q found=%v err=%v", data, found, err)
	}
	releaseMid()
	releaseMid()
	if got := inner.pins[mid.Ref.MarshalString()]; got != 0 {
		t.Fatalf("store pins after release: %d want 0", got)
	}

	// A page a later World writes again stays readable after the first World
	// that reached it leaves memory, whether that World was dropped unstored
	// or stored.
	again := testPage(t, "again")
	first := testPage(t, "first", again.Ref)
	pages.keep([]*block.PutBatchEntry{first, again})
	pages.sweep(nil)

	// A second World writes it again with the stored leaf.
	second := testPage(t, "second", again.Ref, leaf.Ref)
	pages.keep([]*block.PutBatchEntry{second, again, leaf})
	pages.sweep([]*block.BlockRef{second.Ref})
	for _, page := range []*block.PutBatchEntry{again, leaf} {
		if data, found, err := pages.GetBlock(ctx, page.Ref); err != nil || !found || string(data) != string(page.Data) {
			t.Fatalf("rewritten page %q: %q found=%v err=%v", page.Data, data, found, err)
		}
	}

	// The stored leaf stays readable once the rewrite leaves memory.
	pages.sweep(nil)
	if data, found, err := pages.GetBlock(ctx, leaf.Ref); err != nil || !found || string(data) != "leaf" {
		t.Fatalf("stored leaf after its rewrite left: %q found=%v err=%v", data, found, err)
	}
}

// _ checks that the test store implements the complete retention contract.
var _ block.RootRetainer = (*pinCountStore)(nil)
