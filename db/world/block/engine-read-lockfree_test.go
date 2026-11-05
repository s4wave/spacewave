package world_block

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// TestReadOpensWhileWriterHoldsEngineLock runs many readers while a commit
// is held at its Authority's Admit. Every reader opens a snapshot, reads it,
// and discards it while the writer is held, and reads the head published
// before the commit. A snapshot opened before the commit keeps
// reading that head's storage after the commit publishes a newer head.
func TestReadOpensWhileWriterHoldsEngineLock(t *testing.T) {
	// Open a testbed with an empty cursor.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()

	// Hold the commit at Admit on demand.
	var block atomic.Bool
	entered := make(chan struct{})
	releaseCommit := make(chan struct{})
	authority := &hookAuthority{
		Authority: newTestAuthority(t, le, base, "world:lockfree"),
		admit: func(context.Context, world.Candidate) error {
			if block.Load() {
				close(entered)
				<-releaseCommit
			}
			return nil
		},
	}

	// Open the engine on the gated authority.
	eng, err := NewEngine(ctx, le, base, nil, authority, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Define a commit of one new object.
	createObject := func(key string) error {
		// Create the object in a write transaction and commit it.
		wtx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer wtx.Discard()
		obj, err := wtx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return wtx.Commit(ctx)
	}

	// Commit a first object without blocking.
	if err := createObject("lockfree/before"); err != nil {
		t.Fatal(err)
	}
	seqno, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Hold a snapshot of the head before the blocked commit.
	held, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Discard()

	// Start a commit and hold it at its Authority's Admit.
	block.Store(true)
	writerDone := make(chan error, 1)
	go func() { writerDone <- createObject("lockfree/during") }()
	<-entered

	// Run many readers against the published head while the writer is held.
	const readers = 64
	var completed atomic.Int64
	readErrs := make(chan error, readers)
	var wg sync.WaitGroup
	for range readers {
		wg.Go(func() {
			if err := readPublishedHead(ctx, eng, seqno); err != nil {
				readErrs <- err
				return
			}
			completed.Add(1)
		})
	}
	readersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(readersDone)
	}()

	// Every reader must finish while Admit still holds the writer. The
	// deadline only bounds a failing run.
	select {
	case <-readersDone:
	case <-time.After(30 * time.Second):
		close(releaseCommit)
		<-writerDone
		t.Fatalf("%d of %d readers finished; the rest waited on the writer", completed.Load(), readers)
	}
	close(readErrs)
	for err := range readErrs {
		t.Error(err)
	}
	if got := completed.Load(); got != readers {
		t.Fatalf("%d of %d readers completed", got, readers)
	}
	select {
	case err := <-writerDone:
		t.Fatalf("the writer finished before its Admit was released: %v", err)
	default:
	}

	// Release the writer and wait for its commit.
	close(releaseCommit)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}

	// The held snapshot still reads the storage of the head it pinned.
	if found, err := hasObject(ctx, held, "lockfree/during"); err != nil || found {
		t.Fatalf("held snapshot saw the later object: found=%v err=%v", found, err)
	}
	if found, err := hasObject(ctx, held, "lockfree/before"); err != nil || !found {
		t.Fatalf("held snapshot lost its object: found=%v err=%v", found, err)
	}

	// A read opened after the commit sees the new head.
	after, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Discard()
	if found, err := hasObject(ctx, after, "lockfree/during"); err != nil || !found {
		t.Fatalf("read after commit missed the committed object: found=%v err=%v", found, err)
	}
	if got, err := eng.GetSeqno(ctx); err != nil || got <= seqno {
		t.Fatalf("seqno after commit is %d, want above %d: %v", got, seqno, err)
	}
}

// readPublishedHead opens a read snapshot, checks that it reads the head at
// seqno, and discards it.
func readPublishedHead(ctx context.Context, eng *Engine, seqno uint64) error {
	// Open the snapshot and read the revision through the Engine and the snapshot.
	rtx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer rtx.Discard()
	if got, err := eng.GetSeqno(ctx); err != nil || got != seqno {
		return errors.Errorf("engine seqno %d, want %d: %v", got, seqno, err)
	}
	if got, err := rtx.GetSeqno(ctx); err != nil || got != seqno {
		return errors.Errorf("snapshot seqno %d, want %d: %v", got, seqno, err)
	}

	// Read the committed object and miss the one still committing.
	if found, err := hasObject(ctx, rtx, "lockfree/before"); err != nil || !found {
		return errors.Errorf("snapshot missed the committed object: found=%v err=%v", found, err)
	}
	if found, err := hasObject(ctx, rtx, "lockfree/during"); err != nil || found {
		return errors.Errorf("snapshot saw the object still committing: found=%v err=%v", found, err)
	}
	return nil
}

// hasObject reports whether ws has an object at key through GetObject,
// releasing the object it reads.
func hasObject(ctx context.Context, ws world.WorldState, key string) (bool, error) {
	obj, found, err := ws.GetObject(ctx, key)
	world.ReleaseObjectState(obj)
	return found, err
}
