//go:build !tinygo

package block_store_s3

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// errFetchAbandoned settles a fetch whose batch left before reading it, so
// each caller that joined it starts a fetch of its own.
var errFetchAbandoned = errors.New("shared fetch abandoned by its batch")

// fetchGroup shares one fetch among the concurrent callers of a key.
//
// The fetch runs under a context of its own, which ends only when the last
// waiting caller leaves, and in the most urgent read class of its callers,
// which rises as a more urgent caller joins. So the caller that started the
// fetch neither hands its deadline or cancel to the others nor holds a more
// urgent caller behind its own class. A caller that leaves gets its own
// context's cause; a caller that joins after every other left starts a new
// fetch.
//
// A batch claims many keys at once, as claim describes: it joins the running
// fetches and owns the others, which it settles as its reads land.
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
	// owned marks a fetch a batch claimed: the batch settles it, and its
	// joiners leaving neither cancel nor retire it.
	owned bool
}

// do returns the result of fn for key, sharing one call of fn among the
// concurrent callers of key, or the cause of ctx if it ends first. A fetch its
// batch abandoned is no answer, so do starts a fetch of its own.
func (g *fetchGroup[V]) do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	for {
		val, err := g.doOnce(ctx, key, fn)
		if !errors.Is(err, errFetchAbandoned) {
			return val, err
		}
	}
}

// doOnce joins the running fetch of key or starts one, and waits for it.
func (g *fetchGroup[V]) doOnce(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	// Join the running fetch, raising its class to the caller's, or start
	// one under a context that keeps the caller's values but not its end.
	class := block.ReadClassOf(ctx)
	g.mtx.Lock()
	f := g.m[key]
	if f != nil {
		f.demand.Raise(class)
	} else {
		fctx, cancel := context.WithCancel(context.WithoutCancel(ctx)) //nolint:gosec // the fetch keeps cancel: run calls it at the end, leave when its last caller leaves.
		f = &sharedFetch[V]{done: make(chan struct{}), cancel: cancel, demand: block.NewDemand(class)}
		if g.m == nil {
			g.m = make(map[string]*sharedFetch[V])
		}
		g.m[key] = f
		go g.run(block.WithDemand(fctx, f.demand), key, f, fn)
	}
	f.waiters++
	g.mtx.Unlock()
	return g.wait(ctx, key, f)
}

// wait waits for the result of f, the fetch of key the caller joined, or
// leaves, ending a fetch no batch owns once no caller is left.
func (g *fetchGroup[V]) wait(ctx context.Context, key string, f *sharedFetch[V]) (V, error) {
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		g.leave(key, f)
		var zero V
		return zero, context.Cause(ctx)
	}
}

// leave drops a waiter of f, the fetch of key, ending and retiring a fetch no
// batch owns once no caller is left.
func (g *fetchGroup[V]) leave(key string, f *sharedFetch[V]) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	if f.waiters--; f.waiters == 0 && !f.owned {
		f.cancel()
		if g.m[key] == f {
			delete(g.m, key)
		}
	}
}

// fetchClaim is a batch's claim on the keys of a fetchGroup: for each key,
// the running fetch it joined or the fetch it owns.
type fetchClaim[V any] struct {
	// g is the group.
	g *fetchGroup[V]
	// keys are the claimed keys.
	keys []string
	// fetches are the fetches of keys.
	fetches []*sharedFetch[V]
	// owned marks the fetches the batch owns and must settle.
	owned []bool
	// open marks the fetches neither settled nor waited for.
	open []bool
}

// claim joins the running fetch of each key, raising its class to the
// caller's, and starts a fetch the caller owns for each other key. The caller
// settles each owned fetch with settle, waits for each joined one with wait,
// and calls release when done, which abandons what it left open. A batch's
// owned fetches share one read class, which a joiner raises, so the caller
// reads them under the context claim returns. A key named twice is owned
// once and joined after.
func (g *fetchGroup[V]) claim(ctx context.Context, keys []string) (*fetchClaim[V], context.Context) {
	// One demand carries the class of every fetch the batch owns.
	class := block.ReadClassOf(ctx)
	demand := block.NewDemand(class)
	c := &fetchClaim[V]{
		g:       g,
		keys:    keys,
		fetches: make([]*sharedFetch[V], len(keys)),
		owned:   make([]bool, len(keys)),
		open:    make([]bool, len(keys)),
	}
	g.mtx.Lock()
	defer g.mtx.Unlock()
	if g.m == nil {
		g.m = make(map[string]*sharedFetch[V])
	}
	for i, key := range keys {
		f := g.m[key]
		if f != nil {
			f.demand.Raise(class)
			f.waiters++
		} else {
			f = &sharedFetch[V]{done: make(chan struct{}), cancel: func() {}, demand: demand, owned: true}
			g.m[key] = f
			c.owned[i] = true
		}
		c.fetches[i], c.open[i] = f, true
	}
	return c, block.WithDemand(ctx, demand)
}

// partition visits each claimed position in order through owned or joined,
// according to which fetches this batch must settle and which it must wait for.
func (c *fetchClaim[V]) partition(owned, joined func(int)) {
	for i, owns := range c.owned {
		if owns {
			owned(i)
		} else {
			joined(i)
		}
	}
}

// settle publishes the result of owned fetch i to its joiners.
func (c *fetchClaim[V]) settle(i int, val V, err error) {
	// Retire the key so a later caller starts afresh, then wake the joiners.
	if !c.owned[i] || !c.open[i] {
		return
	}
	c.open[i] = false
	f := c.fetches[i]
	c.g.mtx.Lock()
	if c.g.m[c.keys[i]] == f {
		delete(c.g.m, c.keys[i])
	}
	c.g.mtx.Unlock()
	f.val, f.err = val, err
	close(f.done)
}

// wait waits for joined fetch i, as a caller of do waits. An abandoned fetch
// answers errFetchAbandoned, and the caller reads the key afresh.
func (c *fetchClaim[V]) wait(ctx context.Context, i int) (V, error) {
	c.open[i] = false
	return c.g.wait(ctx, c.keys[i], c.fetches[i])
}

// release abandons each owned fetch the batch did not settle and leaves each
// joined fetch it did not wait for.
func (c *fetchClaim[V]) release() {
	for i, open := range c.open {
		switch {
		case !open:
		case c.owned[i]:
			var zero V
			c.settle(i, zero, errFetchAbandoned)
		default:
			c.open[i] = false
			c.g.leave(c.keys[i], c.fetches[i])
		}
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
