package block_store_writeback

import (
	"bytes"
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// syncGate holds the first pack sync a Log runs until the test answers it,
// and lets every later one through.
type syncGate struct {
	once    sync.Once
	entered chan struct{}
	answer  chan error
}

// newSyncGate returns a gate whose first sync waits for an answer.
func newSyncGate() *syncGate {
	return &syncGate{entered: make(chan struct{}), answer: make(chan error, 1)}
}

// sync is a packahead SyncData that waits out the first sync.
func (g *syncGate) sync(f *os.File) error {
	first := false
	g.once.Do(func() { first = true })
	if !first {
		return f.Sync()
	}
	close(g.entered)
	return <-g.answer
}

// putResult is one put's outcome.
type putResult struct {
	err error
}

// TestPackedStoreHeldWaitsDurable stores one block twice at once through a
// diverting PackedStore while the first put's sync is held: the second put
// finds the value in the Log's table before it is durable, and must not
// return before the sync does, or a crash then loses a block it
// acknowledged. When the held sync fails, the second put fails too, or else
// still reads its block.
func TestPackedStoreHeldWaitsDurable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer error
	}{
		{name: "synced", answer: nil},
		{name: "failed", answer: syscall.EIO},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			env := newTestEnv(t)
			gate := newSyncGate()
			log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{SyncData: gate.sync})
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			s := NewPackedStore(log, ownedStore{env.vol}, true)

			data := []byte("held until durable")
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			put := func() <-chan putResult {
				done := make(chan putResult, 1)
				go func() {
					_, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}})
					done <- putResult{err: err}
				}()
				return done
			}

			// The first put writes its batch and holds in its sync.
			first := put()
			select {
			case <-gate.entered:
			case res := <-first:
				t.Fatalf("first put returned before its sync: %v", res.err)
			case <-time.After(10 * time.Second):
				t.Fatal("first put never synced")
			}

			// The second put finds the value written and must wait for it.
			second := put()
			select {
			case res := <-second:
				t.Fatalf("second put acknowledged before the value was durable: %v", res.err)
			case <-time.After(300 * time.Millisecond):
			}

			// Answer the sync; both puts end.
			gate.answer <- tc.answer
			results := make([]putResult, 2)
			for i, ch := range []<-chan putResult{first, second} {
				select {
				case results[i] = <-ch:
				case <-time.After(10 * time.Second):
					t.Fatalf("put %d never returned", i)
				}
			}
			if tc.answer == nil {
				for i, res := range results {
					if res.err != nil {
						t.Fatalf("put %d: %v", i, res.err)
					}
				}
			}
			if results[0].err == nil && tc.answer != nil {
				t.Fatal("first put succeeded on a failed sync")
			}

			// A put that succeeded reads its block.
			if results[1].err == nil {
				stored, err := s.GetStoredBlock(ctx, ref)
				if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
					t.Fatalf("acknowledged block does not read: %v, %v", stored, err)
				}
			}
		})
	}
}

// TestPackedStoreTombstoneKeepsPending tombstones a diverted block whose
// pack is not uploaded: a writer that skipped storing it may have landed it
// meanwhile, and no other copy serves it until the upload, so the Log keeps
// serving it.
func TestPackedStoreTombstoneKeepsPending(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	s := NewPackedStore(log, ownedStore{env.vol}, true)

	data := []byte("tombstoned while pending")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RmBlock(context.WithoutCancel(ctx), ref); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("pending block does not read after its tombstone: %v, %v", stored, err)
	}
}
