package block_store_writeback

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
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
			// Open a PackedStore whose first batch sync the test controls.
			ctx := t.Context()
			env := newTestEnv(t)
			gate := newSyncGate()
			log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{SyncData: gate.sync})
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			defer func() {
				select {
				case gate.answer <- syscall.EIO:
				default:
				}
			}()
			s := NewPackedStore(log, ownedStore{env.vol}, true)

			// Encode a cacheable block for two writes and independent readers.
			msg := "held until durable: " + tc.name
			data, err := (&block_mock.Example{Msg: msg}).MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			cache := block.NewDecodedBlockCache()
			defer cache.Close()
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

			// An unsynced read can decode the bytes but cannot share them.
			before, err := readPackedExample(ctx, cache, s, ref, msg)
			if err != nil || before.BlockReadCount != 1 || before.DecodedBlockStoreAcceptedCount != 0 {
				t.Fatalf("unsynced read = %+v, %v", before, err)
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

			// Check admission after the sync outcome in a fresh transaction.
			after, err := readPackedExample(ctx, cache, s, ref, msg)
			if tc.answer != nil {
				if !errors.Is(err, block.ErrNotFound) || after.DecodedBlockCacheHitCount != 0 {
					t.Fatalf("read after failed sync = %+v, %v", after, err)
				}
			} else {
				if err != nil || after.BlockReadCount != 1 || after.DecodedBlockStoreAcceptedCount != 1 {
					t.Fatalf("synced log read = %+v, %v", after, err)
				}
				again, err := readPackedExample(ctx, cache, s, ref, msg)
				if err != nil || again.BlockReadCount != 0 || again.DecodedBlockCacheHitCount != 1 {
					t.Fatalf("second synced read = %+v, %v", again, err)
				}
			}

			// The first put reports a failed sync to its caller.
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
	// Open a log whose first pack will finalize without another write.
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	s := NewPackedStore(log, ownedStore{env.vol}, true)

	// Write and cache a synced log block before its GC removal.
	const msg = "tombstoned while pending"
	data, err := (&block_mock.Example{Msg: msg}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	cache := block.NewDecodedBlockCache()
	defer cache.Close()
	if got, err := readPackedExample(ctx, cache, s, ref, msg); err != nil || got.DecodedBlockStoreAcceptedCount != 1 {
		t.Fatalf("initial log admission = %+v, %v", got, err)
	}

	// GC invalidates the first admission, while the pending pack still reads.
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

	// Finish a post-GC read after the upload drops its last readable copy.
	seq := waitPackedFinalized(t, log)
	reader := &packedReadBarrier{StoreOps: s, afterRead: func() { log.MarkUploaded(seq) }}
	got, err := readPackedExample(ctx, cache, reader, ref, msg)
	if err != nil || got.BlockReadCount != 1 || got.DecodedBlockStoreAcceptedCount != 0 {
		t.Fatalf("read racing the drop = %+v, %v", got, err)
	}

	// Another transaction must miss both the shared cache and the stores.
	got, err = readPackedExample(ctx, cache, s, ref, msg)
	if !errors.Is(err, block.ErrNotFound) || got.BlockReadMissCount != 1 || got.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("read after the drop = %+v, %v", got, err)
	}
}

// packedReadBarrier runs the upload between a new cursor's Get and admission.
type packedReadBarrier struct {
	block.StoreOps
	afterRead func()
}

// GetBlock reads the packed bytes before allowing the upload to drop them.
func (s *packedReadBarrier) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Let PackedStore capture the durable read first.
	data, found, err := s.StoreOps.GetBlock(ctx, ref)

	// Drop the location before the cursor can admit the block.
	s.afterRead()
	return data, found, err
}

// readPackedExample decodes ref in a fresh transaction, sharing only the pool.
func readPackedExample(ctx context.Context, cache *block.DecodedBlockCache, s block.StoreOps, ref *block.BlockRef, want string) (block.ReadCounterSnapshot, error) {
	// Give each read its own cursor and metrics.
	ctx, counter := block.WithReadCounter(ctx)
	tx, cursor := block.NewTransaction(s, nil, ref, nil)
	tx.SetDecodedBlockCache(cache)

	// Verify the bytes came back as the expected message.
	value, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	if err == nil && value.(*block_mock.Example).GetMsg() != want {
		return counter.Snapshot(), errors.New("unexpected decoded message")
	}
	return counter.Snapshot(), err
}

// waitPackedFinalized returns the sequence of the first finalized test pack.
func waitPackedFinalized(t *testing.T, log *packahead.Log) uint64 {
	// Watch the pack transition through finalization.
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		infos, changed := log.GetPacks()
		if len(infos) != 0 {
			return infos[0].Seq
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatal("pack never finalized")
		}
	}
}

// TestPackedStorePutAfterTombstone stores a diverted block, tombstones it
// while its pack is not uploaded, and stores it again: the second put finds
// no live copy, so it reports the block new, and the block stays readable
// after the pack that held it uploads.
func TestPackedStorePutAfterTombstone(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	s := NewPackedStore(log, ownedStore{env.vol}, true)

	data := []byte("stored again after its tombstone")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	put := []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}
	if _, err := s.PutBlockBatch(ctx, put); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err != nil {
		t.Fatal(err)
	}
	existed, err := s.PutBlockBatch(ctx, put)
	if err != nil {
		t.Fatal(err)
	}
	if existed[0] {
		t.Fatal("put after a tombstone reports the block existed")
	}

	// The uploaded pack retains the block because the second put revived it.
	log.MarkUploaded(waitPackedFinalized(t, log))
	if ok, err := s.GetBlockExists(ctx, ref); err != nil || !ok {
		t.Fatalf("block stored again does not exist after upload: %v, %v", ok, err)
	}
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("block stored again does not read after upload: %v, %v", stored, err)
	}
}
