package provider_spacewave

import (
	"context"
	"encoding/binary"
	"slices"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/db/block"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// linkedQueuePrefix prefixes the upload queue of linked chunks in the store of
// the linked file index: each key is the prefix and a big-endian sequence,
// and holds the encoded ref of a chunk a path gained.
const linkedQueuePrefix = "u/"

// linkedRefPage bounds the refs one reconcile step reads from the index.
const linkedRefPage = 1024

// errLinkedStale reports queued chunks that no file serves now while the index
// still names their files. The folder links a changed file again, which drops
// the old ranges, so a later pass drops or uploads them.
var errLinkedStale = errors.New("linked chunks wait for their changed files to link again")

// linkedLane uploads the chunks of linked files to the cloud. A linked chunk
// lives in its file, not in the upper cache, so the dirty queue never sees
// it: the lane queues each ref the index gains in the index's own write and
// pushes the chunks in packs of their own, beside the dirty lane.
//
// The lane sits below the Flowgraph: it is upkeep of the block store, which
// must work before any Flowgraph can load.
//
// Invariants:
//  1. A ref a path gained is queued in the same transaction that records the
//     range, so a recorded range has its queue entry until a current pack
//     holds its chunk. The reconcile, at start and after a pull that trashes
//     packs, queues again every indexed ref no current pack holds. That
//     covers a crash between the index write and the World write, after
//     which an unchanged file is never linked again, and a lane pack that a
//     reclaim pass trashed before a published tree reached its chunks.
//  2. The lane deletes only its own queue entries, and only once a current
//     pack holds the chunk or the index names no file for the ref.
//  3. A checkpoint does not wait for the lane: it may name a linked chunk the
//     cloud does not hold yet, which the file serves meanwhile.
//  4. The lane never takes flushMtx and pushes no pack while a flush runs,
//     so a checkpoint never waits behind a large linked folder.
type linkedLane struct {
	// s is the sync controller whose packs and manifest the lane extends.
	s *syncController
	// store holds the index and the queue.
	store kvtx.Store
	// files serves the linked chunks from their files.
	files *block_store_filesegment.Store
	// seq is the last queue sequence handed out.
	seq atomic.Uint64

	// bcast wakes the lane when the fields below change.
	bcast broadcast.Broadcast
	// pending counts the queue entries, under bcast.
	pending int
	// flushing counts the flushes running, under bcast.
	flushing int
	// reconcile requests a pass that queues the unheld indexed refs, under
	// bcast.
	reconcile bool
}

// newLinkedLane opens the lane over store, which holds the linked file index
// and the lane's queue, and returns it with its first pass due to reconcile.
// It reads every queue key once to count them.
func newLinkedLane(ctx context.Context, s *syncController, store kvtx.Store) (*linkedLane, error) {
	// Open a read of the queue.
	l := &linkedLane{s: s, store: store, reconcile: true}
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Count the queue and find its last sequence.
	it := tx.Iterate(ctx, []byte(linkedQueuePrefix), true, false)
	defer it.Close()
	var last uint64
	for it.Next() {
		l.pending++
		last = linkedQueueSeq(it.Key())
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	l.seq.Store(last)

	// Serve the files through an index that queues what paths gain.
	l.files = block_store_filesegment.NewStore(block_store_filesegment.NewIndex(store, l.enqueue))
	return l, nil
}

// enqueue writes a queue entry for each gained ref in the index's write, and
// counts them once it commits. It is the index's GainHook.
func (l *linkedLane) enqueue(ctx context.Context, tx kvtx.Tx, gained []*block.BlockRef) (func(), error) {
	// Write one entry per ref.
	for _, ref := range gained {
		data, err := ref.MarshalVT()
		if err != nil {
			return nil, err
		}
		if err := tx.Set(ctx, linkedQueueKey(l.seq.Add(1)), data); err != nil {
			return nil, err
		}
	}

	// Wake the lane after the commit.
	return func() { l.addPending(len(gained)) }, nil
}

// addPending adds n to the queue count and wakes the lane.
func (l *linkedLane) addPending(n int) {
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.pending += n
		broadcast()
	})
}

// startFlush marks a flush running until the returned release, so the lane
// pushes no pack meanwhile.
func (l *linkedLane) startFlush() func() {
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.flushing++
	})
	return func() {
		l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			l.flushing--
			broadcast()
		})
	}
}

// requestReconcile asks the next pass to queue the unheld indexed refs.
func (l *linkedLane) requestReconcile() {
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.reconcile = true
		broadcast()
	})
}

