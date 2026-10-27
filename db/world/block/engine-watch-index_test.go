package world_block

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
)

// TestWatchIndexedReadSetGrows checks that a ReadSet waiting on the Engine's
// indexed change watch is woken only by changes it names, and that a read
// recorded during the wait reaches the index: a change committed before the
// read ends the wait, and so does one committed after it.
func TestWatchIndexedReadSetGrows(t *testing.T) {
	// Create the object a view reads.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	e := newRetirementTestEngine(t, ctx)
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "grow/read")

	// Record the read and start waiting after it.
	reads := world.NewReadSet()
	tracked := reads.Track(world.NewEngineWorldState(e, false))
	readWatchTestObject(t, ctx, tracked, "grow/read")
	result := startIndexedWait(ctx, e, reads, headSeqno(t, ctx, e))

	// The index names only the read object.
	sub := waitIndexedSubscriber(t, ctx, e)
	if candidates := e.indexCandidates(&world.ChangeSet{Keys: []string{"grow/other"}}); slices.Contains(candidates, sub) {
		t.Fatal("an unread key names the waiting view")
	}

	// Commit the object the view reads next, then an unrelated one.
	createWatchTestObject(t, ctx, ws, "grow/late")
	lateSeqno := headSeqno(t, ctx, e)
	createWatchTestObject(t, ctx, ws, "grow/other")
	select {
	case r := <-result:
		t.Fatalf("wait ended at %d before the view read the changed object: %v", r.seqno, r.err)
	default:
	}

	// Reading the earlier change's object ends the wait. Catch-up merges the
	// recorded spans, so it reports the head they reach.
	readWatchTestObject(t, ctx, tracked, "grow/late")
	expectIndexedWait(t, ctx, result, lateSeqno)

	// A read recorded during a new wait is indexed for a later change.
	result = startIndexedWait(ctx, e, reads, headSeqno(t, ctx, e))
	waitIndexedSubscriber(t, ctx, e)
	readWatchTestObject(t, ctx, tracked, "grow/next")
	waitIndexedName(t, ctx, e, "grow/next")
	createWatchTestObject(t, ctx, ws, "grow/next")
	expectIndexedWait(t, ctx, result, headSeqno(t, ctx, e))
}

// TestWatchIndexWriteNotBlocked checks that a World write commits while the
// change index is busy naming a subscriber, and that the subscribers waiting
// on the index still see the write once it frees.
func TestWatchIndexWriteNotBlocked(t *testing.T) {
	// Open an Engine with one object.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	e := newRetirementTestEngine(t, ctx)
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "busy/base")

	// Wait on an object a view reads before it exists.
	reads := world.NewReadSet()
	tracked := reads.Track(world.NewEngineWorldState(e, false))
	readWatchTestObject(t, ctx, tracked, "busy/next")
	before := headSeqno(t, ctx, e)
	indexed := startIndexedWait(ctx, e, reads, before)
	waitIndexedSubscriber(t, ctx, e)

	// Hold the index as a subscriber naming a large interest would, and
	// subscribe a second view, which waits for the index.
	locked := e.bcast.Lock()
	w := e.changeWatch
	locked.Unlock()
	w.indexMtx.Lock()
	joining := startIndexedWait(ctx, e, reads, before)

	// Create the object the views read while the index is held.
	written := make(chan error, 1)
	go func() {
		obj, _, err := world.CreateWorldObject(ctx, ws, "busy/next", func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample("next"), true)
			return nil
		})
		world.ReleaseObjectState(obj)
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			w.indexMtx.Unlock()
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		w.indexMtx.Unlock()
		t.Fatal("a World write waited for the change index")
	}

	// Both views see the write once the index frees.
	w.indexMtx.Unlock()
	after := headSeqno(t, ctx, e)
	expectIndexedWait(t, ctx, indexed, after)
	expectIndexedWait(t, ctx, joining, after)
}

// indexedWait is the result of a ReadSet wait.
type indexedWait struct {
	seqno uint64
	err   error
}

// startIndexedWait starts waiting for a change to reads after seqno.
func startIndexedWait(ctx context.Context, e *Engine, reads *world.ReadSet, seqno uint64) <-chan indexedWait {
	result := make(chan indexedWait, 1)
	go func() {
		got, err := reads.WaitChange(ctx, e, seqno)
		result <- indexedWait{got, err}
	}()
	return result
}

// expectIndexedWait expects the wait to end at or past seqno.
func expectIndexedWait(t *testing.T, ctx context.Context, result <-chan indexedWait, seqno uint64) {
	t.Helper()
	select {
	case r := <-result:
		if r.err != nil || r.seqno < seqno {
			t.Fatalf("wait ended at %d with %v, want at least %d", r.seqno, r.err, seqno)
		}
	case <-ctx.Done():
		t.Fatalf("wait did not end at %d", seqno)
	}
}

// readWatchTestObject reads an object, which may not exist, through ws.
func readWatchTestObject(t *testing.T, ctx context.Context, ws world.WorldState, key string) {
	t.Helper()
	obj, _, err := ws.GetObject(ctx, key)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
}

// waitIndexedSubscriber waits for the change watch to hold one subscriber and
// returns it.
func waitIndexedSubscriber(t *testing.T, ctx context.Context, e *Engine) *changeSubscriber {
	t.Helper()
	for {
		// Return the lone subscriber once it is indexed.
		subs := e.indexedSubscribers()
		if len(subs) == 1 {
			return subs[0]
		}

		// Poll until the test deadline.
		select {
		case <-ctx.Done():
			t.Fatalf("change watch holds %d subscribers, want 1", len(subs))
		case <-time.After(time.Millisecond):
		}
	}
}

// waitIndexedName waits until a change to key names the change watch's
// subscriber, which happens once the restarted wait indexes the new read.
func waitIndexedName(t *testing.T, ctx context.Context, e *Engine, key string) {
	t.Helper()
	for len(e.indexCandidates(&world.ChangeSet{Keys: []string{key}})) == 0 {
		select {
		case <-ctx.Done():
			t.Fatalf("no subscriber is indexed by %q", key)
		case <-time.After(time.Millisecond):
		}
	}
}

// indexedSubscribers returns the subscribers the change watch has indexed.
func (e *Engine) indexedSubscribers() []*changeSubscriber {
	// Find the running change watch.
	locked := e.bcast.Lock()
	w := e.changeWatch
	locked.Unlock()
	if w == nil {
		return nil
	}

	// Read its index under the index lock.
	w.indexMtx.Lock()
	defer w.indexMtx.Unlock()
	return slices.Collect(w.index.All())
}

// indexCandidates returns the subscribers the change watch would test on
// changes.
func (e *Engine) indexCandidates(changes *world.ChangeSet) []*changeSubscriber {
	// Find the running change watch.
	locked := e.bcast.Lock()
	w := e.changeWatch
	locked.Unlock()
	if w == nil {
		return nil
	}

	// Read its index under the index lock.
	w.indexMtx.Lock()
	defer w.indexMtx.Unlock()
	return w.index.Candidates(changes)
}
