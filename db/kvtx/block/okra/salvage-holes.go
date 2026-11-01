package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
)

// salvageHolesConcurrency bounds the pages one SalvageHoles walk loads at
// once. Each load waits on a round trip to the store, so a cold walk over a
// remote store is bound by how many loads are in flight, not by bandwidth.
const salvageHolesConcurrency = 4096

// SalvageHoles returns the pages in [low, high) that read as not found or
// unavailable, ordered by their low key, and the number of readable leaf keys,
// retaining no entries.
//
// Unlike Salvage it visits pages in no particular order: a pool of workers
// loads every page of the tree, up to salvageHolesConcurrency at once, so a
// walk over a remote store keeps many reads in flight. Memory grows with the
// queue of pages found but not yet loaded, not with the tree.
func (t *Tx) SalvageHoles(ctx context.Context, low, high []byte) (uint64, []SalvageHole, error) {
	// An empty tree holds nothing.
	if t.root.GetSize() == 0 {
		return 0, nil, nil
	}

	// Load the root page, which is itself a hole when it is missing.
	page, cursor, err := t.getRootPage(ctx)
	if err != nil {
		if !isSalvageHole(err) {
			return 0, nil, err
		}
		return 0, []SalvageHole{{
			Ref:  t.root.GetRootPageRef().Clone(),
			Keys: t.root.GetSize(),
			Err:  err,
		}}, nil
	}

	// Queue the root page's children.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := &holeWalk{span: salvage{low: low, high: high}, cancel: cancel}
	w.cond = sync.NewCond(&w.mu)
	w.visit(page, cursor, nil, nil)

	// Load the rest of the tree with a pool of loaders.
	var wg sync.WaitGroup
	for range salvageHolesConcurrency {
		wg.Go(func() { w.work(ctx) })
	}
	wg.Wait()
	if w.err != nil {
		return 0, nil, w.err
	}

	// Order the holes by key, as an ordered walk would find them.
	slices.SortFunc(w.holes, func(a, b SalvageHole) int {
		return bytes.Compare(a.Low, b.Low)
	})
	return w.keys.Load(), w.holes, nil
}

// holeWalk is one SalvageHoles walk: a stack of child pages to load, shared
// by a pool of workers.
type holeWalk struct {
	// span bounds the keys to count.
	span salvage
	// cancel stops the other workers after an error.
	cancel context.CancelFunc
	// keys counts the readable leaf keys in range.
	keys atomic.Uint64

	// mu guards the fields below; cond wakes workers waiting for work.
	mu   sync.Mutex
	cond *sync.Cond
	// stack holds the pages to load. It is a stack so the walk stays near
	// depth first and frees each subtree's pages soon after it loads them.
	stack []holeChild
	// pending counts the pages queued or loading. The walk ends at zero.
	pending int
	// holes collects the pages that could not be read.
	holes []SalvageHole
	// err is the first read error that is not a hole.
	err error
}

// holeChild is a child page to load: entry index of parent, bounded by
// [low, high).
type holeChild struct {
	parent *block.Cursor
	index  int
	ref    *block.BlockRef
	level  uint32
	keys   uint64
	// low and high bound the child's keys.
	low, high []byte
	// release drops the child from its parent once loaded. A dirty parent may
	// hold children that are not stored yet, so it keeps them.
	release bool
}

// visit counts the readable leaf keys of page in range, or queues its
// children in range when it is an internal page. Its keys lie in
// [pageLow, pageHigh).
func (w *holeWalk) visit(page *Page, cursor *block.Cursor, pageLow, pageHigh []byte) {
	// Count the leaf keys in range.
	if page.GetLevel() == 0 {
		var n uint64
		for _, ent := range page.GetEntries() {
			if !ent.GetAnchor() && w.span.contains(ent.GetKey()) {
				n++
			}
		}
		w.keys.Add(n)
		return
	}

	// Collect the children in range, bounding each one's keys by its entry
	// key and the next entry's key.
	entries := page.GetEntries()
	release := !cursor.IsDirty()
	var children []holeChild
	for i, ent := range entries {
		low := pageLow
		if !ent.GetAnchor() {
			low = ent.GetKey()
		}
		high := pageHigh
		if i+1 < len(entries) {
			high = entries[i+1].GetKey()
		}
		if !w.span.overlaps(low, high) {
			continue
		}
		children = append(children, holeChild{
			parent:  cursor,
			index:   i,
			ref:     ent.GetChildRef(),
			level:   page.GetLevel() - 1,
			keys:    ent.GetSize(),
			low:     low,
			high:    high,
			release: release,
		})
	}

	// Queue them for the workers.
	if len(children) == 0 {
		return
	}
	w.mu.Lock()
	w.stack = append(w.stack, children...)
	w.pending += len(children)
	w.mu.Unlock()
	w.cond.Broadcast()
}

// work loads queued pages until the walk ends or fails.
func (w *holeWalk) work(ctx context.Context) {
	for {
		// Take the next page, waiting while other workers may still queue more.
		w.mu.Lock()
		for len(w.stack) == 0 && w.pending != 0 && w.err == nil {
			w.cond.Wait()
		}
		if len(w.stack) == 0 || w.err != nil {
			w.mu.Unlock()
			return
		}
		child := w.stack[len(w.stack)-1]
		w.stack[len(w.stack)-1] = holeChild{}
		w.stack = w.stack[:len(w.stack)-1]
		w.mu.Unlock()

		// Load it, recording a missing page as a hole.
		err := w.load(ctx, &child)

		// Finish the page, waking the waiting workers when the walk ends or fails.
		w.mu.Lock()
		w.pending--
		if err != nil && w.err == nil {
			w.err = err
			w.cancel()
		}
		done := w.pending == 0 || w.err != nil
		w.mu.Unlock()
		if done {
			w.cond.Broadcast()
		}
	}
}

// load reads one child page and visits it, recording it as a hole when it
// reads as not found or unavailable.
func (w *holeWalk) load(ctx context.Context, child *holeChild) error {
	// Read the page, dropping it from its parent once read.
	cursor := child.parent.FollowRef(entryChildRefID(child.index), child.ref)
	page, err := loadPage(ctx, cursor)
	if child.release {
		child.parent.ClearRef(entryChildRefID(child.index))
	}

	// Record a missing page as a hole, or visit a read one.
	if err != nil {
		if !isSalvageHole(err) {
			return err
		}
		w.mu.Lock()
		w.holes = append(w.holes, SalvageHole{
			Ref:   child.ref.Clone(),
			Level: child.level,
			Low:   slices.Clone(child.low),
			High:  slices.Clone(child.high),
			Keys:  child.keys,
			Err:   err,
		})
		w.mu.Unlock()
		return nil
	}
	w.visit(page, cursor, child.low, child.high)
	return nil
}
