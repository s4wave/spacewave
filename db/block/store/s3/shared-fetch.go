//go:build !tinygo

package block_store_s3

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// fetchGroup shares one fetch among the concurrent callers of a key.
//
// The fetch runs under a context of its own, which ends only when the last
// waiting caller leaves, and in the most urgent read class of its callers,
// which rises as a more urgent caller joins. So the caller that started the
// fetch neither hands its deadline or cancel to the others nor holds a more
// urgent caller behind its own class. A caller that leaves gets its own
// context's cause; a caller that joins after every other left starts a new
// fetch.
type fetchGroup[V any] struct {
	// mtx guards m and the waiters of its fetches.
	mtx sync.Mutex
	// m holds the running fetches by key.
	m map[string]*sharedFetch[V]
}

// sharedFetch is one running fetch of a fetchGroup.
type sharedFetch[V any] struct {
	// done closes when val and err hold the result.
	done chan struct{}
	// val and err are the result.
	val V
	err error
	// waiters counts the callers waiting for the result.
	waiters int
	// cancel ends the fetch's context.
	cancel context.CancelFunc
	// demand is the read class of the fetch.
	demand *block.Demand
}

// do returns the result of fn for key, sharing one call of fn among the
// concurrent callers of key, or the cause of ctx if it ends first.
func (g *fetchGroup[V]) do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	// Join the running fetch, raising its class to the caller's, or start
	// one under a context that keeps the caller's values but not its end.
	class := block.ReadClassOf(ctx)
	g.mtx.Lock()
	f := g.m[key]
	if f != nil {
		f.demand.Raise(class)
	} else {
		fctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		f = &sharedFetch[V]{done: make(chan struct{}), cancel: cancel, demand: block.NewDemand(class)}
		if g.m == nil {
			g.m = make(map[string]*sharedFetch[V])
		}
		g.m[key] = f
		go g.run(block.WithDemand(fctx, f.demand), key, f, fn)
	}
	f.waiters++
	g.mtx.Unlock()

	// Wait for the result, or leave, ending the fetch if no caller is left.
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		g.mtx.Lock()
		if f.waiters--; f.waiters == 0 {
			f.cancel()
			if g.m[key] == f {
				delete(g.m, key)
			}
		}
		g.mtx.Unlock()
		var zero V
		return zero, context.Cause(ctx)
	}
}

// run calls fn and publishes its result to the fetch's waiters.
func (g *fetchGroup[V]) run(ctx context.Context, key string, f *sharedFetch[V], fn func(context.Context) (V, error)) {
	// Run the fetch, retire its key so a later caller starts afresh, and
	// wake the waiters.
	val, err := fn(ctx)
	g.mtx.Lock()
	if g.m[key] == f {
		delete(g.m, key)
	}
	g.mtx.Unlock()
	f.cancel()
	f.val, f.err = val, err
	close(f.done)
}
