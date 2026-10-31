package block_store_writeback

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"slices"
	"sync"
	"time"

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
	// Admit, when set, runs before each step and returns once the step may
	// run, so the caller can hold the copy off while its own writes are slow.
	Admit func(ctx context.Context) error
}

// CopyStatus reports a copy of the stored blocks to a remote.
type CopyStatus struct {
	// Target is the remote the copy writes to, empty before a copy starts.
	Target string
	// Running reports a copy in progress.
	Running bool
	// Done reports every block stored before the copy started is on the
	// remote.
	Done bool
	// Position is the share of the key space the copy has passed, from 0 to 1.
	// Block keys are hashes, so it tracks the share of blocks.
	Position float64
	// Scanned is the number of blocks the copy listed since it started.
	Scanned int64
	// Present is the number of listed blocks the remote already held.
	Present int64
	// Copied is the number of blocks the copy wrote to the remote.
	Copied int64
	// CopiedBytes is the size of the blocks the copy wrote.
	CopiedBytes int64
	// Gone is the number of listed blocks the local store no longer held when
	// the copy read them.
	Gone int64
	// RefsUnknown is the number of blocks the copy left out because the local
	// store does not know their refs. The remote would keep them as leaves, so
	// they stay local, and a copy with any is not complete for dropping the
	// local store.
	RefsUnknown int64
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

// copyRun tracks the steps of a running copy, so its saved cursor only
// passes steps that landed with every step before them.
type copyRun struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// landed holds the resume key of each landed step past the contiguous
	// head.
	landed map[int][]byte
	// head is the seq of the first step not yet landed with all before it.
	head int
	// cursor is the resume key after the contiguous landed steps.
	cursor []byte
	// ended reports that the step ending the list landed with all before it.
	ended bool
	// counts are the step counts not yet published.
	counts CopyStatus
}

// land records a landed step and its counts, advancing the cursor past every
// contiguous landed step.
func (r *copyRun) land(st copyStep, counts CopyStatus) {
	// Record the step.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.counts.add(counts)
	r.landed[st.seq] = st.next

	// Advance past the contiguous landed steps.
	for {
		next, ok := r.landed[r.head]
		if !ok {
			return
		}
		delete(r.landed, r.head)
		r.head++
		if next == nil {
			r.ended = true
			return
		}
		r.cursor = next
	}
}

// take returns the cursor, whether the list ended, and the counts since the
// last take.
func (r *copyRun) take() ([]byte, bool, CopyStatus) {
	// Swap the counts out under the lock.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	counts := r.counts
	r.counts = CopyStatus{}
	return r.cursor, r.ended, counts
}

// Copy writes every block list names to remote, resuming after the cursor a
// previous copy to target saved, until the list ends or ctx ends. It records
// the copy complete durably, so a later call returns at once.
//
// A copy writes nothing per block to the local store: it reads each block
// with its refs, writes the ones the remote lacks in batches, and confirms the
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
	run := &copyRun{landed: make(map[int][]byte), cursor: cursor}
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
	var workers sync.WaitGroup
	for range copyWorkers {
		workers.Add(1)
		eg.Go(func() error {
			defer workers.Done()
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
	stopped := make(chan struct{})
	go func() {
		workers.Wait()
		close(stopped)
	}()
	eg.Go(func() error {
		ticker := time.NewTicker(copySaveEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.saveCopy(egCtx, target, run, false); err != nil {
					return err
				}
			case <-stopped:
				return nil
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

	// Record the copy complete once the last step landed.
	return s.saveCopy(ctx, target, run, true)
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
	eg, egCtx := errgroup.WithContext(ctx)
	for i, ref := range missing {
		eg.Go(func() error {
			return s.reads.read(egCtx, func() error {
				// Read the block with its refs.
				stored, err := s.local.GetStoredBlock(egCtx, ref)
				if err != nil {
					return errors.Wrap(err, "read block to copy")
				}

				// Count it, or keep it to write.
				mtx.Lock()
				defer mtx.Unlock()
				switch {
				case stored == nil:
					counts.Gone++
				case !stored.RefsKnown:
					counts.RefsUnknown++
				default:
					read[i] = &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true}
				}
				return nil
			})
		})
	}
	if err := eg.Wait(); err != nil {
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
// and the list ended, then publishes its counts and position.
func (s *Store) saveCopy(ctx context.Context, target string, run *copyRun, complete bool) error {
	// Save the cursor, or the completion once the last step landed.
	cursor, ended, counts := run.take()
	done := complete && ended
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

// add adds the step counts of o to c.
func (c *CopyStatus) add(o CopyStatus) {
	// Sum each count.
	c.Scanned += o.Scanned
	c.Present += o.Present
	c.Copied += o.Copied
	c.CopiedBytes += o.CopiedBytes
	c.Gone += o.Gone
	c.RefsUnknown += o.RefsUnknown
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
