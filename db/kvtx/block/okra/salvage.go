package kvtx_block_okra

import (
	"bytes"
	"context"
	"iter"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// SalvageHole is a page of the tree that could not be read: not found, or
// unavailable when no peer was connected to serve it.
type SalvageHole struct {
	// Ref is the missing page.
	Ref *block.BlockRef
	// Level is the level of the missing page.
	Level uint32
	// Low is the first key the page can hold, or empty from the start of the
	// key space.
	Low []byte
	// High is the first key past the page, or empty to the end of the key
	// space.
	High []byte
	// Keys is the number of leaf keys the parent counted below the page.
	Keys uint64
	// Err is the read error.
	Err error
}

// errSalvageStop ends a Salvage walk when its consumer stops early.
var errSalvageStop = errors.New("salvage walk stopped")

// salvageConcurrency bounds the child pages one page loads at once.
const salvageConcurrency = 32

// Salvage walks the leaf entries in [low, high) in key order. It calls entry
// for each readable leaf entry and hole for each page that reads as not found
// or unavailable, then continues past it. Either callback may be nil, and
// either may return false to stop the walk. Any other read error stops the
// walk. An empty low or high leaves that side unbounded. A caller that must
// not wait for peers to serve pages it lacks reads under
// block.WithoutPeerWait, which reports those pages unavailable.
//
// Nothing is retained between callbacks: the walk holds only the pages on
// the path to the current one, so it runs in memory bounded by the tree's
// height. The children of each page load in parallel.
func (t *Tx) Salvage(
	ctx context.Context,
	low, high []byte,
	entry func(BuildEntry) bool,
	hole func(SalvageHole) bool,
) error {
	// An empty tree holds nothing.
	if t.root.GetSize() == 0 {
		return nil
	}

	// Load the root page, which is itself a hole when it is missing.
	s := &salvage{low: low, high: high, entry: entry, hole: hole}
	page, cursor, err := t.getRootPage(ctx)
	if err != nil {
		if !isSalvageHole(err) {
			return err
		}
		s.emitHole(SalvageHole{
			Ref:  t.root.GetRootPageRef().Clone(),
			Keys: t.root.GetSize(),
			Err:  err,
		})
		return nil
	}

	// Walk the whole key space from the root page.
	if err := s.walk(ctx, page, cursor, nil, nil); err != nil && err != errSalvageStop {
		return err
	}
	return nil
}

// SalvageHoles returns the pages in [low, high) that read as not found and
// the number of readable leaf keys, retaining no entries.
func (t *Tx) SalvageHoles(ctx context.Context, low, high []byte) (uint64, []SalvageHole, error) {
	var keys uint64
	var holes []SalvageHole
	err := t.Salvage(ctx, low, high, func(BuildEntry) bool {
		keys++
		return true
	}, func(h SalvageHole) bool {
		holes = append(holes, h)
		return true
	})
	return keys, holes, err
}

// salvage is one Salvage walk.
type salvage struct {
	// low and high bound the keys to collect; empty is unbounded.
	low, high []byte
	// entry and hole receive the walk's results; nil ignores them.
	entry func(BuildEntry) bool
	hole  func(SalvageHole) bool
}

// isSalvageHole reports whether a page read error makes the page a hole: the
// page is not found, or no peer is connected to serve it.
func isSalvageHole(err error) bool {
	return errors.Is(err, block.ErrNotFound) || errors.Is(err, block.ErrUnavailable)
}

// emitHole passes a hole to the consumer, reporting whether to continue.
func (s *salvage) emitHole(h SalvageHole) bool {
	return s.hole == nil || s.hole(h)
}

// salvageChild is a child page loaded for a walk.
type salvageChild struct {
	// index is the entry of the parent page that links the child.
	index int
	// low and high bound the child's keys.
	low, high []byte
	// page and cursor are the loaded child, nil when err is set.
	page   *Page
	cursor *block.Cursor
	// err is the load error.
	err error
}

// walk passes on the entries below page, whose keys lie in
// [pageLow, pageHigh), in key order.
func (s *salvage) walk(ctx context.Context, page *Page, cursor *block.Cursor, pageLow, pageHigh []byte) error {
	entries := page.GetEntries()

	// Pass on the readable leaf keys in range.
	if page.GetLevel() == 0 {
		for _, ent := range entries {
			if ent.GetAnchor() || !s.contains(ent.GetKey()) {
				continue
			}
			if s.entry == nil {
				continue
			}
			if !s.entry(BuildEntry{
				Key:         slices.Clone(ent.GetKey()),
				ValueRef:    ent.GetValueRef().Clone(),
				ValueIsBlob: ent.GetValueIsBlob(),
				ValueBlob:   ent.GetValueBlob().CloneVT(),
			}) {
				return errSalvageStop
			}
		}
		return nil
	}

	// Collect the children in range, bounding each one's keys by its entry
	// key and the next entry's key.
	var children []salvageChild
	for i, ent := range entries {
		low := pageLow
		if !ent.GetAnchor() {
			low = ent.GetKey()
		}
		high := pageHigh
		if i+1 < len(entries) {
			high = entries[i+1].GetKey()
		}
		if s.overlaps(low, high) {
			children = append(children, salvageChild{index: i, low: low, high: high})
		}
	}

	// Load the children in parallel.
	sem := make(chan struct{}, salvageConcurrency)
	var wg sync.WaitGroup
	for i := range children {
		child := &children[i]
		child.cursor = page.FollowChild(cursor, child.index)
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			child.page, child.err = loadPage(ctx, child.cursor)
		}()
	}
	wg.Wait()

	// Walk the children in key order, passing on a missing one as a hole.
	// The parent cursor keeps each child it followed, with its decoded page,
	// until the transaction ends. Below a clean parent every child is a plain
	// load the page entry can follow again, so drop each one once walked, or
	// a walk of the whole tree holds every page it read. A dirty parent may
	// hold children that are not stored yet, so keep those.
	release := !cursor.IsDirty()
	for i := range children {
		child := &children[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if child.err != nil {
			if !isSalvageHole(child.err) {
				return child.err
			}
			ent := entries[child.index]
			hole := SalvageHole{
				Ref:   ent.GetChildRef().Clone(),
				Level: page.GetLevel() - 1,
				Low:   slices.Clone(child.low),
				High:  slices.Clone(child.high),
				Keys:  ent.GetSize(),
				Err:   child.err,
			}
			s.release(cursor, child, release)
			if !s.emitHole(hole) {
				return errSalvageStop
			}
			continue
		}
		err := s.walk(ctx, child.page, child.cursor, child.low, child.high)
		s.release(cursor, child, release)
		if err != nil {
			return err
		}
	}
	return nil
}

// release drops a walked child from the walk and, with detach set, from its
// parent cursor, so its decoded page can be collected.
func (s *salvage) release(parent *block.Cursor, child *salvageChild, detach bool) {
	if detach {
		parent.ClearRef(entryChildRefID(child.index))
	}
	child.page, child.cursor = nil, nil
}

// contains reports whether key lies in [s.low, s.high).
func (s *salvage) contains(key []byte) bool {
	if len(s.low) != 0 && bytes.Compare(key, s.low) < 0 {
		return false
	}
	return len(s.high) == 0 || bytes.Compare(key, s.high) < 0
}

// overlaps reports whether [low, high) meets [s.low, s.high). An empty low or
// high is unbounded.
func (s *salvage) overlaps(low, high []byte) bool {
	if len(s.high) != 0 && len(low) != 0 && bytes.Compare(low, s.high) >= 0 {
		return false
	}
	return len(s.low) == 0 || len(high) == 0 || bytes.Compare(high, s.low) > 0
}

// ReplaceAllEntries builds one tree from sorted leaf entries, keeping each
// entry's value as it is stored, and replaces all keys with it. Discard the
// transaction on error.
func (t *Tx) ReplaceAllEntries(ctx context.Context, entries iter.Seq2[BuildEntry, error]) error {
	return t.replaceAll(ctx, entries)
}

// ValueEntry returns the leaf entry of key holding value, stored the way this
// tree stores new values: inline when small enough, otherwise in a blob
// written into the transaction's staged writes.
func (t *Tx) ValueEntry(ctx context.Context, key, value []byte) (BuildEntry, error) {
	return t.buildValueEntry(ctx, key, value)
}

// CopyEntries returns entries read from the tree from, with each value that
// lives in its own block written again into this tree's staged writes, so the
// entries stay readable here without the blocks of from. Inline values are
// kept as they are.
func (t *Tx) CopyEntries(ctx context.Context, from *Tx, entries []BuildEntry) ([]BuildEntry, error) {
	out := make([]BuildEntry, 0, len(entries))
	for _, entry := range entries {
		// Keep an inline value, which the entry holds itself.
		if entry.ValueBlob != nil || entry.ValueRef.GetEmpty() {
			out = append(out, entry)
			continue
		}

		// Read the value from its block and store it again here.
		value, found, err := from.Get(ctx, entry.Key)
		if err != nil {
			return nil, errors.Wrapf(err, "read value of key %q", entry.Key)
		}
		if !found {
			return nil, errors.Errorf("key %q not found while copying its value", entry.Key)
		}
		copied, err := t.buildValueEntry(ctx, entry.Key, value)
		if err != nil {
			return nil, err
		}
		out = append(out, copied)
	}
	return out, nil
}
