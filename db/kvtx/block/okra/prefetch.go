package kvtx_block_okra

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
)

// PrefetchWriteBatch reads the pages a write batch of keys reads before it
// rebuilds anything, and writes nothing. The batch reads the path from the
// root to each key's leaf, and at each level it pulls the preceding page into
// the rebuild when the changed entry begins its page: at the leaf when the key
// is the page's first entry, and above it when the rebuilt child is its
// page's first child. The pages stay decoded in the transaction's cursors, so
// the batch then reads them from memory.
//
// A read that fails, such as one deferred to a caller's peer fetch, skips the
// rest of that path and goes on, so one pass records every page missing at
// its first missing level. It returns the first failure.
func (t *Tx) PrefetchWriteBatch(ctx context.Context, keys [][]byte) error {
	// Only a written, populated tree has pages to read.
	if !t.write || t.commitOnce.Load() || t.root.GetSize() == 0 {
		return ctx.Err()
	}

	// Walk the keys in order, reusing a leaf while the keys stay inside it:
	// a later key in the same leaf is not its first entry and shares its path.
	sorted := slices.Clone(keys)
	slices.SortFunc(sorted, bytes.Compare)
	var first error
	var leaf *Page
	for _, key := range sorted {
		if err := ctx.Err(); err != nil {
			return err
		}
		nodeKey := okraNodeKey{key: key}
		if len(key) == 0 || (leaf != nil && pageContainsKey(leaf, nodeKey)) {
			continue
		}
		path, err := t.findPagePath(ctx, 0, nodeKey)
		if err != nil {
			leaf = nil
			first = cmp.Or(first, err)
			continue
		}
		leaf = path.leaf().page
		first = cmp.Or(first, t.prefetchPredecessors(ctx, path, len(path)-1, nodeKey))
	}
	return first
}

// prefetchPredecessors reads the preceding page of each frame of path at or
// above depth that a batch changing path's leaf pulls into its rebuild, and
// then the predecessors of those pages' own frames above them. The root, at
// depth zero, has none.
func (t *Tx) prefetchPredecessors(ctx context.Context, path okraPagePath, depth int, key okraNodeKey) error {
	var first error
	for d := depth; d >= 1; d-- {
		page := path[d].page
		var startsChanged bool
		if d == len(path)-1 {
			startsChanged = compareNodeKeys(pageStartKey(page), key) == 0
		} else {
			startsChanged = path[d+1].index == 0
		}
		if !startsChanged || page.GetStartsAtAnchor() {
			continue
		}
		prev, ok, err := t.previousPagePath(ctx, path[:d+1], page.GetLevel())
		if err != nil {
			first = cmp.Or(first, err)
			continue
		}
		if ok {
			first = cmp.Or(first, t.prefetchPredecessors(ctx, prev, d-1, key))
		}
	}
	return first
}

// prefetchHeld reads the pages the batch applying a held write to key reads;
// see PrefetchWriteBatch. Its failure is left to the batch.
func (t *Tx) prefetchHeld(ctx context.Context, key []byte) {
	_ = t.PrefetchWriteBatch(ctx, [][]byte{key})
}

// _ is a type assertion
var _ kvtx.WriteBatchPrefetcher = (*Tx)(nil)
