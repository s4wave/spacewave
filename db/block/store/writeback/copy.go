package block_store_writeback

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// copyCursorPrefix holds the key after which a copy to each remote resumes:
// copy/<target>.
const copyCursorPrefix = "copy/"

// copyDonePrefix marks a copy to each remote complete: copy-done/<target>.
const copyDonePrefix = "copy-done/"

// copyBatchBlocks bounds the blocks one copy step lists, checks and writes.
const copyBatchBlocks = 1024

// copyWorkers bounds the copy steps in flight at once. Each step waits on the
// remote's round trips, so the copy's rate scales with them.
const copyWorkers = 16

// copySaveEvery is how often the copy saves its cursor and publishes its
// counts. A restart repeats at most the steps since the last save, which the
// remote ignores for the blocks it holds.
const copySaveEvery = 5 * time.Second

// CopyList lists up to limit blocks the local store holds after the key
// after, in key order, nil listing from the first. It returns the key to list
// the next blocks after, nil once the list ended. Each call reads in its own
// short snapshot, so the copy pins no freed space for its whole walk.
type CopyList func(ctx context.Context, after []byte, limit int) (refs []*block.BlockRef, next []byte, err error)

// CopyOptions adjusts a Copy.
type CopyOptions struct {
	// MaxBytesPerSecond bounds the block bytes the copy writes a second. Zero
	// sets no bound beyond the disk's load.
	MaxBytesPerSecond int64
	// Admit runs before each step when set and returns once the step may run,
	// so the caller can hold the copy off while its own writes are slow.
	Admit func(ctx context.Context) error
}

// copyStep is one listed batch of a copy, in list order.
type copyStep struct {
	// seq is the step's place in the list.
	seq int
	// refs are the listed blocks.
	refs []*block.BlockRef
	// next is the key the list resumes after once the step lands.
	next []byte
}

// copyLanded is a landed step waiting for the steps before it.
type copyLanded struct {
	// next is the key the list resumes after the step.
	next []byte
	// corrupt reports that the step left a corrupt block out.
	corrupt bool
}

// copyRun tracks the steps of a running copy, so its saved cursor only
// passes steps that landed with every step before them.
type copyRun struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// landed holds each landed step past the contiguous head.
	landed map[int]copyLanded
	// head is the seq of the first step not yet landed with all before it.
	head int
	// cursor is the resume key after the contiguous landed steps.
	cursor []byte
	// ended reports that the step ending the list landed with all before it.
	ended bool
	// held reports that a contiguous landed step left a corrupt block out.
	// The cursor stays before that step.
	held bool
	// counts are the step counts not yet published.
	counts CopyStatus
	// corrupt is the count of corrupt blocks the run left out.
	corrupt int64
}

// land records a landed step and its counts, advancing the cursor past every
// contiguous landed step up to the first that left a corrupt block out.
func (r *copyRun) land(st copyStep, counts CopyStatus) {
	// Record the step.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.counts.add(counts)
	r.corrupt += counts.Corrupt
	r.landed[st.seq] = copyLanded{next: st.next, corrupt: counts.Corrupt != 0}

	// Advance past the contiguous landed steps. Invariant: copy-done is
	// written only when every block before the cursor was copied, so the
	// cursor never passes the first step that left a corrupt block out, and
	// a save after a remount, crash or cancel resumes at that step.
	for {
		step, ok := r.landed[r.head]
		if !ok {
			return
		}
		delete(r.landed, r.head)
		r.head++
		r.held = r.held || step.corrupt
		if step.next == nil {
			r.ended = true
			return
		}
		if !r.held {
			r.cursor = step.next
		}
	}
}

// take returns the cursor, whether every listed block was copied, the counts
// since the last take, and the corrupt blocks the run left out.
func (r *copyRun) take() ([]byte, bool, CopyStatus, int64) {
	// Swap the counts out under the lock.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	counts := r.counts
	r.counts = CopyStatus{}
	return r.cursor, r.ended && !r.held, counts, r.corrupt
}

