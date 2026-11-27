package block_store_writeback

import (
	"slices"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// recordingObserver records what it sees, one line a call.
type recordingObserver struct {
	mu   sync.Mutex
	seen []string
}

func (r *recordingObserver) add(kind string, h *hash.Hash) {
	r.mu.Lock()
	r.seen = append(r.seen, kind+" "+h.MarshalString())
	r.mu.Unlock()
}

func (r *recordingObserver) Put(h *hash.Hash)  { r.add("p", h) }
func (r *recordingObserver) Root(h *hash.Hash) { r.add("r", h) }
func (r *recordingObserver) Fill(h *hash.Hash) { r.add("f", h) }

func (r *recordingObserver) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.seen
	r.seen = nil
	return out
}

// TestObserverSeesWrites checks the observer sees every accepted put, a put
// of a held block, each root, and each fill, through the store and its read
// copies, and nothing of a failed write or after it is cleared.
func TestObserverSeesWrites(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	obs := &recordingObserver{}
	s.SetObserver(obs)
	entries, refs := batchOf(t, "a", "b")
	name := func(kind string, ref *block.BlockRef) string { return kind + " " + ref.GetHash().MarshalString() }

	// A batch, then the same batch again: both are seen.
	for range 2 {
		if _, err := s.PutBlockBatch(ctx, entries); err != nil {
			t.Fatal(err)
		}
		if got := obs.take(); !slices.Equal(got, []string{name("p", refs[0]), name("p", refs[1])}) {
			t.Fatalf("batch seen as %v", got)
		}
	}

	// A single put is seen, and a read copy shares the observer.
	ref, _, err := s.PutBlock(ctx, []byte("c"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	if got := obs.take(); !slices.Equal(got, []string{name("p", ref)}) {
		t.Fatalf("put seen as %v", got)
	}
	ops, release, err := s.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ops.(*MarkingStore).observer != s.observer {
		t.Fatal("read copy has its own observer")
	}
	release()

	// Roots: each entry, then each root's block.
	parent := []byte("parent")
	parentRef, _ := block.BuildBlockRef(parent, nil)
	err = s.SetRoots(ctx, []*block.PutBatchEntry{
		{Ref: parentRef, Data: parent, Refs: refs, RefsKnown: true},
	}, []block.NamedRoot{{Name: "main", Ref: parentRef}, {Name: "gone"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := obs.take(); !slices.Equal(got, []string{name("p", parentRef), name("r", parentRef)}) {
		t.Fatalf("roots seen as %v", got)
	}

	// A fill is seen when queued, even when the queue holds it already.
	filled, _ := block.BuildBlockRef([]byte("filled"), nil)
	stored := &block.StoredBlock{Data: []byte("filled"), RefsKnown: true}
	s.CacheFill(filled, stored)
	s.CacheFill(filled, stored)
	if got := obs.take(); !slices.Equal(got, []string{name("f", filled), name("f", filled)}) {
		t.Fatalf("fills seen as %v", got)
	}

	// A failed write is not seen, and a cleared observer sees nothing.
	failing, err := NewStore(ctx, &failingStore{StoreOps: newUntrackedBlockStore()}, env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}
	failing.SetObserver(obs)
	if _, err := failing.PutBlockBatch(ctx, entries); err == nil {
		t.Fatal("failing write succeeded")
	}
	s.SetObserver(nil)
	if _, err := s.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if got := obs.take(); len(got) != 0 {
		t.Fatalf("seen %v", got)
	}
}

// TestNoObserverCostsNothing checks a batch with no observer allocates
// nothing in the observer.
func TestNoObserverCostsNothing(t *testing.T) {
	entries, _ := batchOf(t, "a", "b", "c")
	m := NewMarkingStore(newUntrackedBlockStore(), nil)
	roots := []block.NamedRoot{{Name: "main", Ref: entries[0].Ref}}
	if n := testing.AllocsPerRun(100, func() { m.observeEntries(entries, roots) }); n != 0 {
		t.Fatalf("observer with none set allocates %v", n)
	}
}