// Execute uploads queued chunks until ctx ends. A refusal for the account's
// access or limits holds the lane until the account state changes; another
// failure, or chunks waiting for their files, retries after a backoff or the
// next queue change.
func (l *linkedLane) Execute(ctx context.Context) {
	bo := providerBackoff.Construct()
	for {
		// Wait for work, then run one pass over the queue.
		if l.wait(ctx, true) != nil {
			return
		}
		err := l.pass(ctx)
		if err == nil {
			bo.Reset()
			continue
		}
		if ctx.Err() != nil {
			return
		}

		// Hold on the account's state, or back off.
		if clouderror.IsAccessGated(err) || clouderror.IsLimitReached(err) {
			bo.Reset()
			l.s.le.WithError(err).Warn("linked chunk upload held, waiting for account state change")
			if l.waitGate(ctx) != nil {
				return
			}
			continue
		}
		if !errors.Is(err, errLinkedStale) {
			l.s.le.WithError(err).Warn("linked chunk upload failed")
		}
		var changed <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) { changed = getWait() })
		if waitDirtySyncRetry(ctx, changed, nextProviderRetryDelay(bo, err)) != nil {
			return
		}
	}
}

// wait returns once no flush runs and, when work is set, the queue holds an
// entry or a reconcile is due.
func (l *linkedLane) wait(ctx context.Context, work bool) error {
	for {
		var ready bool
		var changed <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			ready = l.flushing == 0 && (!work || l.pending > 0 || l.reconcile)
			changed = getWait()
		})
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// waitGate waits for the account state that gates uploads to change, or for
// the queue to change when nothing reports account state.
func (l *linkedLane) waitGate(ctx context.Context) error {
	var changed <-chan struct{}
	if l.s.gateBcast != nil {
		l.s.gateBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) { changed = getWait() })
	} else {
		l.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) { changed = getWait() })
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	}
}

// linkedQueueEntry is one queue entry.
type linkedQueueEntry struct {
	// key is the entry's key.
	key []byte
	// ref is the queued chunk's ref.
	ref *block.BlockRef
}

// linkedPack gathers the chunks of the next pack.
type linkedPack struct {
	// blocks are the chunks, each once.
	blocks []packfile_store.PackBlock
	// size is the chunks' total size.
	size int64
	// has holds the chunks' block keys.
	has map[string]struct{}
	// keys are the queue entries the pack completes.
	keys [][]byte
}

// pass reconciles when due, then reads the queue in order and pushes its
// chunks in packs up to the sync pack target. It deletes the entries of each
// pushed pack, of chunks a current pack already holds, and of refs the index
// no longer names. It returns errLinkedStale when it skipped chunks whose
// files changed.
func (l *linkedLane) pass(ctx context.Context) error {
	// Queue the unheld indexed refs when asked.
	var reconcile bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		reconcile, l.reconcile = l.reconcile, false
	})
	if reconcile {
		if err := l.queueUnheld(ctx); err != nil {
			l.requestReconcile()
			return err
		}
	}

	// Read the queue a page at a time against the current packs.
	view := l.s.lower.SnapshotManifest().WithoutTrash()
	pack := &linkedPack{has: make(map[string]struct{})}
	var stale int
	var after []byte
	for {
		page, err := l.readQueue(ctx, after)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		after = page[len(page)-1].key
		refs := make([]*block.BlockRef, len(page))
		for i, e := range page {
			refs[i] = e.ref
		}
		held, err := view.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return errors.Wrap(err, "checking linked chunks against the catalog")
		}

		// Take each chunk a current pack lacks, pushing each full pack.
		var done [][]byte
		for i, e := range page {
			key := string(packfile.BlockKey(e.ref.GetHash()))
			if _, ok := pack.has[key]; ok || held[i] {
				if ok {
					pack.keys = append(pack.keys, e.key)
				} else {
					done = append(done, e.key)
				}
				continue
			}
			stored, err := l.files.GetStoredBlock(ctx, e.ref)
			if err != nil {
				return err
			}
			if stored == nil {
				segs, err := l.files.GetIndex().Lookup(ctx, e.ref)
				if err != nil {
					return err
				}
				if len(segs) == 0 {
					done = append(done, e.key)
				} else {
					stale++
				}
				continue
			}
			size := int64(len(stored.GetData()))
			if len(pack.blocks) != 0 && (pack.size+size > syncFlushMaxPackBytes || uint64(len(pack.blocks)) >= writer.DefaultMaxBlocksPerPack) {
				if err := l.push(ctx, pack); err != nil {
					return err
				}
				view = l.s.lower.SnapshotManifest().WithoutTrash()
				pack = &linkedPack{has: make(map[string]struct{})}
			}
			pack.blocks = append(pack.blocks, packfile_store.PackBlock{Hash: e.ref.GetHash(), Block: stored})
			pack.size += size
			pack.has[key] = struct{}{}
			pack.keys = append(pack.keys, e.key)
		}
		if err := l.dequeue(ctx, done); err != nil {
			return err
		}
	}

	// Push the last pack.
	if len(pack.blocks) != 0 {
		if err := l.push(ctx, pack); err != nil {
			return err
		}
	}
	if stale != 0 {
		return errors.Wrapf(errLinkedStale, "%d chunks", stale)
	}
	return nil
}