// Copy writes every block list returns to remote, resuming after the cursor a
// previous copy to target saved, until the list ends or ctx ends. It records
// the copy complete durably, so a later call returns at once.
//
// A copy writes nothing per block to the local store: it reads each block with
// its refs, writes the ones the remote lacks in batches, and confirms the
// remote holds them before its cursor passes them. Local reads share the
// upload's bound, which falls while the disk is loaded. Blocks the copy writes
// are not queued for eviction.
func (s *Store) Copy(ctx context.Context, target string, remote block.StoreOps, list CopyList, opts CopyOptions) error {
	// Read where a previous copy to target stopped.
	var cursor []byte
	var done bool
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// A completed copy needs no cursor.
		var err error
		done, err = tx.Exists(ctx, []byte(copyDonePrefix+target))
		if err != nil || done {
			return err
		}
		cursor, _, err = tx.Get(ctx, []byte(copyCursorPrefix+target))
		return err
	})
	if err != nil {
		return errors.Wrap(err, "read copy cursor")
	}
	s.setCopy(func(c *CopyStatus) {
		*c = CopyStatus{Target: target, Running: !done, Done: done, Position: copyPosition(cursor, done)}
	})
	if done {
		return nil
	}
	defer s.setCopy(func(c *CopyStatus) { c.Running = false })

	// Bound the bytes the copy writes a second.
	var limit *rate.Limiter
	if opts.MaxBytesPerSecond > 0 {
		burst := int(min(opts.MaxBytesPerSecond, math.MaxInt32))
		limit = rate.NewLimiter(rate.Limit(opts.MaxBytesPerSecond), max(burst, uploadBatchBytes))
	}

	// List the steps in order and copy them on the workers. A failed list
	// stops listing but lets the listed steps land, so their progress saves.
	run := &copyRun{landed: make(map[int]copyLanded), cursor: cursor}
	steps := make(chan copyStep)
	var listErr error
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer close(steps)
		after := cursor
		for seq := 0; ; seq++ {
			refs, next, err := list(egCtx, after, copyBatchBlocks)
			if err != nil {
				listErr = errors.Wrap(err, "list blocks to copy")
				return nil
			}
			select {
			case steps <- copyStep{seq: seq, refs: refs, next: next}:
			case <-egCtx.Done():
				return context.Cause(egCtx)
			}
			if next == nil {
				return nil
			}
			after = next
		}
	})

	// Copy the steps on the workers. workersBcast guards liveWorkers, the
	// workers still running, and wakes the saver as each one exits.
	var workersBcast broadcast.Broadcast
	liveWorkers := copyWorkers
	for range copyWorkers {
		eg.Go(func() error {
			defer func() {
				l := workersBcast.Lock()
				liveWorkers--
				l.Broadcast()
				l.Unlock()
			}()
			for st := range steps {
				if opts.Admit != nil {
					if err := opts.Admit(egCtx); err != nil {
						return err
					}
				}
				counts, err := s.copyStep(egCtx, remote, st.refs, limit)
				if err != nil {
					return err
				}
				run.land(st, counts)
			}
			return nil
		})
	}

	// Save the cursor every few seconds until the workers finish.
	eg.Go(func() error {
		//gostyle:timer deadline
		ticker := time.NewTicker(copySaveEvery)
		defer ticker.Stop()
		for {
			// Stop once the last worker exited, or wait for the next exit.
			l := workersBcast.Lock()
			stopped := liveWorkers == 0
			var exited <-chan struct{}
			if !stopped {
				exited = l.WaitCh()
			}
			l.Unlock()
			if stopped {
				return nil
			}

			// Save on each tick until a worker exits or the copy stops.
			select {
			case <-ticker.C:
				if err := s.saveCopy(egCtx, target, run, false); err != nil {
					return err
				}
			case <-exited:
			case <-egCtx.Done():
				return nil
			}
		}
	})
	err = eg.Wait()
	if err == nil {
		err = listErr
	}
	if err != nil {
		// Keep the progress of the landed steps for the next call.
		_ = s.saveCopy(context.WithoutCancel(ctx), target, run, false)
		return err
	}

	// Record the copy complete once the last step landed, unless it left
	// corrupt blocks out.
	if err := s.saveCopy(ctx, target, run, true); err != nil {
		return err
	}
	if _, _, _, corrupt := run.take(); corrupt != 0 {
		return errors.Wrapf(block.ErrBlockCorrupt, "%d blocks fail their hash and were not copied", corrupt)
	}
	return nil
}

