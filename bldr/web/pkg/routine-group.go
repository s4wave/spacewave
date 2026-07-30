package web_pkg

import (
	"context"
	"sync"
)

// RoutineGroup tracks routines that must stop before their owner closes.
type RoutineGroup struct {
	mtx    sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

// Wrap registers a routine until it returns, refusing work after shutdown begins.
func (g *RoutineGroup) Wrap(r func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		if !g.begin() {
			return context.Canceled
		}
		defer g.done()
		return r(ctx)
	}
}

func (g *RoutineGroup) begin() bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

// Begin reserves a routine slot unless shutdown has begun.
func (g *RoutineGroup) Begin() bool {
	return g.begin()
}

func (g *RoutineGroup) done() {
	g.wg.Done()
}

// Done releases a routine slot.
func (g *RoutineGroup) Done() {
	g.done()
}

// StopAccepting prevents new routines from joining the group.
func (g *RoutineGroup) StopAccepting() {
	g.mtx.Lock()
	g.closed = true
	g.mtx.Unlock()
}

// Wait blocks until all accepted routines have returned.
func (g *RoutineGroup) Wait() {
	g.wg.Wait()
}
