package world

import (
	"context"
	"sync"
)

// bodyLookahead runs the body lookups of keys in key order, up to width ahead
// of the key its reader waits on, so a page of cold bodies waits on about one
// lookup's dependent reads per width keys instead of per key. Lookups past the
// page's end are cancelled at close. Results land in a ring of width slots: a
// lookup starts only once its slot's previous key has been read.
type bodyLookahead struct {
	ctx    context.Context
	cancel context.CancelFunc
	keys   []string
	lookup func(context.Context, string) (*ObjectBody, error)
	slots  []bodyLookaheadSlot
	next   int
	wg     sync.WaitGroup
}

// bodyLookaheadSlot holds one started lookup's result, written before done
// closes.
type bodyLookaheadSlot struct {
	body *ObjectBody
	err  error
	done chan struct{}
}

// newBodyLookahead returns a lookahead of width over keys. A width of 1 or
// less runs each lookup on the reader's goroutine.
func newBodyLookahead(
	ctx context.Context,
	keys []string,
	width int,
	lookup func(context.Context, string) (*ObjectBody, error),
) *bodyLookahead {
	a := &bodyLookahead{ctx: ctx, keys: keys, lookup: lookup}
	if width > 1 && len(keys) > 1 {
		a.ctx, a.cancel = context.WithCancel(ctx)
		a.slots = make([]bodyLookaheadSlot, min(width, len(keys)))
	}
	return a
}

// result returns the lookup of keys[i]. Callers read i in increasing order,
// each once.
func (a *bodyLookahead) result(i int) (*ObjectBody, error) {
	// Run inline without a ring.
	if len(a.slots) == 0 {
		return a.lookup(a.ctx, a.keys[i])
	}

	// Fill the ring up to width keys ahead of i, then wait on i.
	for ; a.next < len(a.keys) && a.next < i+len(a.slots); a.next++ {
		a.start(a.next)
	}
	s := &a.slots[i%len(a.slots)]
	<-s.done
	return s.body, s.err
}

// start runs the lookup of keys[i] into its ring slot.
func (a *bodyLookahead) start(i int) {
	s := &a.slots[i%len(a.slots)]
	*s = bodyLookaheadSlot{done: make(chan struct{})}
	a.wg.Go(func() {
		defer close(s.done)
		s.body, s.err = a.lookup(a.ctx, a.keys[i])
	})
}

// close cancels the lookups still running and waits for them to end.
func (a *bodyLookahead) close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
}
