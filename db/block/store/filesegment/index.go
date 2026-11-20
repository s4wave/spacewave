// Package block_store_filesegment serves blocks from byte ranges of files on
// this Device. A range is a block when encoding its bytes with the Space's
// transform hashes to the block's ref; the store checks that on every read.
package block_store_filesegment

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// refKeyPrefix prefixes the key of a ref's SegmentSet.
const refKeyPrefix = "r/"

// pathKeyPrefix prefixes the key of a path's PathRefs.
const pathKeyPrefix = "p/"

// Index is the SegmentIndex of one Space: each ref maps to the linked file
// ranges that held its block, and each path to the refs of its ranges. It
// lives in a key/value store on this Device, so it outlives the process.
type Index struct {
	// store holds the entries.
	store kvtx.Store
}

// NewIndex constructs the Index kept in store, which holds no other keys.
func NewIndex(store kvtx.Store) *Index {
	return &Index{store: store}
}

// Lookup returns the segments recorded for ref with one read of the store.
func (x *Index) Lookup(ctx context.Context, ref *block.BlockRef) ([]*Segment, error) {
	// Read the ref's entry.
	key, err := refKey(ref)
	if err != nil {
		return nil, err
	}
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, key)
	if err != nil || !found {
		return nil, err
	}

	// Decode its segments.
	set := &SegmentSet{}
	if err := set.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return set.GetSegments(), nil
}

// SetPath replaces the segments of path with segs, where segs[i] held refs[i],
// in one write: a reader sees the path's old segments or its new ones. A ref
// keeps at most one segment per path, the first. Nil refs drop the path.
func (x *Index) SetPath(ctx context.Context, path string, refs []*block.BlockRef, segs []*Segment) error {
	// Read the refs the path held.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	pkey := []byte(pathKeyPrefix + path)
	old := &PathRefs{}
	data, found, err := tx.Get(ctx, pkey)
	if err != nil {
		return err
	}
	if found {
		if err := old.UnmarshalVT(data); err != nil {
			return err
		}
	}

	// Drop the path's segment from each ref it held.
	sets := make(map[string]*SegmentSet)
	for _, ref := range old.GetRefs() {
		set, err := loadSet(ctx, tx, sets, ref)
		if err != nil {
			return err
		}
		set.Segments = slices.DeleteFunc(set.Segments, func(s *Segment) bool { return s.GetPath() == path })
	}

	// Add the new segments, one per ref.
	next := &PathRefs{}
	for i, ref := range refs {
		set, err := loadSet(ctx, tx, sets, ref)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(set.Segments, func(s *Segment) bool { return s.GetPath() == path }) {
			continue
		}
		set.Segments = append(set.Segments, segs[i])
		next.Refs = append(next.Refs, ref)
	}

	// Write the changed refs and the path, then commit.
	for key, set := range sets {
		if err := putOrDelete(ctx, tx, []byte(key), set, len(set.GetSegments()) == 0); err != nil {
			return err
		}
	}
	if err := putOrDelete(ctx, tx, pkey, next, len(next.GetRefs()) == 0); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DropPath removes every segment of path in one write.
func (x *Index) DropPath(ctx context.Context, path string) error {
	return x.SetPath(ctx, path, nil, nil)
}

// loadSet returns the SegmentSet of ref, read from tx on its first use and
// kept in sets by key.
func loadSet(ctx context.Context, tx kvtx.Tx, sets map[string]*SegmentSet, ref *block.BlockRef) (*SegmentSet, error) {
	// Reuse the set this write already read.
	key, err := refKey(ref)
	if err != nil {
		return nil, err
	}
	if set, ok := sets[string(key)]; ok {
		return set, nil
	}

	// Read and decode it.
	set := &SegmentSet{}
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if found {
		if err := set.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	sets[string(key)] = set
	return set, nil
}

// putOrDelete writes msg at key, or deletes key when drop is set.
func putOrDelete(ctx context.Context, tx kvtx.Tx, key []byte, msg interface{ MarshalVT() ([]byte, error) }, drop bool) error {
	// Delete an emptied entry.
	if drop {
		return tx.Delete(ctx, key)
	}

	// Write the encoded entry.
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, key, data)
}

// refKey returns the key of ref's SegmentSet.
func refKey(ref *block.BlockRef) ([]byte, error) {
	data, err := ref.MarshalVT()
	if err != nil {
		return nil, err
	}
	return append([]byte(refKeyPrefix), data...), nil
}
