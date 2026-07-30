package web_pkg_rpc_server

import (
	"context"
	"sync"
)

type routineGroup struct {
	mtx    sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func (g *routineGroup) wrap(r func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		if !g.begin() {
			return context.Canceled
		}
		defer g.wg.Done()
		return r(ctx)
	}
}

func (g *routineGroup) begin() bool {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

func (g *routineGroup) stopAccepting() {
	g.mtx.Lock()
	g.closed = true
	g.mtx.Unlock()
}

func (g *routineGroup) wait() {
	g.wg.Wait()
}
