package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// SalvageHole is a page of the tree that could not be read.
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

// Salvage returns, in key order, every leaf entry the tree holds in
// [low, high) that it can read, and the pages it could not read. An empty low
// or high leaves that side unbounded. A page that reads as not found is a
// hole and the walk continues past it; any other error stops the walk.
func (t *Tx) Salvage(ctx context.Context, low, high []byte) ([]BuildEntry, []SalvageHole, error) {
	// An empty tree holds nothing.
	if t.root.GetSize() == 0 {
		return nil, nil, nil
	}

	// Load the root page, which is itself a hole when it is missing.
	s := &salvage{low: low, high: high}
	page, cursor, err := t.getRootPage(ctx)
	if err != nil {
		if !errors.Is(err, block.ErrNotFound) {
			return nil, nil, err
		}
		return nil, []SalvageHole{{
			Ref:  t.root.GetRootPageRef().Clone(),
			Keys: t.root.GetSize(),
			Err:  err,
		}}, nil
	}

	// Walk the whole key space from the root page.
	s.sem = make(chan struct{}, salvageConcurrency)
	res := s.walk(ctx, page, cursor, nil, nil)
	if res.err != nil {
		return nil, nil, res.err
	}
	return res.entries, res.holes, nil
}

// salvageConcurrency bounds the pages a Salvage walk loads at once.
const salvageConcurrency = 32

// salvage holds the bounds and load slots of a Salvage walk.
type salvage struct {
	// low and high bound the keys to collect; empty is unbounded.
	low, high []byte
	// sem holds a slot for each child page loading on its own goroutine.
	sem chan struct{}
}

// salvageResult is what a walk below one page collected.
type salvageResult struct {
	// entries are the readable leaf entries in key order.
	entries []BuildEntry
	// holes are the pages that read as not found, in key order.
	holes []SalvageHole
	// err stops the walk.
	err error
}

// walk collects the entries below page, whose keys lie in [pageLow, pageHigh).
// Child pages load in parallel while load slots are free and inline
// otherwise, and their results join in key order.
func (s *salvage) walk(ctx context.Context, page *Page, cursor *block.Cursor, pageLow, pageHigh []byte) salvageResult {
	var out salvageResult
	entries := page.GetEntries()

	// Collect the readable leaf keys in range.
	if page.GetLevel() == 0 {
		for _, ent := range entries {
			if ent.GetAnchor() || !s.contains(ent.GetKey()) {
				continue
			}
			out.entries = append(out.entries, BuildEntry{
				Key:         slices.Clone(ent.GetKey()),
				ValueRef:    ent.GetValueRef().Clone(),
				ValueIsBlob: ent.GetValueIsBlob(),
				ValueBlob:   ent.GetValueBlob().CloneVT(),
			})
		}
		return out
	}

	// Walk each child in range, recording a missing child as a hole.
	children := make([]salvageResult, len(entries))
	var wg sync.WaitGroup
	for i, ent := range entries {
		if err := ctx.Err(); err != nil {
			children[i].err = err
			break
		}

		// Bound the keys of this entry by its key and the next entry's key.
		entLow := pageLow
		if !ent.GetAnchor() {
			entLow = ent.GetKey()
		}
		entHigh := pageHigh
		if i+1 < len(entries) {
			entHigh = entries[i+1].GetKey()
		}
		if !s.overlaps(entLow, entHigh) {
			continue
		}

		childCursor := page.FollowChild(cursor, i)
		visit := func() {
			child, err := loadPage(ctx, childCursor)
			if err != nil {
				if !errors.Is(err, block.ErrNotFound) {
					children[i].err = err
					return
				}
				children[i].holes = []SalvageHole{{
					Ref:   ent.GetChildRef().Clone(),
					Level: page.GetLevel() - 1,
					Low:   slices.Clone(entLow),
					High:  slices.Clone(entHigh),
					Keys:  ent.GetSize(),
					Err:   err,
				}}
				return
			}
			children[i] = s.walk(ctx, child, childCursor, entLow, entHigh)
		}
		select {
		case s.sem <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-s.sem }()
				visit()
			}()
		default:
			visit()
		}
	}
	wg.Wait()

	// Join the children in key order.
	for _, child := range children {
		if child.err != nil {
			return salvageResult{err: child.err}
		}
		out.entries = append(out.entries, child.entries...)
		out.holes = append(out.holes, child.holes...)
	}
	return out
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
func (t *Tx) ReplaceAllEntries(ctx context.Context, entries []BuildEntry) error {
	return t.replaceAll(ctx, func(yield func(BuildEntry, error) bool) {
		for _, entry := range entries {
			if !yield(entry, nil) {
				return
			}
		}
	})
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
