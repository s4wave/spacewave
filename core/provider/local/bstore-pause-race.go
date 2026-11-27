package provider_local

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// backendPause watches the backend's pause, as block_store.PauseWatcher
// reports it: whether the backend's requests are paused, and a channel closed
// at the next change, or nil when nothing can change.
type backendPause func() (paused bool, changed <-chan struct{})

// watchBackendPause returns the pause watch of the store remote returns at
// each call, so a backend that opens or changes is watched in its turn. A
// store that cannot pause, or no store, reports no pause.
func watchBackendPause(remote block_store.StoreSource) backendPause {
	if remote == nil {
		return nil
	}
	return func() (bool, <-chan struct{}) {
		w, ok := remote().(block_store.PauseWatcher)
		if !ok {
			return false, nil
		}
		return w.WatchPause()
	}
}

// racingKey marks the context of an alternate's read in raceAlternates.
type racingKey struct{}

// racingBackend reports whether ctx is an alternate's read that races a
// paused backend, which has not answered yet: a miss there is not yet a miss
// of every tier.
func racingBackend(ctx context.Context) bool {
	return ctx.Value(racingKey{}) != nil
}

// raceResult is the answer of raceAlternates.
type raceResult[T any] struct {
	// v and err are the answer: the primary read's, or an alternate's hit.
	v   T
	err error
	// fromAlternate marks v as an alternate's hit.
	fromAlternate bool
	// missed holds the answers of the alternates that answered without a
	// hit, in order, so a caller need not ask them again.
	missed []raceAnswer[T]
}

// raceAnswer is one read's answer in a race.
type raceAnswer[T any] struct {
	v   T
	err error
}

// raceAlternates runs primary, a read that reaches the backend, and returns its
// answer. A backend whose requests are paused makes primary wait without
// failing, so while primary waits and the backend is paused, or once it
// pauses, raceAlternates asks alternates beside it, in order, each once: an
// alternate starts only once the one before it answered without a hit, and
// only while the backend is paused. An alternate's hit, as hit reports it,
// answers and cancels primary. Once every alternate missed, primary answers.
// With the backend never paused, only primary runs.
//
// primary runs in its own goroutine under a context that ends when
// raceAlternates returns, so a primary that lost the race returns soon after.
// The alternates read under that context marked as racingBackend describes.
func raceAlternates[T any](
	ctx context.Context,
	pause backendPause,
	primary func(context.Context) (T, error),
	hit func(T) bool,
	alternates ...func(context.Context) (T, error),
) raceResult[T] {
	// Without a watch or an alternate the primary answers alone.
	if pause == nil || len(alternates) == 0 {
		v, err := primary(ctx)
		return raceResult[T]{v: v, err: err}
	}

	// Run the primary read beside the watch.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	primaryDone := make(chan raceAnswer[T], 1)
	go func() {
		v, err := primary(ctx)
		primaryDone <- raceAnswer[T]{v, err}
	}()

	// Ask the next alternate while the backend is paused, and serve the
	// first answer that counts.
	altCtx := context.WithValue(ctx, racingKey{}, true)
	var altDone chan raceAnswer[T]
	var res raceResult[T]
	for {
		var changed <-chan struct{}
		if next := len(res.missed); altDone == nil && next < len(alternates) {
			var paused bool
			paused, changed = pause()
			if paused {
				altDone = make(chan raceAnswer[T], 1)
				alt := alternates[next]
				go func() {
					v, err := alt(altCtx)
					altDone <- raceAnswer[T]{v, err}
				}()
				changed = nil
			}
		}
		select {
		case a := <-primaryDone:
			res.v, res.err = a.v, a.err
			return res
		case a := <-altDone:
			if a.err == nil && hit(a.v) {
				return raceResult[T]{v: a.v, fromAlternate: true, missed: res.missed}
			}
			altDone = nil
			res.missed = append(res.missed, a)
		case <-changed:
		}
	}
}

// pausedReads serves reads from the read chain it embeds and, while the
// backend is paused, from alternates in order, as raceAlternates describes.
// Each alternate is a whole source with its own fill, such as the read
// fallback or the peers, so a hit fills the local store as the chain would
// have. Existence checks pass to the chain: the read fallback races its own,
// and peers never answer them.
type pausedReads struct {
	// StoreOps is the read chain.
	block.StoreOps
	// pause watches the backend's pause.
	pause backendPause
	// alternates read a block from another source. A nil block is a miss.
	alternates []readAlternate
}

