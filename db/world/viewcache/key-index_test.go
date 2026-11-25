package viewcache

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
)

// keyPred is the predicate the key index tests follow.
const keyPred = "test/member"

// editEdge sets or deletes the edge subject keyPred object in one commit.
func editEdge(t *testing.T, engine world.Engine, subject, object string, set bool) {
	// Open a write.
	t.Helper()
	ctx := t.Context()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Set or delete the edge and commit.
	q := world.NewGraphQuadWithKeys(subject, keyPred, object, "")
	if set {
		err = tx.SetGraphQuad(ctx, q)
	} else {
		err = tx.DeleteGraphQuad(ctx, q)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// followedCache returns a cache the World's change watch feeds, as a server
// runs it. The index it holds makes each catch-up read its changes.
func followedCache(t *testing.T, engine world.Engine) *Cache {
	// Follow from the World's revision now.
	t.Helper()
	ctx := t.Context()
	cache := &Cache{engine: engine}
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cache.Follow(after)
	t.Cleanup(cache.Unfollow)

	// Feed it each later revision.
	go func() {
		_ = world.WatchChanges(ctx, engine, after, func(seqno uint64, _ *world.ChangeSet) error {
			cache.Follow(seqno)
			return nil
		})
	}()
	return cache
}

// TestKeyIndexViewReruns runs a view that answers from the index while its
// key has no edge, and walks the graph otherwise. It must never miss its own
// key's change: each edit to the key's edges reruns it to the World's answer,
// including an edge written while the index still stood behind the view.
func TestKeyIndexViewReruns(t *testing.T) {
	// b has an edge before the index builds; a has none.
	_, engine := readsStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, key := range []string{"a", "b", "m1", "m2"} {
		seed(t, engine, key)
	}
	editEdge(t, engine, "b", "m1", true)
	cache := followedCache(t, engine)
	index := cache.KeyIndex(keyPred, KeySubject)
	if keys, _ := waitBuilt(t, index); keys != 1 {
		t.Fatalf("index holds %d keys, want b alone", keys)
	}

	// The view reports a's edge count, and whether the index answered.
	type run struct {
		edges   int
		indexed bool
	}
	runs := make(chan run, 16)
	go func() {
		_ = world.WatchView(ctx, engine, func(ctx context.Context, ws world.WorldState) error {
			// Answer from the index at the snapshot's revision.
			seqno, err := ws.GetSeqno(ctx)
			if err != nil {
				return err
			}
			if index.Absent(ctx, seqno, "a") {
				runs <- run{0, true}
				return nil
			}

			// Otherwise walk the graph.
			edges, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("a", keyPred, "", ""), 0)
			if err != nil {
				return err
			}
			runs <- run{len(edges), false}
			return nil
		})
	}()
	next := func(want int) run {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case r := <-runs:
				if r.edges == want {
					return r
				}
			case <-deadline:
				t.Fatalf("the view never answered %d edges", want)
			}
		}
	}

	// The first answer comes from the index alone, so only the pattern it
	// recorded can rerun the view.
	if r := next(0); !r.indexed {
		t.Fatal("the built index did not answer the first view")
	}

	// a gains an edge: the view reruns and walks to it.
	editEdge(t, engine, "a", "m1", true)
	if r := next(1); r.indexed {
		t.Fatal("the index answered for a key with an edge")
	}

	// a loses it again: the view reruns and reads no edge.
	editEdge(t, engine, "a", "m1", false)
	next(0)

	// b's edges never name a, so the view answers from the index once it
	// patches past a's removal.
	editEdge(t, engine, "b", "m2", true)
	deadline := time.After(10 * time.Second)
	for {
		if keys, at := index.Size(); keys == 1 && at >= mustSeqno(t, engine) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the index never patched to the World")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestKeyIndexBehindWalks holds an index at an old revision whose set lacks a
// key the World has since given an edge: a reader at the newer revision must
// walk, not answer from it.
func TestKeyIndexBehindWalks(t *testing.T) {
	// a gains an edge after the old revision.
	_, engine := readsStore(t)
	ctx := t.Context()
	seed(t, engine, "a")
	seed(t, engine, "m1")
	old := mustSeqno(t, engine)
	editEdge(t, engine, "a", "m1", true)

	// An index published at the old revision, with no worker to move it.
	index := &KeyIndex{c: &Cache{engine: engine}, filter: world.NewGraphQuad("", keyPred, "", ""), first: make(chan struct{})}
	index.running.Store(true)
	index.publish(map[string]struct{}{}, old)

	// Only a reader at the old revision may use it.
	if index.Absent(ctx, mustSeqno(t, engine), "a") {
		t.Fatal("an index behind the reader answered for it")
	}
	if !index.Absent(ctx, old, "a") {
		t.Fatal("an index at the reader's revision did not answer")
	}
}

// TestKeyIndexPatches checks the set the worker keeps against the World as
// edges come and go, and as an object it lists is deleted.
func TestKeyIndexPatches(t *testing.T) {
	// a names m1 before the index builds.
	_, engine := readsStore(t)
	for _, key := range []string{"a", "b", "m1"} {
		seed(t, engine, key)
	}
	editEdge(t, engine, "a", "m1", true)
	cache := followedCache(t, engine)
	index := cache.KeyIndex(keyPred, KeyObject)
	if keys, _ := waitBuilt(t, index); keys != 1 {
		t.Fatalf("object index holds %d keys, want m1", keys)
	}

	// m1 stays while b also names it, and leaves with the last edge.
	expect := func(m1 bool) {
		t.Helper()
		seqno := mustSeqno(t, engine)
		deadline := time.After(10 * time.Second)
		for {
			if set := index.set.Load(); set != nil && set.at >= seqno {
				if _, has := set.keys["m1"]; has != m1 {
					t.Fatalf("m1 listed %v, want %v", has, m1)
				}
				return
			}
			select {
			case <-deadline:
				t.Fatal("the index never patched to the World")
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
	editEdge(t, engine, "b", "m1", true)
	expect(true)
	editEdge(t, engine, "a", "m1", false)
	expect(true)
	editEdge(t, engine, "b", "m1", false)
	expect(false)

	// Deleting m1 while b names it takes it out too.
	editEdge(t, engine, "b", "m1", true)
	expect(true)
	tx, err := engine.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, err := tx.DeleteObject(t.Context(), "m1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	expect(false)
}

// TestKeyIndexAbsentScales holds Absent's allocations flat between n and 10n
// keys: a reader costs one lookup, never a walk of the set.
func TestKeyIndexAbsentScales(t *testing.T) {
	ctx := t.Context()
	allocs := func(n int) float64 {
		// Publish n keys.
		keys := make(map[string]struct{}, n)
		for i := range n {
			keys["k"+strconv.Itoa(i)] = struct{}{}
		}
		index := &KeyIndex{filter: world.NewGraphQuad("", keyPred, "", ""), first: make(chan struct{})}
		index.publish(keys, 10)

		// Count one answer of each kind.
		return testing.AllocsPerRun(100, func() {
			if !index.Absent(ctx, 10, "missing") || index.Absent(ctx, 10, "k1") {
				t.Fatal("wrong answer")
			}
		})
	}
	if small, large := allocs(1000), allocs(10000); small != large {
		t.Fatalf("Absent allocates %v at n and %v at 10n", small, large)
	}
}

// waitBuilt waits for the index's first build and returns its size.
func waitBuilt(t *testing.T, index *KeyIndex) (int, uint64) {
	t.Helper()
	select {
	case <-index.first:
	case <-time.After(10 * time.Second):
		t.Fatal("the index never built")
	}
	return index.Size()
}

// mustSeqno returns the World's newest revision.
func mustSeqno(t *testing.T, engine world.Engine) uint64 {
	t.Helper()
	seqno, err := engine.GetSeqno(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return seqno
}

// TestKeyIndexCacheJumps opens a cache over a World with history behind it:
// with no value cached, its first catch-up jumps to the index's build without
// reading the World's changes since revision zero, then reads only the changes
// after the build.
func TestKeyIndexCacheJumps(t *testing.T) {
	// Write history the cache never passed, then build an index over it.
	_, engine := readsStore(t)
	ctx := t.Context()
	for i := range 20 {
		seed(t, engine, "k"+strconv.Itoa(i))
	}
	cache := &Cache{engine: engine}
	index := cache.KeyIndex(keyPred, KeySubject)
	_, built := waitBuilt(t, index)

	// The catch-up to the build reads no changes.
	if err := cache.Advance(ctx, built); err != nil {
		t.Fatal(err)
	}
	if cache.Seqno() != built || len(cache.history) != 0 {
		t.Fatalf("the cache stands at %d with %d catch-ups of history, want %d with none", cache.Seqno(), len(cache.history), built)
	}

	// The next one reads the changes after the build alone.
	editEdge(t, engine, "k1", "k2", true)
	if err := cache.Advance(ctx, mustSeqno(t, engine)); err != nil {
		t.Fatal(err)
	}
	if len(cache.history) != 1 || cache.history[0].from != built {
		t.Fatalf("the cache read %d catch-ups, want one from %d", len(cache.history), built)
	}
}
