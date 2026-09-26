package kvtx_block_okra

import (
	"bytes"
	"context"
	"iter"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// BuildEntry is one sorted key/value entry used by the Okra tree builder.
// The builder copies the entry before requesting the next one.
type BuildEntry struct {
	Key []byte

	ValueRef    *block.BlockRef
	ValueIsBlob bool
	ValueBlob   *blob.Blob
}

// BuildTree builds a packed Okra page DAG from sorted key/value refs.
func BuildTree(
	ctx context.Context,
	store block.StoreOps,
	xfrm block.Transformer,
	putOpts *block.PutOpts,
	entries iter.Seq2[[]byte, *block.BlockRef],
) (*block.Transaction, *block.Cursor, error) {
	return BuildTreeWithEntries(ctx, store, xfrm, putOpts, func(yield func(BuildEntry) bool) {
		for key, ref := range entries {
			if !yield(BuildEntry{Key: key, ValueRef: ref}) {
				return
			}
		}
	})
}

// BuildTreeWithEntries builds a packed Okra page DAG from sorted entries. The
// pages are staged in the returned transaction until it writes the root.
func BuildTreeWithEntries(
	ctx context.Context,
	store block.StoreOps,
	xfrm block.Transformer,
	putOpts *block.PutOpts,
	entries iter.Seq[BuildEntry],
) (*block.Transaction, *block.Cursor, error) {
	tx, rootCursor := block.NewTransaction(store, xfrm, nil, putOpts)
	root, err := buildTree(entries, func(page *Page) (*block.BlockRef, error) {
		return writeStagedPage(ctx, rootCursor, page)
	})
	if err != nil {
		return nil, nil, err
	}
	rootCursor.SetBlock(root, true)
	return tx, rootCursor, nil
}

// buildTree packs the sorted entries into pages, writing each finished page
// with writePage, and returns the root referencing the top page.
func buildTree(entries iter.Seq[BuildEntry], writePage func(*Page) (*block.BlockRef, error)) (*Root, error) {
	builder := newTreeBuilder(writePage)
	var prevKey []byte
	for ent := range entries {
		if len(ent.Key) == 0 {
			return nil, ErrUnexpectedEntryMetadata
		}
		if prevKey != nil && bytes.Compare(prevKey, ent.Key) >= 0 {
			return nil, ErrUnsortedEntries
		}
		leafHash, err := hashBuildEntry(ent)
		if err != nil {
			return nil, err
		}
		leaf := &Entry{
			Key:         slices.Clone(ent.Key),
			Hash:        leafHash,
			Size:        1,
			ValueRef:    ent.ValueRef.Clone(),
			ValueIsBlob: ent.ValueIsBlob,
			ValueBlob:   ent.ValueBlob.CloneVT(),
		}
		if err := builder.add(0, leaf); err != nil {
			return nil, err
		}
		prevKey = leaf.Key
	}
	// Without entries the root stays empty.
	if prevKey == nil {
		return &Root{}, nil
	}

	rootPageRef, top, height, err := builder.finish()
	if err != nil {
		return nil, err
	}
	return &Root{
		Size:         top.GetSize(),
		Height:       height,
		RootHash:     slices.Clone(top.GetHash()),
		HashSize:     HashSize,
		FanoutDegree: FanoutDegree,
		RootPageRef:  rootPageRef,
	}, nil
}