// readAlternate reads ref from another source than the backend, returning
// nil on a miss.
type readAlternate func(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error)

// newPausedReads wraps chain so its reads race alternates, leaving out the
// nil ones, while pause reports the backend paused. Without a watch or an
// alternate it returns chain.
func newPausedReads(chain block.StoreOps, pause backendPause, alternates ...readAlternate) block.StoreOps {
	alternates = slices.DeleteFunc(alternates, func(alt readAlternate) bool { return alt == nil })
	if pause == nil || len(alternates) == 0 {
		return chain
	}
	return &pausedReads{StoreOps: chain, pause: pause, alternates: alternates}
}

// readSource is a readAlternate of the store src returns, or a miss without
// one. A nil src is no alternate.
func readSource(src block_store.StoreSource) readAlternate {
	if src == nil {
		return nil
	}
	return func(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
		store := src()
		if store == nil {
			return nil, nil
		}
		return store.GetStoredBlock(ctx, ref)
	}
}

// BeginReadOperation scopes the chain. The alternates read unscoped.
func (r *pausedReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &pausedReads{StoreOps: scoped, pause: r.pause, alternates: r.alternates}, release, nil
}

// GetStoredBlock reads the chain, racing the alternates in a pause.
func (r *pausedReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return r.read(ctx, ref, func(ctx context.Context) (*block.StoredBlock, error) {
		return r.StoreOps.GetStoredBlock(ctx, ref)
	})
}

// GetBlock reads the chain, racing the alternates in a pause.
func (r *pausedReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := r.read(ctx, ref, func(ctx context.Context) (*block.StoredBlock, error) {
		data, found, err := r.StoreOps.GetBlock(ctx, ref)
		if err != nil || !found {
			return nil, err
		}
		return &block.StoredBlock{Data: data}, nil
	})
	return stored.GetData(), stored != nil, err
}

// StatBlock stats the block through the chain's metadata path, racing the
// alternates' reads in a pause.
func (r *pausedReads) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Stat each alternate's block from its read.
	alternates := make([]func(context.Context) (*block.BlockStat, error), len(r.alternates))
	for i, alt := range r.alternates {
		alternates[i] = func(ctx context.Context) (*block.BlockStat, error) {
			stored, err := alt(ctx, ref)
			if err != nil || stored == nil {
				return nil, err
			}
			return &block.BlockStat{Ref: ref, Size: int64(len(stored.Data))}, nil
		}
	}

	// Stat the chain, racing the alternates in a pause.
	res := raceAlternates(ctx, r.pause, func(ctx context.Context) (*block.BlockStat, error) {
		return r.StoreOps.StatBlock(ctx, ref)
	}, statHit, alternates...)
	return res.v, res.err
}

// read runs primary, racing the alternates while the backend is paused.
func (r *pausedReads) read(
	ctx context.Context,
	ref *block.BlockRef,
	primary func(context.Context) (*block.StoredBlock, error),
) (*block.StoredBlock, error) {
	// Bind each alternate to ref.
	alternates := make([]func(context.Context) (*block.StoredBlock, error), len(r.alternates))
	for i, alt := range r.alternates {
		alternates[i] = func(ctx context.Context) (*block.StoredBlock, error) { return alt(ctx, ref) }
	}
	res := raceAlternates(ctx, r.pause, primary, storedHit, alternates...)
	return res.v, res.err
}

// storedHit reports whether a read found the block.
func storedHit(stored *block.StoredBlock) bool {
	return stored != nil
}

// statHit reports whether a stat found the block.
func statHit(stat *block.BlockStat) bool {
	return stat != nil
}

// existsHit reports whether a check confirmed the block.
func existsHit(found bool) bool {
	return found
}

// allExistHit reports whether a batch check confirmed every block.
func allExistHit(found []bool) bool {
	for _, ok := range found {
		if !ok {
			return false
		}
	}
	return found != nil
}

// _ is a type assertion
var _ block.StoreOps = (*pausedReads)(nil)
