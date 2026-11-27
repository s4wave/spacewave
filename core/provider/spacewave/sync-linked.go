package provider_spacewave

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/provider/spacewave/synctelemetry"
	"github.com/s4wave/spacewave/db/block"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// linkedQueuePrefix prefixes the upload queue of linked chunks in the store of
// the linked file index: each key is the prefix and the encoded ref of a chunk
// a path gained, with an empty value, so a ref is queued at most once and the
// write that drops its last range deletes its entry by key.
const linkedQueuePrefix = "u/"

// linkedRefPage bounds the refs one reconcile step reads from the index.
const linkedRefPage = 1024

// errLinkedStale reports queued chunks that no file serves now while the index
// still references their files. The folder links a changed file again,
// dropping the old ranges and their entries, so a later pass finds them gone.
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
//     holds its chunk. The transaction that drops a ref's last range deletes
//     its entry, so a superseded chunk is never read or pushed. The
//     reconcile, at start and after a pull that trashes packs, queues again
//     every indexed ref no current pack holds. That covers a crash between
//     the index write and the World write, after which an unchanged file is
//     never linked again, and a lane pack that a reclaim pass trashed before
//     a published tree reached its chunks.
//  2. The lane deletes only its own queue entries, and only once a current
//     pack holds the chunk or the index references no file for the ref.
//  3. pending counts the queue's keys: each write counts only the entries it
//     created or deleted.
//  4. A checkpoint does not wait for the lane: it may reference a linked chunk
//     the cloud does not hold yet, which the file serves meanwhile.
//  5. The lane never takes flushMtx and pushes no pack while a flush runs,
//     so a checkpoint never waits behind a large linked folder.
type linkedLane struct {
	// s is the sync controller whose packs and manifest the lane extends.
	s *syncController
	// store holds the index and the queue.
	store kvtx.Store
	// files serves the linked chunks from their files.
	files *block_store_filesegment.Store

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

	// Count the queue.
	err = tx.ScanPrefixKeys(ctx, []byte(linkedQueuePrefix), func([]byte) error {
		l.pending++
		return nil
	})
	if err != nil {
		return nil, err
	}
	l.reportPending()

	// Serve the files through an index that queues what paths gain and
	// dequeues what no path holds.
	l.files = block_store_filesegment.NewStore(block_store_filesegment.NewIndex(store, l.enqueue))
	return l, nil
}

// wrapPrimary places the linked files behind primary without copying file hits into its cache.
func (l *linkedLane) wrapPrimary(primary block.StoreOps) block.StoreOps {
	return l.files.Behind(primary)
}

// enqueue writes a queue entry for each gained ref and deletes the entry of
// each lost ref in the index's write, and counts the entries it created and
// deleted once the write commits. It is the index's RefsHook. Each ref costs
// one point read.
func (l *linkedLane) enqueue(ctx context.Context, tx kvtx.Tx, gained, lost []*block.BlockRef) (func(), error) {
	// Create the missing entries of the gained refs.
	var n int
	for _, ref := range gained {
		key, err := linkedQueueKey(ref)
		if err != nil {
			return nil, err
		}
		queued, err := tx.Exists(ctx, key)
		if err != nil || queued {
			if err != nil {
				return nil, err
			}
			continue
		}
		if err := tx.Set(ctx, key, nil); err != nil {
			return nil, err
		}
		n++
	}

	// Delete the entries of the lost refs.
	for _, ref := range lost {
		key, err := linkedQueueKey(ref)
		if err != nil {
			return nil, err
		}
		deleted, err := deleteQueued(ctx, tx, key)
		if err != nil {
			return nil, err
		}
		if deleted {
			n--
		}
	}

	// Wake the lane after the commit.
	return func() { l.addPending(n) }, nil
}

// deleteQueued deletes the queue entry at key and reports whether it existed.
func deleteQueued(ctx context.Context, tx kvtx.Tx, key []byte) (bool, error) {
	queued, err := tx.Exists(ctx, key)
	if err != nil || !queued {
		return false, err
	}
	return true, tx.Delete(ctx, key)
}

// addPending adds n to the queue count, shows it in the sync status, and
// wakes the lane.
func (l *linkedLane) addPending(n int) {
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.pending += n
		l.reportPending()
		broadcast()
	})
}

// reportPending shows the queue count in the store's sync status, so a held
// queue of linked chunks reads as pending uploads. Call under bcast.
func (l *linkedLane) reportPending() {
	l.s.telemetrySafeCall(func(t *synctelemetry.Store, id string) {
		t.SetLinkedPending(id, l.pending)
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
// access or limits holds the lane until the account state changes. Another
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
		if errors.Is(err, ErrSigningUnavailable) || clouderror.IsAccessGated(err) || clouderror.IsLimitReached(err) {
			bo.Reset()
			if errors.Is(err, ErrSigningUnavailable) {
				l.s.le.WithError(err).Warn("no live session signer, waiting for account state change")
			} else {
				l.s.le.WithError(err).Warn("linked chunk upload held, waiting for account state change")
			}
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

// linkedPack contains the chunks of the next pack.
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
// no longer references. It returns errLinkedStale when it skipped chunks whose
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
				// A reconcile can queue a ref just as the
				// ref's last range drops. The index then
				// references no file and the entry goes.
				indexed, err := l.files.HasIndexedBlock(ctx, e.ref)
				if err != nil {
					return err
				}
				if !indexed {
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

	// Decode each entry's ref from its key.
	var page []linkedQueueEntry
	for ; ok && len(page) < syncDirtyPageLimit; ok = it.Next() {
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(it.Key()[len(linkedQueuePrefix):]); err != nil {
			return nil, errors.Wrap(err, "decoding queued linked chunk")
		}
		page = append(page, linkedQueueEntry{key: slices.Clone(it.Key()), ref: ref})
	}
	return page, it.Err()
}

// dequeue deletes the queue entries at keys in one write, counting only the
// entries that still existed: the index's write may have deleted some.
func (l *linkedLane) dequeue(ctx context.Context, keys [][]byte) error {
	// Skip an empty write.
	if len(keys) == 0 {
		return nil
	}

	// Delete the entries that remain.
	var n int
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) { return l.store.NewTransaction(ctx, true) },
		func(ctx context.Context, tx kvtx.Tx) error {
			n = 0
			for _, key := range keys {
				deleted, err := deleteQueued(ctx, tx, key)
				if err != nil {
					return err
				}
				if deleted {
					n++
				}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	l.addPending(-n)
	return nil
}

// queueUnheld queues every indexed ref no current pack holds, a page of refs
// at a time, each page in one write.
func (l *linkedLane) queueUnheld(ctx context.Context) error {
	view := l.s.lower.SnapshotManifest().WithoutTrash()
	var after *block.BlockRef
	for {
		// Read the next page of indexed refs and probe the catalog.
		refs, err := l.files.IndexedRefs(ctx, after, linkedRefPage)
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
				counted, err = l.enqueue(ctx, tx, missing, nil)
				return err
			},
		)
		if err != nil {
			return err
		}
		counted()
	}
}

// linkedQueueKey returns the queue key of ref.
func linkedQueueKey(ref *block.BlockRef) ([]byte, error) {
	data, err := ref.MarshalVT()
	if err != nil {
		return nil, err
	}
	return append([]byte(linkedQueuePrefix), data...), nil
}
