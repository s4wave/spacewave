package space_world

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// storedPages holds the transformer the Adapter reads its kept World pages
// with: that of the last World cursor it built, since every World cursor
// writes and reads the Space's pages under the Space's transform.
type storedPages struct {
	// mtx guards xfrm.
	mtx sync.Mutex
	// xfrm decodes the World's pages, nil before the first cursor.
	xfrm block.Transformer
}

// set records the transformer of a World cursor just built.
func (s *storedPages) set(xfrm block.Transformer) {
	s.mtx.Lock()
	s.xfrm = xfrm
	s.mtx.Unlock()
}

// get returns the recorded transformer, nil before the first cursor.
func (s *storedPages) get() block.Transformer {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.xfrm
}

// countStoredPages counts into world_block.StoredPages, by tree and level, the
// kept pages below the Worlds at roots that the next Flush or root move
// stores: those no open Flush stored. Call it just before the store, so each
// page counts once, where it reaches the store.
//
// It misses the pages a stage stored at put time, such as an overflowed
// writer's, since the PendingStore never kept them, and two stores that
// overlap may both count a page that only one of them writes.
//
// Cost per store: one decode of each new page, and a lookup of each ref the
// page holds. Stored pages those refs identify end the walk unread.
func (a *Adapter) countStoredPages(ctx context.Context, roots ...*block.BlockRef) {
	// Skip a transient adapter, and one with no World cursor yet.
	xfrm := a.storedPages.get()
	if a.opts.transient || len(roots) == 0 || xfrm == nil {
		return
	}

	// Count the kept pages the store adds, and only report a failure, since
	// the count is not part of the store.
	err := world_block.WalkPages(ctx, a.pages, xfrm, roots, func(p world_block.Page) (bool, error) {
		size, ok := a.pages.Unflushed(p.Ref)
		if ok {
			world_block.StoredPages.Add(p, size)
		}
		return ok, nil
	})
	if err != nil {
		a.le.WithError(err).Debug("cannot count stored world pages")
	}
}
