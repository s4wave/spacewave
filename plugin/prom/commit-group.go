package prom_plugin

import (
	"context"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// commitGroup commits the windows of every target one Device scrapes in one
// Space, in one World transaction per commit window. Each target's process
// joins with its own handle on the Space's World engine; any member's handle
// can write every member's target.
type commitGroup struct {
	// le is the logger.
	le *logrus.Entry
	// window is the time between commits.
	window time.Duration

	// mtx guards the fields below.
	mtx sync.Mutex
	// members maps each joined target's Store to its World engine handle.
	members map[*Store]world.Engine
	// cancel stops the commit loop; nil while the group has no members.
	cancel context.CancelFunc
}

// newCommitGroup constructs an empty commit group.
func newCommitGroup(le *logrus.Entry, window time.Duration) *commitGroup {
	return &commitGroup{le: le, window: window, members: make(map[*Store]world.Engine)}
}

// join adds store, written through eng, to the next commits, starting the
// commit loop for the first member. It returns the function that removes the
// store, which stops the loop after the last member leaves. The uncommitted
// window of a leaving store is dropped, as a crash would drop it.
func (g *commitGroup) join(store *Store, eng world.Engine) func() {
	// Add the member, starting the loop if it is the first.
	g.mtx.Lock()
	g.members[store] = eng
	if g.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		g.cancel = cancel
		go g.run(ctx)
	}
	g.mtx.Unlock()

	// Remove the member, stopping the loop after the last.
	return func() {
		g.mtx.Lock()
		defer g.mtx.Unlock()
		delete(g.members, store)
		if len(g.members) == 0 && g.cancel != nil {
			g.cancel()
			g.cancel = nil
		}
	}
}

// run commits the members once per window until ctx ends.
func (g *commitGroup) run(ctx context.Context) {
	//gostyle:timer deadline
	t := time.NewTicker(g.window)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		g.commit(ctx)
	}
}

// commit writes every member's open window in one transaction. A failed
// commit leaves each window frozen, and the next commit retries it.
func (g *commitGroup) commit(ctx context.Context) {
	// Snapshot the members and pick a World handle to write through.
	g.mtx.Lock()
	stores := make([]*Store, 0, len(g.members))
	var eng world.Engine
	for store, storeEng := range g.members {
		stores = append(stores, store)
		eng = storeEng
	}
	g.mtx.Unlock()
	if eng == nil {
		return
	}

	// Commit them together.
	stats, err := CommitStores(ctx, eng, stores...)
	if err != nil {
		if ctx.Err() == nil {
			g.le.WithError(err).Warn("commit scrape window")
		}
		return
	}
	g.le.WithField("segments", stats.Segments).
		WithField("samples", stats.Samples).
		WithField("blocks", stats.Blocks).
		WithField("bytes", stats.Bytes).
		Debug("committed scrape window")
}
