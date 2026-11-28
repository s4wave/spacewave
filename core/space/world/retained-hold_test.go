package space_world

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// peerServedStore is a bucket whose PinRoot stands in for the local provider's
// read path and peers: a pin of a root the bucket lacks fails with
// block.ErrUnavailable until the test serves the root's block, and then stages
// the block into the bucket and pins it. A pin of a root marked not found fails
// with block.ErrNotFound, as the local provider's read path answers when no
// store holds the block.
type peerServedStore struct {
	*bucketStore
	// mtx guards the fields below.
	mtx sync.Mutex
	// served holds the data a peer serves by ref.
	served map[string][]byte
	// notFound marks the refs whose pin answers block.ErrNotFound.
	notFound map[string]bool
	// tries counts the pins asked for by ref.
	tries map[string]int
	// gate, when set for a ref, holds its pin until closed, after sending
	// on entered.
	gate map[string]chan struct{}
	// entered receives a ref's key when its pin waits at its gate.
	entered chan string
}

// serve lets the peers serve data.
func (s *peerServedStore) serve(data []byte) *block.BlockRef {
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		panic(err)
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.served[ref.MarshalString()] = data
	return ref
}

// triesOf returns the pins asked for of ref.
func (s *peerServedStore) triesOf(ref *block.BlockRef) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.tries[ref.MarshalString()]
}

// PinRoot pins a root the bucket holds, and stages a served one in first.
func (s *peerServedStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	// Count the try and wait at the gate.
	key := ref.MarshalString()
	s.mtx.Lock()
	s.tries[key]++
	gate := s.gate[key]
	s.mtx.Unlock()
	if gate != nil {
		s.entered <- key
		<-gate
	}

	// Answer that no store holds a root marked not found.
	s.mtx.Lock()
	notFound := s.notFound[key]
	s.mtx.Unlock()
	if notFound {
		return nil, fmt.Errorf("%w: no store holds block %s", block.ErrNotFound, key)
	}

	// Pin a held root, and refuse one no peer serves.
	found, err := s.GetBlockExists(ctx, ref)
	if err != nil || found {
		if err != nil {
			return nil, err
		}
		return s.LeasedRoots.PinRoot(ctx, ref)
	}
	s.mtx.Lock()
	data, ok := s.served[key]
	s.mtx.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: no peer served block %s", block.ErrUnavailable, key)
	}

	// Stage the served block, then pin it.
	staged, release, err := s.OpenStage(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if _, err := staged.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
		return nil, err
	}
	return s.LeasedRoots.PinRoot(ctx, ref)
}

