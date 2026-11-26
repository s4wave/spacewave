package provider_local

import (
	"context"

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

// raceResult is the answer of raceFallback.
type raceResult[T any] struct {
	// v and err are the answer: the primary read's, or the fallback's hit.
	v   T
	err error
	// fromFallback marks v as the fallback's hit.
	fromFallback bool
	// fallbackAsked reports that the fallback answered without a hit, with
	// fallbackErr, so a caller need not ask it again.
	fallbackAsked bool
	fallbackErr   error
}

// raceFallback runs primary, a read that reaches the backend, and returns its
// answer. A backend whose requests are paused makes primary wait without
// failing, so while primary waits and the backend is paused, or once it
// pauses, raceFallback runs fallback beside it, once. A fallback hit, as hit
// reports it, answers and cancels primary; a fallback miss or failure leaves
// primary to answer. With the backend never paused, only primary runs.
//
// primary runs in its own goroutine under a context that ends when
// raceFallback returns, so a primary that lost the race returns soon after.
func raceFallback[T any](
	ctx context.Context,
	pause backendPause,
	primary, fallback func(context.Context) (T, error),
	hit func(T) bool,
) raceResult[T] {
	// Without a watch the primary answers alone.
	if pause == nil {
		v, err := primary(ctx)
		return raceResult[T]{v: v, err: err}
	}

	// Run the primary read beside the watch.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type answer struct {
		v   T
		err error
	}
	primaryDone := make(chan answer, 1)
	go func() {
		v, err := primary(ctx)
		primaryDone <- answer{v, err}
	}()

	// Start the fallback once the backend is paused, and serve the first
	// answer that counts.
	var fallbackDone chan answer
	var res raceResult[T]
	for {
		var changed <-chan struct{}
		if fallbackDone == nil && !res.fallbackAsked {
			var paused bool
			paused, changed = pause()
			if paused {
				fallbackDone = make(chan answer, 1)
				go func() {
					v, err := fallback(ctx)
					fallbackDone <- answer{v, err}
				}()
				changed = nil
			}
		}
		select {
		case a := <-primaryDone:
			res.v, res.err = a.v, a.err
			return res
		case a := <-fallbackDone:
			if a.err == nil && hit(a.v) {
				return raceResult[T]{v: a.v, fromFallback: true}
			}
			fallbackDone = nil
			res.fallbackAsked, res.fallbackErr = true, a.err
		case <-changed:
		}
	}
}

// storedHit reports whether a read found the block.
func storedHit(stored *block.StoredBlock) bool {
	return stored != nil
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
