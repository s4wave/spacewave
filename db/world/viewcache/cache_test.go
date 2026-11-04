package viewcache

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/testbed"
)

type testStore struct{ engine world.Engine }

func readsStore(t *testing.T) (*testStore, world.Engine) {
	// Start a real World with the production changelog.
	t.Helper()
	tb, err := world_testbed.Default(t.Context(), world_testbed.WithChangelog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	return &testStore{engine: tb.Engine}, tb.Engine
}

func seed(t *testing.T, engine world.Engine, key string) {
	// Commit one object so subsequent snapshots carry its changed key.
	t.Helper()
	ctx := t.Context()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRevisionCache(t *testing.T) {
	// compute counts its calls and reads a; read opens a snapshot.
	store, engine := readsStore(t)
	ctx := t.Context()
	cache := &Cache{engine: engine}
	calls := 0
	compute := func(value int) func(context.Context, world.WorldState) (any, error) {
		return func(ctx context.Context, tx world.WorldState) (any, error) {
			calls++
			if _, err := tx.HasObject(ctx, "a"); err != nil {
				return nil, err
			}
			return value, nil
		}
	}
	read := func() world.Tx {
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tx.Discard)
		return tx
	}

	// One revision computes the value once.
	old := read()
	for range 3 {
		if got, err := cache.Get(ctx, old, "v", compute(5)); err != nil || got != 5 {
			t.Fatal(got, err)
		}
	}
	if calls != 1 {
		t.Fatal("a revision's value was computed", calls, "times")
	}

	// An unrelated write keeps it; a write to a replaces it, for older readers
	// too.
	seed(t, engine, "b")
	if got, _ := cache.Get(ctx, read(), "v", compute(6)); got != 5 || calls != 1 {
		t.Fatal("an unrelated revision recomputed the value", got, calls)
	}
	seed(t, engine, "a")
	current := read()
	if got, _ := cache.Get(ctx, current, "v", compute(7)); got != 7 {
		t.Fatal("a change to what the value read kept the old value", got)
	}
	if got, _ := cache.Get(ctx, old, "v", compute(4)); got != 7 || calls != 2 {
		t.Fatal("an older revision did not share the newest value", got, calls)
	}

	// A value expires at the time it asked to rerun at.
	expiring := func(ctx context.Context, _ world.WorldState) (any, error) {
		calls++
		world.RerunAt(ctx, time.Now())
		return calls, nil
	}
	first, _ := cache.Get(ctx, current, "expiring", expiring)
	if again, _ := cache.Get(ctx, current, "expiring", expiring); again == first {
		t.Fatal("a value outlived the time it asked to rerun at")
	}

	// A failure reaches its caller and is not kept.
	if _, err := cache.Get(ctx, current, "fails", func(context.Context, world.WorldState) (any, error) { return nil, errors.New("boom") }); err == nil {
		t.Fatal("a failure was not returned")
	}
	if got, err := cache.Get(ctx, current, "fails", compute(8)); err != nil || got != 8 {
		t.Fatal("a failure was cached", got, err)
	}

	// A World ahead of its cache does not repeatedly wake cache readers.
	lagging := &Cache{engine: engine}
	before, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lagging.seqno.Store(before)

	// Publish a World revision without moving its watched cache.
	seed(t, engine, "ahead-of-cache")
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lagging.watched.Store(after)

	// Drive cache progress and deadline expiry without wall-clock sleeps.
	synctest.Test(t, func(t *testing.T) {
		// Sample before reading, then wait while the World is already ahead.
		change := lagging.NextChange()
		result := make(chan error, 1)
		go func() {
			result <- change.Wait(t.Context())
		}()
		synctest.Wait()
		if len(result) != 0 {
			t.Fatal("a lagging cache woke on the World's newer revision")
		}

		// Publishing the cache's actual advance wakes the same pending read.
		if err := lagging.Advance(t.Context(), after); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if err := <-result; err != nil {
			t.Fatal(err)
		}

		// A timer requests a reread; cancellation still ends the routine.
		change = lagging.NextChange()
		timed, end := context.WithTimeout(t.Context(), time.Second)
		defer end()
		if err := change.Wait(timed); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline = %v", err)
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := change.Wait(cancelled); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait = %v", err)
		}
	})
}

