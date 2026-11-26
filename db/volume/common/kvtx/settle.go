package kvtx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// hideAbsent clears exists for each of refs that reads absent until a put
// stores it again, as block_gc.Records.Hidden describes, so a writer that
// checks before it puts puts the block again, which gives birth to it again.
// It reads nothing once the Volume is Settled, or when frees are off and it
// opened Open with no parked block or reopened since, as Volume.hidesNone
// describes; otherwise
// one snapshot reads the state and, per block found, what Hidden reads.
func (v *Volume) hideAbsent(ctx context.Context, refs []*block.BlockRef, exists []bool) error {
	// A Volume without liveness, a Settled one, or one that cannot hide a
	// block until it opens again has none.
	if !v.atomic || settledWord(v.settled.load()) || v.hidesNone.Load() {
		return nil
	}

	// Read the generation, then whether each block found is hidden.
	return v.readRecords(ctx, func(rec *block_gc.Records) error {
		if _, err := rec.GetState(ctx); err != nil || rec.Settled() {
			return err
		}
		for i, ref := range refs {
			if !exists[i] {
				continue
			}
			hidden, err := rec.Hidden(ctx, ref)
			if err != nil {
				return err
			}
			exists[i] = !hidden
		}
		return nil
	})
}

// settledFlag caches that a Volume's generation is Settled. Its word holds
// the flag in bit 0 and an epoch above it that each clear advances. A
// transaction loads the word when it opens and, once it commits having seen
// the Volume Settled, sets the flag only if the word is unchanged: a reopen
// that cleared the flag in between advanced the epoch, so a commit that saw
// the old Settled cannot restore it. A reopen clears the flag before its
// commit, and a transaction opens its snapshot before it loads the flag, so
// one that loads the flag set reads a snapshot from before the reopen. A
// write that takes the store's writer opens after the reopen commits, so it
// reads the generation from the state. An optimistic fill may open before
// the reopen and commit after it: one that lands goes through Land, which
// reads the state record and so conflicts with the reopen and runs again,
// and one that only refills writes the bytes of listed blocks and no record,
// which is right at any generation, and its late set fails on the advanced
// epoch.
type settledFlag struct {
	word atomic.Uint64
}

// load returns the flag's word.
func (f *settledFlag) load() uint64 {
	return f.word.Load()
}

// set sets the flag if its word is still word.
func (f *settledFlag) set(word uint64) {
	f.word.CompareAndSwap(word&^1, word|1)
}

// clear clears the flag and advances its epoch.
func (f *settledFlag) clear() {
	for {
		word := f.word.Load()
		if f.word.CompareAndSwap(word, (word|1)+1) {
			return
		}
	}
}

// settledWord reports whether word, a settledFlag's, has the flag set.
func settledWord(word uint64) bool {
	return word&1 != 0
}

// retrySwept runs attempt, a write that lands a batch, and runs it again
// each time the generation moves while it fails with block_gc.ErrSweeping: a
// copied put marked block.WithBackground waits for the sweep to settle, then
// reopens the Volume, as block_gc.Land describes. The sweep can run for
// minutes on a large Volume, so a foreground put gets the error at once and
// its caller retries; no user request waits on the sweep. The put waits only
// while a pass will come: with frees off, or with no caller keeping the
// collection running, as KeepCollecting describes, the sweep does not end, so
// the error returns. The wait channel is taken before each attempt, so a move
// of the generation or the last keeper's release between the attempt and the
// wait still wakes it.
func (v *Volume) retrySwept(ctx context.Context, attempt func() error) error {
	wait := block.IsBackground(ctx)
	for {
		var moved <-chan struct{}
		v.genBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			moved = getWaitCh()
		})
		err := attempt()
		if !wait || !errors.Is(err, block_gc.ErrSweeping) || v.collector == nil || !v.collector.Frees() || v.keepers.Load() == 0 {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-moved:
		}
	}
}

// KeepCollecting reports that the caller runs Collect passes until it calls
// release, so a background copied put waits for the sweep, as retrySwept
// describes. The release wakes the waiting writers, which return the error
// once no caller keeps collecting.
func (v *Volume) KeepCollecting() func() {
	v.keepers.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			v.keepers.Add(-1)
			v.broadcastGeneration()
		})
	}
}

// broadcastGeneration wakes the writers retrySwept holds.
func (v *Volume) broadcastGeneration() {
	v.genBcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		broadcastFn()
	})
}
