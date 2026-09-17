package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
)

// ApplyWriteBatch replaces each affected leaf window once rather than once per
// key. Work stays local to affected pages: a sparse batch does not rebuild the
// span between its first and last keys. Existing page/cursor snapshot ownership
// and the scalar path's canonical page-boundary rules remain unchanged.
func (t *Tx) ApplyWriteBatch(ctx context.Context, entries []kvtx.WriteBatchEntry) error {
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.commitOnce.Load() {
		return kvtx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Validate the entire input first and collapse overwrites before building
	// values. Sorting only the private slice never changes the caller's batch.
	last := make(map[string]kvtx.WriteBatchEntry, len(entries))
	for _, entry := range entries {
		if len(entry.Key) == 0 {
			return kvtx.ErrEmptyKey
		}
		last[string(entry.Key)] = entry
	}
	ordered := make([]kvtx.WriteBatchEntry, 0, len(last))
	for _, entry := range last {
		ordered = append(ordered, entry)
	}
	slices.SortFunc(ordered, func(a, b kvtx.WriteBatchEntry) int { return bytes.Compare(a.Key, b.Key) })

	for start := 0; start < len(ordered); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := len(ordered)
		var page *Page
		if t.root.GetSize() != 0 {
			path, err := t.findPagePath(ctx, 0, okraNodeKey{key: ordered[start].Key})
			if err != nil {
				return err
			}
			page = path.leaf().page
			if upper := page.GetUpperBound(); len(upper) != 0 {
				end = start + 1
				for end < len(ordered) && bytes.Compare(ordered[end].Key, upper) < 0 {
					end++
				}
			}
		}
		oldKeys := make([]okraNodeKey, 0, end-start)
		nodes := make([]okraLevelNode, 0, end-start+1)
		if page == nil {
			nodes = append(nodes, okraLevelNode{entry: &Entry{Anchor: true, Hash: mustAnchorHash()}})
		}
		for _, entry := range ordered[start:end] {
			var old *Entry
			if page != nil {
				idx := page.searchEntry(entry.Key)
				if idx >= 0 {
					candidate := page.GetEntries()[idx]
					if !candidate.GetAnchor() && bytes.Equal(candidate.GetKey(), entry.Key) {
						old = candidate
					}
				}
			}
			if !entry.Delete {
				value, err := t.buildValueEntry(ctx, entry.Key, entry.Value)
				if err != nil {
					return err
				}
				node, err := newLevelValueNode(value)
				if err != nil {
					return err
				}
				if old != nil && bytes.Equal(old.GetHash(), node.entry.GetHash()) {
					continue
				}
				nodes = append(nodes, node)
			}
			if old != nil {
				oldKeys = append(oldKeys, entryKey(old))
			}
		}
		if page == nil {
			if len(nodes) > 1 {
				if err := t.setRootFromLevelNodes(ctx, 0, nodes); err != nil {
					return err
				}
			}
		} else if err := t.replaceLevelEntries(ctx, 0, oldKeys, nodes); err != nil {
			return err
		}
		start = end
	}
	return ctx.Err()
}

var _ kvtx.WriteBatchTxOps = (*Tx)(nil)