// TestRevisionCacheIndexedParts keeps a value built from a part across a write
// neither read, and drops both when the part's read changes: the index names
// the outer value through the part's reads.
func TestRevisionCacheIndexedParts(t *testing.T) {
	// The outer value reads b and reuses the part, which reads a.
	store, engine := readsStore(t)
	ctx := t.Context()
	cache := &Cache{engine: engine}
	calls := map[string]int{}
	part := func(ctx context.Context, tx world.WorldState) (any, error) {
		calls["part"]++
		return tx.HasObject(ctx, "a")
	}
	outer := func(ctx context.Context, tx world.WorldState) (any, error) {
		calls["outer"]++
		if _, err := tx.HasObject(ctx, "b"); err != nil {
			return nil, err
		}
		return cache.Get(ctx, tx, "part", part)
	}

	// get reads the value name at the newest revision.
	get := func(name string, compute func(context.Context, world.WorldState) (any, error)) any {
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		got, err := cache.Get(ctx, tx, name, compute)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// Cache the part first, so the outer value reuses it without reading a
	// itself. A write neither read keeps both values.
	get("part", part)
	get("outer", outer)
	seed(t, engine, "c")
	get("outer", outer)
	if calls["outer"] != 1 || calls["part"] != 1 {
		t.Fatal("an unrelated write recomputed", calls)
	}

	// A write the part read drops the outer value too.
	seed(t, engine, "a")
	if got := get("outer", outer); got != true || calls["outer"] != 2 || calls["part"] != 2 {
		t.Fatal("a write to the part's read kept a value", got, calls)
	}
}

// TestRevisionCacheComputingAcrossRevisions computes a value while the World
// moves on: a reader at the newer revision shares it when the changes left its
// reads alone, and computes its own when they touched them.
func TestRevisionCacheComputingAcrossRevisions(t *testing.T) {
	// compute reads a, holding until release closes.
	store, engine := readsStore(t)
	ctx := t.Context()
	cache := &Cache{engine: engine}
	read := func() world.Tx {
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tx.Discard)
		return tx
	}
	var calls atomic.Int32
	held := func(started, release chan struct{}) func(context.Context, world.WorldState) (any, error) {
		return func(ctx context.Context, tx world.WorldState) (any, error) {
			n := calls.Add(1)
			if _, err := tx.HasObject(ctx, "a"); err != nil {
				return nil, err
			}
			if started != nil {
				close(started)
				<-release
			}
			return n, nil
		}
	}

	for _, touch := range []bool{false, true} {
		// Start a computation at the current revision and hold it.
		name := "v" + strconv.FormatBool(touch)
		calls.Store(0)
		started, release := make(chan struct{}), make(chan struct{})
		first := make(chan any, 1)
		old := read()
		go func() {
			value, err := cache.Get(ctx, old, name, held(started, release))
			if err != nil {
				t.Error(err)
			}
			first <- value
		}()
		<-started

		// Move the World on, touching a or not, and read at the new
		// revision while the first computation still runs.
		key := "unrelated-" + name
		if touch {
			key = "a"
		}
		seed(t, engine, key)
		second := make(chan any, 1)
		newer := read()
		go func() {
			value, err := cache.Get(ctx, newer, name, held(nil, nil))
			if err != nil {
				t.Error(err)
			}
			second <- value
		}()

		// Once the newer reader moved the cache past the first computation,
		// release it: the newer reader shares it only when a was left alone.
		seqno, err := newer.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			cache.mtx.Lock()
			caught := cache.seqno.Load() >= seqno
			cache.mtx.Unlock()
			if caught {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the newer reader never moved the cache")
			}
		}
		close(release)
		if got := <-first; got != int32(1) {
			t.Fatal("the first reader got", got)
		}
		got := <-second
		if want := int32(1); !touch && (got != want || calls.Load() != 1) {
			t.Fatal("an untouched value was not shared:", got, calls.Load())
		}
		if touch && (got != int32(2) || calls.Load() != 2) {
			t.Fatal("a touched value was shared at the newer revision:", got, calls.Load())
		}
	}
}

