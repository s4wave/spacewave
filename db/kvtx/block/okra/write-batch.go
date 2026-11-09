package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
)

// ApplyWriteBatch replaces affected page windows bottom-up rather than once per
// key or once per leaf ancestor path. Work stays local to affected pages: a sparse batch does not rebuild the
// span between its first and last keys. Existing page/cursor snapshot ownership
// and the scalar path's canonical page-boundary rules remain unchanged.
func (t *Tx) ApplyWriteBatch(ctx context.Context, entries []kvtx.WriteBatchEntry) error {
	// Require a live write transaction and a usable context.
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.commitOnce.Load() {
		return kvtx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Apply the held cursor writes first, so this batch lands on top of them.
	if err := t.Flush(ctx); err != nil {
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
	ordered := make([]batchOp, 0, len(last))
	for _, entry := range last {
		ordered = append(ordered, batchOp{key: entry.Key, value: entry.Value, del: entry.Delete})
	}
	slices.SortFunc(ordered, func(a, b batchOp) int { return bytes.Compare(a.key, b.key) })
	return t.applyBatch(ctx, ordered)
}

// batchOp is one key's change in a batch: a delete, a built entry, or a value
// to build an entry from.
type batchOp struct {
	key   []byte
	value []byte
	entry *BuildEntry
	del   bool
}

// applyBatch applies ops, sorted by key with one op per key, rebuilding each
// affected page window once.
func (t *Tx) applyBatch(ctx context.Context, ordered []batchOp) error {
	// Seed the node builder, adding the anchor for an empty tree.
	oldKeys := make([]okraNodeKey, 0, len(ordered))
	nodes := make([]okraLevelNode, 0, len(ordered)+1)
	empty := t.root.GetSize() == 0
	if empty {
		nodes = append(nodes, okraLevelNode{entry: &Entry{Anchor: true, Hash: mustAnchorHash()}})
	}

	// Resolve each entry against its leaf page, reusing the loaded page while
	// lookups stay inside it, and prepare the replacement nodes.
	var page *Page
	for _, op := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}

		var old *Entry
		if !empty {
			if page == nil || !pageContainsKey(page, okraNodeKey{key: op.key}) {
				path, err := t.findPagePath(ctx, 0, okraNodeKey{key: op.key})
				if err != nil {
					return err
				}
				page = path.leaf().page
			}
			idx := page.searchEntry(op.key)
			if idx >= 0 {
				candidate := page.GetEntries()[idx]
				if !candidate.GetAnchor() && bytes.Equal(candidate.GetKey(), op.key) {
					old = candidate
				}
			}
		}

		if !op.del {
			value, err := op.build(ctx, t)
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

	// An empty tree is built directly from the nodes; otherwise the leaf
	// windows rebuild and propagate upward.
	if empty {
		if len(nodes) > 1 {
			return t.setRootFromLevelNodes(ctx, 0, nodes)
		}
		return ctx.Err()
	}
	return t.replaceLevelEntries(ctx, 0, oldKeys, nodes)
}

// build returns op's entry, building it from its value when op holds none.
func (op *batchOp) build(ctx context.Context, t *Tx) (BuildEntry, error) {
	if op.entry != nil {
		return *op.entry, nil
	}
	return t.buildValueEntry(ctx, op.key, op.value)
}

var _ kvtx.WriteBatchTxOps = (*Tx)(nil)