// testRetainedRootWaits walks a retained root no source serves through a
// Volume that frees every unheld block at once. The install that retains it
// lands, so replay continues, and the set keeps the root while it waits as
// missing, which RootWaits shows. Installs neither try it again nor drop it,
// and a held root never waits. Once a peer serves it, the retrier holds it
// even while an install lands meanwhile, and it survives collection after
// the retrier's pin ends. An adapter opened again checks each root off the
// install path, and finds the one still missing. A root no store holds waits
// as missing the same way, on the install path and at each retry, until a peer
// serves it. With RetainedWaitEnv "off"
// the install stops as before and holds nothing.
func testRetainedRootWaits(t *testing.T) {
	// Build an adapter over a Volume, with the bucket's pins standing in
	// for peers.
	ctx := t.Context()
	a, collect := newVolumeAdapter(t, 1<<20, time.Nanosecond)
	bs := a.so.GetBlockStore().(*bucketStore)
	store := &peerServedStore{
		bucketStore: bs,
		served:      make(map[string][]byte),
		notFound:    make(map[string]bool),
		tries:       make(map[string]int),
		gate:        make(map[string]chan struct{}),
		entered:     make(chan string, 1),
	}
	a.pages = block.NewPendingStore(store, 1<<20)
	a.retained = newRetainedHold()
	kept := func(name string) *block.BlockRef {
		t.Helper()
		data := []byte("retained World " + name)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	absent := func(name string) *block.BlockRef {
		t.Helper()
		ref, err := block.BuildBlockRef([]byte("retained World "+name), nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// install installs a head retaining roots.
	install := func(b *Adapter, roots ...*RetainedRoot) error {
		b.replay = &spaceReplay{a: b, stage: newReplayStage(b.pages), base: &InnerState{
			HeadRef:       &bucket.ObjectRef{RootRef: absent("head")},
			RetainedRoots: roots,
		}}
		return b.installHead(ctx)
	}

	// expect checks the set the Volume's root holds, the waits and the
	// missing roots RootWaits shows, that a held root never waits, and that
	// the Volume still holds each held root after a collection.
	expect := func(b *Adapter, step string, set []*RetainedRoot, missing []string, unchecked []string, held ...*block.BlockRef) {
		t.Helper()
		read := &Adapter{pages: b.pages, held: map[string]*block.BlockRef{RootRetained: b.held[RootRetained]}}
		if err := read.readRetained(ctx); err != nil {
			t.Fatal(err)
		}
		if !slices.EqualFunc(read.retained.roots, set, (*RetainedRoot).EqualVT) {
			t.Fatalf("%s: the Volume's retained set is %v, want %v", step, read.retained.roots, set)
		}
		var gotMissing, gotUnchecked []string
		for _, w := range b.retained.waits {
			if w.state == rootMissing {
				gotMissing = append(gotMissing, w.root.GetName())
			} else {
				gotUnchecked = append(gotUnchecked, w.root.GetName())
			}
		}
		shown, _ := b.RootWaits()
		var gotShown []string
		for _, w := range shown {
			gotShown = append(gotShown, w.GetName())
		}
		if !slices.Equal(gotMissing, missing) || !slices.Equal(gotShown, missing) || !slices.Equal(gotUnchecked, unchecked) {
			t.Fatalf("%s: missing %v, shown %v, unchecked %v, want missing and shown %v, unchecked %v", step, gotMissing, gotShown, gotUnchecked, missing, unchecked)
		}
		collect()
		for _, ref := range held {
			found, err := bs.GetBlockExists(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("%s: the collection freed held retained root %s", step, ref.MarshalString())
			}
		}
	}

	// The switch restores the stop: the install fails and holds nothing.
	refA, refM := kept("A"), absent("M")
	rootA := &RetainedRoot{Name: "A", RootRef: refA}
	rootM := &RetainedRoot{Name: "M", RootRef: refM}
	t.Run("off", func(t *testing.T) {
		t.Setenv(RetainedWaitEnv, "off")
		if err := install(a, rootA, rootM); !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("the install with the wait off returned %v, want block.ErrUnavailable", err)
		}
		if !a.held[RootRetained].GetEmpty() || len(a.retained.waits) != 0 {
			t.Fatalf("the stopped install held %v and recorded waits %v", a.held[RootRetained], a.retained.waits)
		}
	})

	// The install that retains M lands with every root, M missing, and
	// tells the driver.
	_, changed := a.RootWaits()
	if err := install(a, rootA, rootM); err != nil {
		t.Fatalf("the install of a root no source serves stopped replay: %v", err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("the missing root did not wake the driver's report")
	}
	expect(a, "first install", []*RetainedRoot{rootA, rootM}, []string{"M"}, nil, refA)

	// Installs of the same set, and of one adding B, neither try M again on
	// the install path nor drop it.
	tries := store.triesOf(refM)
	refB := kept("B")
	rootB := &RetainedRoot{Name: "B", RootRef: refB}
	if err := install(a, rootA, rootM); err != nil {
		t.Fatal(err)
	}
	if err := install(a, rootA, rootM, rootB); err != nil {
		t.Fatal(err)
	}
	if n := store.triesOf(refM) - tries; n != 0 {
		t.Fatalf("installs tried the missing root %d times on the install path", n)
	}
	expect(a, "installs while M waits", []*RetainedRoot{rootA, rootM, rootB}, []string{"M"}, nil, refA, refB)

	// The retrier finds no peer serving M, and M keeps waiting.
	if waiting, err := a.retryRetained(ctx); err != nil || !waiting {
		t.Fatalf("the retry of an unserved root returned waiting %v, error %v", waiting, err)
	}
	if store.triesOf(refM) != tries+1 {
		t.Fatal("the retrier did not try the missing root")
	}
	expect(a, "unserved retry", []*RetainedRoot{rootA, rootM, rootB}, []string{"M"}, nil, refA, refB)

	// A peer serves M. The retrier brings it in and holds it, and it
	// survives a collection after the retrier's pin ended, held only by the
	// set that landed before its block arrived.
	store.serve([]byte("retained World M"))
	if waiting, err := a.retryRetained(ctx); err != nil || waiting {
		t.Fatalf("the retry of a served root returned waiting %v, error %v", waiting, err)
	}
	expect(a, "served", []*RetainedRoot{rootA, rootM, rootB}, nil, nil, refA, refM, refB)

	// N waits as missing. Then a peer serves N and C. An install adding C
	// and P, which no peer serves, copies the waits, then waits at C's pin
	// while the retrier holds N. The install must not put N back among the
	// waits.
	refN := absent("N")
	rootN := &RetainedRoot{Name: "N", RootRef: refN}
	if err := install(a, rootA, rootM, rootB, rootN); err != nil {
		t.Fatal(err)
	}
	expect(a, "N missing", []*RetainedRoot{rootA, rootM, rootB, rootN}, []string{"N"}, nil, refA, refM, refB)
	store.serve([]byte("retained World N"))
	refC := store.serve([]byte("retained World C"))
	refP := absent("P")
	rootC := &RetainedRoot{Name: "C", RootRef: refC}
	rootP := &RetainedRoot{Name: "P", RootRef: refP}
	gate := make(chan struct{})
	store.mtx.Lock()
	store.gate[refC.MarshalString()] = gate
	store.mtx.Unlock()
	all := []*RetainedRoot{rootA, rootM, rootB, rootN, rootC, rootP}
	installed := make(chan error, 1)
	go func() { installed <- install(a, all...) }()
	<-store.entered
	if waiting, err := a.retryRetained(ctx); err != nil || waiting {
		t.Fatalf("the retry of a served root during an install returned waiting %v, error %v", waiting, err)
	}
	close(gate)
	if err := <-installed; err != nil {
		t.Fatal(err)
	}
	expect(a, "served while an install lands", all, []string{"P"}, nil, refA, refM, refB, refN, refC)

	// An adapter opened again over the Volume starts every root unchecked,
	// shows none missing, and its retrier finds P still missing without
	// pinning the held roots again.
	b := &Adapter{
		moves:    newRootMoveQueue(false),
		le:       a.le,
		so:       a.so,
		pages:    block.NewPendingStore(store, 1<<20),
		held:     make(map[string]*block.BlockRef),
		retained: newRetainedHold(),
	}
	unlock, err := b.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = b.readHeldRoots(ctx, heldRootNames)
	if err == nil {
		err = b.readRetained(ctx)
	}
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	heldRefs := []*block.BlockRef{refA, refM, refB, refN, refC}
	expect(b, "load", all, nil, []string{"A", "M", "B", "N", "C", "P"}, heldRefs...)
	triesOf := func() []int {
		var out []int
		for _, ref := range heldRefs {
			out = append(out, store.triesOf(ref))
		}
		return out
	}
	before := triesOf()
	if waiting, err := b.retryRetained(ctx); err != nil || !waiting {
		t.Fatalf("the retry after load returned waiting %v, error %v", waiting, err)
	}
	if after := triesOf(); !slices.Equal(before, after) {
		t.Fatalf("the retry after load pinned held roots again: tries %v, then %v", before, after)
	}
	expect(b, "checked after load", all, []string{"P"}, nil, heldRefs...)

	// A root whose pin answers that no store holds it waits as missing, at
	// the install and at each retry, and the retrier holds it once a peer
	// serves it.
	refQ := absent("Q")
	rootQ := &RetainedRoot{Name: "Q", RootRef: refQ}
	store.mtx.Lock()
	store.notFound[refQ.MarshalString()] = true
	store.mtx.Unlock()
	if err := install(b, rootA, rootQ); err != nil {
		t.Fatalf("the install of a root no store holds stopped replay: %v", err)
	}
	expect(b, "Q not found", []*RetainedRoot{rootA, rootQ}, []string{"Q"}, nil, refA)
	if waiting, err := b.retryRetained(ctx); err != nil || !waiting {
		t.Fatalf("the retry of a root no store holds returned waiting %v, error %v", waiting, err)
	}
	expect(b, "Q not found at retry", []*RetainedRoot{rootA, rootQ}, []string{"Q"}, nil, refA)
	store.mtx.Lock()
	delete(store.notFound, refQ.MarshalString())
	store.mtx.Unlock()
	store.serve([]byte("retained World Q"))
	if waiting, err := b.retryRetained(ctx); err != nil || waiting {
		t.Fatalf("the retry of a served root returned waiting %v, error %v", waiting, err)
	}
	expect(b, "Q served", []*RetainedRoot{rootA, rootQ}, nil, nil, refA, refQ)

	// Only an install of a state that no longer retains a root releases it.
	if err := install(b, rootA); err != nil {
		t.Fatal(err)
	}
	expect(b, "dropped", []*RetainedRoot{rootA}, nil, nil, refA)
}

// TestResetPayloadsHoldsLostPayload checks, on a Volume that frees at once,
// that cursor saves hold a payload the store lost and no peer serves instead
// of stopping replay, and never ask the peers again for a payload the set
// already holds. The set holds a stored payload and a lost one, and the
// positions add a payload a peer serves and one no source serves. Both saves,
// the one that appends and the one that drops, land with all four held, the
// served one readable after collection, and each unserved payload asked for
// at most once, by the save that first holds it.
func TestResetPayloadsHoldsLostPayload(t *testing.T) {
	// Hold a stored payload and a lost one, with the bucket's pins standing
	// in for peers.
	ctx := t.Context()
	a, collect := newVolumeAdapter(t, 1<<20, time.Nanosecond)
	bs := a.so.GetBlockStore().(*bucketStore)
	store := &peerServedStore{
		bucketStore: bs,
		served:      make(map[string][]byte),
		notFound:    make(map[string]bool),
		tries:       make(map[string]int),
		gate:        make(map[string]chan struct{}),
		entered:     make(chan string, 1),
	}
	a.pages = block.NewPendingStore(store, 1<<20)
	staged, endStage, err := block.OpenStage(ctx, bs)
	if err != nil {
		t.Fatal(err)
	}
	live := putPayloads(ctx, t, staged, "stored payload ", 1)[0]
	lost, err := block.BuildBlockRef([]byte("lost payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addPayloads(ctx, []*block.BlockRef{live, lost}, false); err != nil {
		t.Fatal(err)
	}
	endStage()

	// Add positions referencing them, a payload a peer serves, and one no
	// source serves.
	served := store.serve([]byte("served payload"))
	gone, err := block.BuildBlockRef([]byte("unserved payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	all := []*block.BlockRef{live, lost, served, gone}
	w := putPayloads(ctx, t, a.pages, "World ", 1)[0]
	a.replay = &spaceReplay{a: a, stage: newReplayStage(a.pages), base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: w}}, positions: []spacePosition{
		{payloads: []*block.BlockRef{live, lost}},
		{payloads: []*block.BlockRef{served, gone}},
	}}
	holdOrder(a, w, all, false)

	// check requires every payload held, the stored and served ones
	// readable after collection, and the pins asked for.
	check := func(step string) {
		t.Helper()
		for _, ref := range all {
			if !a.payloads.holds(ref) {
				t.Fatalf("%s: payload set %v dropped %s", step, a.payloads.roots, ref.MarshalString())
			}
		}
		collect()
		for _, ref := range []*block.BlockRef{live, served} {
			if found, err := bs.GetBlockExists(ctx, ref); err != nil || !found {
				t.Fatalf("%s: collection freed held payload %s: %v, %v", step, ref.MarshalString(), found, err)
			}
		}
		if n := store.triesOf(lost); n != 0 {
			t.Fatalf("%s: saves asked the peers %d times for the lost payload the set holds", step, n)
		}
		if n := store.triesOf(gone); n != 1 {
			t.Fatalf("%s: saves asked the peers %d times for the unserved payload, want once", step, n)
		}
	}

	// A save that appends holds them all.
	if err := a.resetPayloads(ctx, false); err != nil {
		t.Fatalf("the save holding a lost payload stopped replay: %v", err)
	}
	check("append")

	// A save that may drop keeps them all, since the order names them.
	if err := a.resetPayloads(ctx, true); err != nil {
		t.Fatalf("the dropping save holding a lost payload stopped replay: %v", err)
	}
	check("drop")
}
