package web_pkg

import (
	"context"
	"sync"
)

// RoutineGroup tracks routines so StopAccepting and Wait can drain them.
type RoutineGroup struct {
	mtx    sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

// Wrap registers a routine until it returns, refusing work after shutdown begins.
func (g *RoutineGroup) Wrap(r func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		if !g.Begin() {
			return context.Canceled
		}
		defer g.Done()
		return r(ctx)
	}
}

// Begin reserves a routine slot unless shutdown has begun.
func (g *RoutineGroup) Begin() bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

// Done releases a routine slot.
func (g *RoutineGroup) Done() {
	g.wg.Done()
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
