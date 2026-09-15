package kvtx_block_okra

import (
	"context"
	"iter"

	"github.com/s4wave/spacewave/db/kvtx"
)

// ReplaceAll builds one tree from sorted key/value pairs. It replaces all keys,
// retaining the same packed representation as incremental Set operations.
// Existing iterators keep their snapshot. Discard the transaction on error.
func (t *Tx) ReplaceAll(ctx context.Context, values iter.Seq2[[]byte, []byte]) error {
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.commitOnce.Load() {
		return kvtx.ErrDiscarded
	}
	var valueErr error
	entries := func(yield func(BuildEntry) bool) {
		for key, value := range values {
			if valueErr = ctx.Err(); valueErr != nil {
				return
			}
			ref, err := t.buildBlobValue(ctx, value)
			if err != nil {
				valueErr = err
				return
			}
			if !yield(BuildEntry{Key: key, ValueRef: ref, ValueIsBlob: true}) {
				return
			}
		}
	}
	rootCursor := t.bcs.Detach(false)
	defer discardPages(rootCursor)
	root, err := buildTreeAtCursor(rootCursor, entries)
	if valueErr != nil {
		return valueErr
	}
	if err != nil {
		return err
	}
	if root.GetSize() == 0 {
		return t.setEmptyRoot(ctx)
	}
	t.replaceRoot(root, rootCursor.FollowRef(rootPageRefID, root.GetRootPageRef()))
	return ctx.Err()
}