// TestRevisionCachePartBehindHead reads a part while computing a value whose
// snapshot the cache moves past: the part comes from the value's own snapshot,
// without opening the newest revision, and is not kept for newer readers.
func TestRevisionCachePartBehindHead(t *testing.T) {
	// part reports whether c exists in the snapshot it reads.
	store, engine := readsStore(t)
	ctx := t.Context()
	opens := &countingEngine{Engine: engine}
	cache := &Cache{engine: opens}
	parts := 0
	part := func(ctx context.Context, tx world.WorldState) (any, error) {
		parts++
		return tx.HasObject(ctx, "c")
	}
	read := func() world.Tx {
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tx.Discard)
		return tx
	}

	// While the outer value computes before c, c is written and a newer
	// reader moves the cache past the outer snapshot.
	got, err := cache.Get(ctx, read(), "outer", func(ctx context.Context, tx world.WorldState) (any, error) {
		seed(t, engine, "c")
		if _, err := cache.Get(ctx, read(), "newer", func(context.Context, world.WorldState) (any, error) { return 0, nil }); err != nil {
			return nil, err
		}
		return cache.Get(ctx, tx, "part", part)
	})
	if err != nil || got != false {
		t.Fatal("the part did not come from the outer snapshot", got, err)
	}
	if opens.opened.Load() != 0 {
		t.Fatal("the part opened", opens.opened.Load(), "transactions")
	}

	// A reader at the newest revision computes the part again.
	if got, err := cache.Get(ctx, read(), "part", part); err != nil || got != true || parts != 2 {
		t.Fatal("the outer snapshot's part was kept", got, err, parts)
	}

	// A part computed behind the cache whose reads the changes since left
	// alone is kept: the next reader shares it. The cache reads changes from
	// the engine itself, which names what changed.
	cache = &Cache{engine: engine}
	untouched := 0
	other := func(ctx context.Context, tx world.WorldState) (any, error) {
		untouched++
		return tx.HasObject(ctx, "d")
	}
	if _, err := cache.Get(ctx, read(), "outer-2", func(ctx context.Context, tx world.WorldState) (any, error) {
		seed(t, engine, "e")
		if _, err := cache.Get(ctx, read(), "newer-2", func(context.Context, world.WorldState) (any, error) { return 0, nil }); err != nil {
			return nil, err
		}
		return cache.Get(ctx, tx, "other", other)
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Get(ctx, read(), "other", other); err != nil || got != false || untouched != 1 {
		t.Fatal("an untouched part computed behind the cache was not kept", got, err, untouched)
	}
}

// TestRevisionCacheAbandoned cancels the caller computing a shared value while
// others wait on it: the waiters share one computation again instead of each
// computing the value alone, with a transaction for each part.
func TestRevisionCacheAbandoned(t *testing.T) {
	// The first computation blocks until its caller is cancelled.
	store, engine := readsStore(t)
	ctx := t.Context()
	opens := &countingEngine{Engine: engine}
	cache := &Cache{engine: opens}
	tx, err := store.engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	started := make(chan struct{})
	var calls atomic.Int32
	compute := func(ctx context.Context, tx world.WorldState) (any, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return cache.Get(ctx, tx, "part", func(context.Context, world.WorldState) (any, error) { return 5, nil })
	}
	owner, cancel := context.WithCancel(ctx)
	ownerErr := make(chan error, 1)
	go func() {
		_, err := cache.Get(owner, tx, "v", compute)
		ownerErr <- err
	}()
	<-started

	// Waiters queue behind it, then its caller leaves.
	const waiters = 8
	results := make(chan any, waiters)
	for range waiters {
		go func() {
			got, err := cache.Get(ctx, tx, "v", compute)
			if err != nil {
				got = err
			}
			results <- got
		}()
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-ownerErr; !errors.Is(err, context.Canceled) {
		t.Fatal("the abandoned caller got", err)
	}

	// The waiters share one computation and open no transaction.
	for range waiters {
		if got := <-results; got != 5 {
			t.Fatal("a waiter got", got)
		}
	}
	if calls.Load() != 2 || opens.opened.Load() != 0 {
		t.Fatal("waiters computed", calls.Load()-1, "times and opened", opens.opened.Load(), "transactions")
	}
}

func TestRevisionCacheParts(t *testing.T) {
	// The composite reads b and the part, which reads partKey.
	store, engine := readsStore(t)
	ctx := t.Context()
	cache := &Cache{engine: engine}
	partKey := "a"
	parts, composites := 0, 0
	part := func(ctx context.Context, tx world.WorldState) (any, error) {
		parts++
		return tx.HasObject(ctx, partKey)
	}
	composite := func(ctx context.Context, tx world.WorldState) (any, error) {
		composites++
		if _, err := tx.HasObject(ctx, "b"); err != nil {
			return nil, err
		}
		return cache.Get(ctx, tx, "part", part)
	}
	get := func(name string) any {
		t.Helper()
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		compute := part
		if name == "composite" {
			compute = composite
		}
		value, err := cache.Get(ctx, tx, name, compute)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	expect := func(wantParts, wantComposites int) {
		t.Helper()
		get("composite")
		get("part")
		if parts != wantParts || composites != wantComposites {
			t.Fatal("computed the part", parts, "and the composite", composites, "times")
		}
	}

	// The composite records the part it reused.
	expect(1, 1)
	cache.mtx.Lock()
	recorded := len(cache.entries["composite"].parts)
	cache.mtx.Unlock()
	if recorded != 1 {
		t.Fatal("the composite recorded", recorded, "parts")
	}

	// An unrelated change keeps both; one to b keeps the part; one to a
	// drops both.
	seed(t, engine, "c")
	expect(1, 1)
	seed(t, engine, "b")
	expect(1, 2)
	seed(t, engine, "a")
	expect(2, 3)

	// A part computed again, apart from the composite, is checked itself:
	// the composite's untouched reads no longer cover it.
	cache.Forget("part")
	partKey = "d"
	expect(3, 3)
	seed(t, engine, "d")
	if get("part"); parts != 4 {
		t.Fatal("a change to a part computed apart from its composite kept it")
	}

	// A value reading a part through a facet survives a change to the part
	// that leaves its sub-key alone, and drops when the sub-key changes.
	facetCache := &Cache{engine: engine}
	table := map[string]int{"x": 1, "y": 1}
	partRuns, readerRuns := 0, map[string]int{}
	facetPart := func(ctx context.Context, tx world.WorldState) (any, error) {
		partRuns++
		for _, key := range []string{"f1", "f2", "f3"} {
			if _, err := tx.HasObject(ctx, key); err != nil {
				return nil, err
			}
		}
		return maps.Clone(table), nil
	}
	reader := func(key string) func(context.Context, world.WorldState) (any, error) {
		return func(ctx context.Context, tx world.WorldState) (any, error) {
			readerRuns[key]++
			value, err := facetCache.Get(ctx, tx, "facet-part", facetPart)
			if err != nil {
				return nil, err
			}
			facetCache.Facet(ctx, "facet-part", facetPart, func(previous, next any) bool {
				return previous.(map[string]int)[key] != next.(map[string]int)[key]
			})
			return value.(map[string]int)[key], nil
		}
	}
	readFacets := func() (int, int) {
		t.Helper()
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		x, err := facetCache.Get(ctx, tx, "reader:x", reader("x"))
		if err != nil {
			t.Fatal(err)
		}
		y, err := facetCache.Get(ctx, tx, "reader:y", reader("y"))
		if err != nil {
			t.Fatal(err)
		}
		return x.(int), y.(int)
	}
	if x, y := readFacets(); x != 1 || y != 1 || partRuns != 1 {
		t.Fatal("first facet reads", x, y, partRuns)
	}
	table["x"] = 2
	seed(t, engine, "f1")
	if x, y := readFacets(); x != 2 || y != 1 || readerRuns["x"] != 2 || readerRuns["y"] != 1 || partRuns != 2 {
		t.Fatal("a change to x:", x, y, readerRuns, partRuns)
	}
	seed(t, engine, "f2")
	if x, y := readFacets(); x != 2 || y != 1 || readerRuns["x"] != 2 || readerRuns["y"] != 1 {
		t.Fatal("a change to neither key recomputed a reader", readerRuns)
	}
	table["y"] = 3
	seed(t, engine, "f3")
	if x, y := readFacets(); x != 2 || y != 3 || readerRuns["x"] != 2 || readerRuns["y"] != 2 {
		t.Fatal("a change to y:", x, y, readerRuns)
	}
}

func TestRevisionCacheFacetHolder(t *testing.T) {
	// The facet part reads the keys listed and serves the table; the reader
	// reads x through a facet; the holder reuses the reader.
	store, engine := readsStore(t)
	ctx := t.Context()
	cache := &Cache{engine: engine}
	table, keys := map[string]int{"x": 1}, []string{"f1"}
	facetPart := func(ctx context.Context, tx world.WorldState) (any, error) {
		for _, key := range keys {
			if _, err := tx.HasObject(ctx, key); err != nil {
				return nil, err
			}
		}
		return maps.Clone(table), nil
	}
	reader := func(ctx context.Context, tx world.WorldState) (any, error) {
		value, err := cache.Get(ctx, tx, "facet-part", facetPart)
		if err != nil {
			return nil, err
		}
		cache.Facet(ctx, "facet-part", facetPart, func(previous, next any) bool {
			return previous.(map[string]int)["x"] != next.(map[string]int)["x"]
		})
		return value.(map[string]int)["x"], nil
	}
	holder := func(ctx context.Context, tx world.WorldState) (any, error) {
		return cache.Get(ctx, tx, "reader", reader)
	}

	// get reads the value name at the newest revision.
	get := func(name string, compute func(context.Context, world.WorldState) (any, error)) any {
		t.Helper()
		tx, err := store.engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		value, err := cache.Get(ctx, tx, name, compute)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}

	// Build each value apart, so none reads another's keys itself.
	get("facet-part", facetPart)
	get("reader", reader)
	if got := get("holder", holder); got != 1 {
		t.Fatal("first holder value", got)
	}

	// The part computed again now reads g, leaving x alone: the reader and
	// the holder stay, pointing at the newer part.
	keys = append(keys, "g")
	seed(t, engine, "f1")
	if got := get("holder", holder); got != 1 {
		t.Fatal("holder after f1", got)
	}

	// A write to g changes x: the holder drops with the reader.
	table["x"] = 5
	seed(t, engine, "g")
	if got := get("holder", holder); got != 5 {
		t.Fatal("a change to the newer part's read kept the holder", got)
	}
}

func TestRevisionCachePartsOtherWorld(t *testing.T) {
	// The composite reuses a value of a second cache.
	store, engine := readsStore(t)
	other, otherEngine := readsStore(t)
	ctx := t.Context()
	cache, otherCache := &Cache{engine: engine}, &Cache{engine: otherEngine}
	otherTx, err := other.engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer otherTx.Discard()
	tx, err := store.engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, err := cache.Get(ctx, tx, "composite", func(ctx context.Context, _ world.WorldState) (any, error) {
		return otherCache.Get(ctx, otherTx, "part", func(ctx context.Context, tx world.WorldState) (any, error) {
			return tx.HasObject(ctx, "a")
		})
	}); err != nil {
		t.Fatal(err)
	}

	// The second cache's value is not a part of the first's.
	cache.mtx.Lock()
	defer cache.mtx.Unlock()
	if parts := cache.entries["composite"].parts; len(parts) != 0 {
		t.Fatal("the composite recorded another World's value as a part")
	}
}

// countingEngine counts the transactions opened on its Engine.
type countingEngine struct {
	world.Engine
	opened atomic.Int64
	// held, when set, holds each change watch until it closes, as a slow
	// diff does.
	held atomic.Pointer[chan struct{}]
}

// NewTransaction counts the transaction and opens it on the Engine.
func (e *countingEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	e.opened.Add(1)

	return e.Engine.NewTransaction(ctx, write)
}

// WatchChanges forwards the Engine's change feed, which embedding the Engine
// interface alone would hide, once held lets it through.
func (e *countingEngine) WatchChanges(ctx context.Context, afterSeqno uint64, touches world.ChangeFilter, cb func(uint64, *world.ChangeSet) error) error {
	if held := e.held.Load(); held != nil {
		select {
		case <-*held:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return world.WatchTouchingChanges(ctx, e.Engine, afterSeqno, touches, cb)
}

func TestValuesAndViewsHoldTheirBlocks(t *testing.T) {
	// Start a World with one object and a cache over it, with holds on.
	block.EnableRefHolds()
	_, engine := readsStore(t)
	seed(t, engine, "a")
	ctx := t.Context()
	cache := &Cache{engine: engine}
	hasA := func(ctx context.Context, tx world.WorldState) (any, error) {
		return tx.HasObject(ctx, "a")
	}

	// A view evaluation holds the blocks it reads, and the cached value it
	// reuses holds its own, until the evaluation is replaced or the view
	// ends.
	stop := errors.New("stop")
	var viewHold *block.RefHold
	err := world.WatchView(ctx, engine, func(ctx context.Context, ws world.WorldState) error {
		viewHold = block.RefHoldFromContext(ctx)
		if _, err := cache.Get(ctx, ws, "has-a", hasA); err != nil {
			return err
		}
		if _, err := ws.HasObject(ctx, "a"); err != nil {
			return err
		}
		if viewHold.Len() == 0 {
			t.Error("view evaluation held none of the blocks it read")
		}
		if cache.entries["has-a"].blocks.Len() == 0 {
			t.Error("cached value held none of the blocks it read")
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if viewHold.Len() != 0 {
		t.Fatal("ended view still holds its blocks")
	}
}