// copyStep copies one listed batch: it asks the remote which blocks it holds,
// reads the rest with their refs, writes them, and confirms the remote holds
// them. A nil limit bounds no rate.
func (s *Store) copyStep(ctx context.Context, remote block.StoreOps, refs []*block.BlockRef, limit *rate.Limiter) (CopyStatus, error) {
	// Skip the blocks the remote holds.
	counts := CopyStatus{Scanned: int64(len(refs))}
	if len(refs) == 0 {
		return counts, nil
	}
	held, err := remote.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return counts, errors.Wrap(err, "check remote blocks")
	}
	var missing []*block.BlockRef
	for i, ref := range refs {
		if held[i] {
			counts.Present++
			continue
		}
		missing = append(missing, ref)
	}
	if len(missing) == 0 {
		return counts, nil
	}

	// Read the missing blocks with their refs, as many at once as the disk's
	// load admits.
	read := make([]*block.PutBatchEntry, len(missing))
	var mtx sync.Mutex
	err = s.reads.readEach(ctx, len(missing), func(ctx context.Context, i int) error {
		// Read the block with its refs.
		ref := missing[i]
		stored, err := s.local.GetStoredBlock(ctx, ref)
		if err != nil && !errors.Is(err, block.ErrBlockCorrupt) {
			return errors.Wrap(err, "read block to copy")
		}

		// Count it, or keep it to write. A corrupt block is left out, so one
		// bad block does not stop the copy of the rest.
		mtx.Lock()
		defer mtx.Unlock()
		switch {
		case err != nil:
			counts.Corrupt++
		case stored == nil:
			counts.Gone++
		case !stored.RefsSource.Recorded():
			counts.RefsUnknown++
		default:
			// The remote never marks a list and reads its refs back as
			// listed, so recorded refs go in known.
			read[i] = &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true}
		}
		return nil
	})
	if err != nil {
		return counts, err
	}
	entries := slices.DeleteFunc(read, func(entry *block.PutBatchEntry) bool { return entry == nil })

	// Write the blocks in chunks within the upload's byte limit, each after the
	// rate bound admits it.
	for start := 0; start < len(entries); {
		end, chunkBytes := start, 0
		for end < len(entries) && (end == start || chunkBytes+len(entries[end].Data) <= uploadBatchBytes) {
			chunkBytes += len(entries[end].Data)
			end++
		}
		if limit != nil {
			if err := limit.WaitN(ctx, min(chunkBytes, limit.Burst())); err != nil {
				return counts, err
			}
		}
		if _, err := remote.PutBlockBatch(ctx, entries[start:end]); err != nil {
			return counts, errors.Wrap(err, "copy blocks")
		}
		counts.Copied += int64(end - start)
		counts.CopiedBytes += int64(chunkBytes)
		start = end
	}

	// Confirm the remote holds every block written.
	written := make([]*block.BlockRef, len(entries))
	for i, entry := range entries {
		written[i] = entry.Ref
	}
	if len(written) == 0 {
		return counts, nil
	}
	held, err = remote.GetBlockExistsBatch(ctx, written)
	if err != nil {
		return counts, errors.Wrap(err, "confirm copied blocks")
	}
	for i, ok := range held {
		if !ok {
			return counts, errors.Errorf("remote does not hold copied block %s", written[i].MarshalString())
		}
	}
	return counts, nil
}

// saveCopy saves the copy's cursor, or marks it complete when complete is set
// and every listed block was copied, then publishes its counts and position.
// A run that left corrupt blocks out is never complete: its cursor stays
// before the first of them, so the next call reads them again.
func (s *Store) saveCopy(ctx context.Context, target string, run *copyRun, complete bool) error {
	// Save the cursor, or the completion once every listed block was copied.
	cursor, copied, counts, _ := run.take()
	done := complete && copied
	err := kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		if done {
			if err := tx.Delete(ctx, []byte(copyCursorPrefix+target)); err != nil {
				return err
			}
			return tx.Set(ctx, []byte(copyDonePrefix+target), nil)
		}
		if cursor == nil {
			return nil
		}
		return tx.Set(ctx, []byte(copyCursorPrefix+target), cursor)
	})
	if err != nil {
		return errors.Wrap(err, "save copy cursor")
	}

	// Publish the progress.
	s.setCopy(func(c *CopyStatus) {
		c.add(counts)
		c.Position = copyPosition(cursor, done)
		c.Done = done
	})
	return nil
}

// setCopy applies fn to the copy status and wakes the status watchers.
func (s *Store) setCopy(fn func(*CopyStatus)) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Apply and wake.
		fn(&s.status.Copy)
		broadcast()
	})
}

// copyPosition returns the share of the key space before cursor. A block key
// ends in its hash, whose first bytes place it in the key space.
func copyPosition(cursor []byte, done bool) float64 {
	// A completed copy covers the whole space.
	if done {
		return 1
	}

	// Place the cursor by its hash's leading bytes.
	ref := &block.BlockRef{}
	if len(cursor) == 0 || ref.UnmarshalVT(bytes.Clone(cursor)) != nil {
		return 0
	}
	var lead [8]byte
	copy(lead[:], ref.GetHash().GetHash())
	return float64(binary.BigEndian.Uint64(lead[:])) / math.Exp2(64)
}