// push waits out any running flush, then pushes the pack, adds it to the
// manifest and deletes its queue entries.
func (l *linkedLane) push(ctx context.Context, pack *linkedPack) error {
	// Yield to the dirty lane.
	if err := l.wait(ctx, false); err != nil {
		return err
	}

	// Push and record the pack, as a rescue does.
	chunk, err := l.s.preparePackBlocks(pack.blocks, nil)
	if err != nil {
		return err
	}
	if err := l.s.pushPreparedChunk(ctx, chunk); err != nil {
		return err
	}
	if err := l.s.applyManifestDelta(ctx, []*packfile.PackfileEntry{chunk.entry}, nil, 0); err != nil {
		return errors.Wrap(err, "applying linked pack delta")
	}
	return l.dequeue(ctx, pack.keys)
}

// readQueue returns up to syncDirtyPageLimit queue entries after the key
// after, or from the first when after is nil.
func (l *linkedLane) readQueue(ctx context.Context, after []byte) ([]linkedQueueEntry, error) {
	// Position the iterator past after.
	tx, err := l.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, []byte(linkedQueuePrefix), true, false)
	defer it.Close()
	ok := false
	if after != nil {
		if err := it.Seek(after); err != nil {
			return nil, err
		}
		ok = it.Valid()
		if ok && slices.Equal(it.Key(), after) {
			ok = it.Next()
		}
	} else {
		ok = it.Next()
	}

	// Decode each entry's ref.
	var page []linkedQueueEntry
	for ; ok && len(page) < syncDirtyPageLimit; ok = it.Next() {
		data, err := it.Value()
		if err != nil {
			return nil, err
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(data); err != nil {
			return nil, errors.Wrap(err, "decoding queued linked chunk")
		}
		page = append(page, linkedQueueEntry{key: slices.Clone(it.Key()), ref: ref})
	}
	return page, it.Err()
}

// dequeue deletes the queue entries at keys in one write.
func (l *linkedLane) dequeue(ctx context.Context, keys [][]byte) error {
	// Skip an empty write.
	if len(keys) == 0 {
		return nil
	}
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) { return l.store.NewTransaction(ctx, true) },
		func(ctx context.Context, tx kvtx.Tx) error {
			for _, key := range keys {
				if err := tx.Delete(ctx, key); err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	l.addPending(-len(keys))
	return nil
}

// queueUnheld queues every indexed ref no current pack holds, a page of refs
// at a time, each page in one write.
func (l *linkedLane) queueUnheld(ctx context.Context) error {
	view := l.s.lower.SnapshotManifest().WithoutTrash()
	var after *block.BlockRef
	for {
		// Read the next page of indexed refs and probe the catalog.
		refs, err := l.files.GetIndex().Refs(ctx, after, linkedRefPage)
		if err != nil || len(refs) == 0 {
			return err
		}
		after = refs[len(refs)-1]
		held, err := view.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return errors.Wrap(err, "checking linked chunks against the catalog")
		}
		var missing []*block.BlockRef
		for i, ref := range refs {
			if !held[i] {
				missing = append(missing, ref)
			}
		}
		if len(missing) == 0 {
			continue
		}

		// Queue them.
		var counted func()
		err = kvtx.RunTransaction(ctx, true,
			func(ctx context.Context) (kvtx.Tx, error) { return l.store.NewTransaction(ctx, true) },
			func(ctx context.Context, tx kvtx.Tx) error {
				counted, err = l.enqueue(ctx, tx, missing)
				return err
			},
		)
		if err != nil {
			return err
		}
		counted()
	}
}

// linkedQueueKey returns the queue key of sequence seq.
func linkedQueueKey(seq uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte(linkedQueuePrefix), seq)
}

// linkedQueueSeq returns the sequence of the queue key key.
func linkedQueueSeq(key []byte) uint64 {
	if len(key) != len(linkedQueuePrefix)+8 {
		return 0
	}
	return binary.BigEndian.Uint64(key[len(linkedQueuePrefix):])
}
