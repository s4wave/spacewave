package kvtx_block_okra

import (
	"context"
	"iter"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// ReplaceAll builds one tree from sorted key/value pairs. It replaces all keys,
// retaining the same packed representation as incremental Set operations.
// Existing iterators keep their snapshot. Discard the transaction on error.
func (t *Tx) ReplaceAll(ctx context.Context, values iter.Seq2[[]byte, []byte]) error {
	return t.replaceAll(ctx, func(yield func(BuildEntry, error) bool) {
		for key, value := range values {
			entry, err := t.buildValueEntry(ctx, key, value)
			if !yield(entry, err) {
				return
			}
		}
	})
}

// ReplaceAllBlocks builds one tree from sorted keys and their value blocks.
// Each block is encoded into the tree's staged writes as the builder reaches
// it, so the build retains no cursor per value. Discard the transaction on
// error.
func (t *Tx) ReplaceAllBlocks(ctx context.Context, values iter.Seq2[[]byte, block.Block]) error {
	return t.replaceAll(ctx, func(yield func(BuildEntry, error) bool) {
		for key, value := range values {
			ref, err := writeStagedBlock(ctx, t.bcs, value)
			if !yield(BuildEntry{Key: key, ValueRef: ref}, err) {
				return
			}
		}
	})
}

// replaceAll replaces every key with a tree built from sorted entries. The
// first entry error stops the build and is returned.
func (t *Tx) replaceAll(ctx context.Context, values iter.Seq2[BuildEntry, error]) error {
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.commitOnce.Load() {
		return kvtx.ErrDiscarded
	}
	var valueErr error
	entries := func(yield func(BuildEntry) bool) {
		for entry, err := range values {
			if valueErr = ctx.Err(); valueErr != nil {
				return
			}
			if err != nil {
				valueErr = err
				return
			}
			if !yield(entry) {
				return
			}
		}
	}
	root, err := buildTree(entries, func(page *Page) (*block.BlockRef, error) {
		return writeStagedBlock(ctx, t.bcs, page)
	})
	if valueErr != nil {
		return valueErr
	}
	if err != nil {
		return err
	}
	if root.GetSize() == 0 {
		return t.setEmptyRoot(ctx)
	}
	// The root references its staged top page; reads follow it on demand.
	t.replaceRoot(root, nil)
	return ctx.Err()
}
