// Package block_store_filesegment serves blocks from byte ranges of files on
// this Device. A range is a block when encoding its bytes with the Space's
// transform hashes to the block's ref; the store checks that on every read.
package block_store_filesegment

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// segKeyPrefix prefixes the key of one ref's range in one path.
const segKeyPrefix = "r/"

// pathKeyPrefix prefixes the key of a path's PathRefs.
const pathKeyPrefix = "p/"

// rootKeyPrefix prefixes the key of a linked folder's root identity.
const rootKeyPrefix = "f/"

// Index is the SegmentIndex of one Space: one entry per ref and path, holding
// the linked file range that held the ref's block, and one entry per path
// holding the refs of its ranges, and the file-system identity of each linked
// folder's root. It lives in a key/value store on this
// Device, so it outlives the process.
//
// A ref's ranges are the keys under its prefix, so linking or dropping one
// path writes only that path's entries, however many other files share its
// blocks.
type Index struct {
	// store holds the entries.
	store kvtx.Store
}

// NewIndex constructs the Index kept in store, which holds no other keys.
func NewIndex(store kvtx.Store) *Index {
	return &Index{store: store}
}

// Lookup returns the segments recorded for ref, ordered by path, with one scan
// of the ref's prefix.
func (x *Index) Lookup(ctx context.Context, ref *block.BlockRef) ([]*Segment, error) {
	// Scan the ref's entries.
	prefix, err := refPrefix(ref)
	if err != nil {
		return nil, err
	}
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Decode each range.
	var segs []*Segment
	err = tx.ScanPrefix(ctx, prefix, func(_, value []byte) error {
		seg := &Segment{}
		if err := seg.UnmarshalVT(value); err != nil {
			return err
		}
		segs = append(segs, seg)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return segs, nil
}

// SetPath replaces the segments of path with segs, where segs[i] held refs[i],
// in one write: a reader sees the path's old segments or its new ones. A ref
// keeps one segment per path, the first. Nil refs drop the path. The write
// reads the path's entry and touches only the path's own keys.
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

	// Write the new segments, one per ref.
	next := &PathRefs{}
	keep := make(map[string]struct{}, len(refs))
	for i, ref := range refs {
		key, err := segKey(ref, path)
		if err != nil {
			return err
		}
		if _, ok := keep[string(key)]; ok {
			continue
		}
		keep[string(key)] = struct{}{}
		if err := putOrDelete(ctx, tx, key, segs[i], false); err != nil {
			return err
		}
		next.Refs = append(next.Refs, ref)
	}

	// Delete the old segments the new ones did not replace.
	for _, ref := range old.GetRefs() {
		key, err := segKey(ref, path)
		if err != nil {
			return err
		}
		if _, ok := keep[string(key)]; ok {
			continue
		}
		if err := tx.Delete(ctx, key); err != nil {
			return err
		}
	}

	// Write the path, then commit.
	if err := putOrDelete(ctx, tx, pkey, next, len(next.GetRefs()) == 0); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FolderRoot returns the file-system identity recorded for the root of the
// linked folder at folder, and whether one is recorded.
func (x *Index) FolderRoot(ctx context.Context, folder string) (uint64, bool, error) {
	// Read the folder's entry.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return 0, false, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(rootKeyPrefix+folder))
	if err != nil || !found {
		return 0, false, err
	}

	// Decode the identity.
	id, n := binary.Uvarint(data)
	if n <= 0 {
		return 0, false, errors.Errorf("invalid root identity of folder %q", folder)
	}
	return id, true, nil
}

// SetFolderRoot records id as the file-system identity of the root of the
// linked folder at folder.
func (x *Index) SetFolderRoot(ctx context.Context, folder string, id uint64) error {
	// Write the folder's entry in a transaction of its own.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(rootKeyPrefix+folder), binary.AppendUvarint(nil, id)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DropPath removes every segment of path in one write.
func (x *Index) DropPath(ctx context.Context, path string) error {
	return x.SetPath(ctx, path, nil, nil)
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

// refPrefix returns the prefix of ref's segment keys: the encoded ref behind
// its length, so no ref's prefix is a prefix of another ref's.
func refPrefix(ref *block.BlockRef) ([]byte, error) {
	data, err := ref.MarshalVT()
	if err != nil {
		return nil, err
	}
	key := binary.AppendUvarint([]byte(segKeyPrefix), uint64(len(data)))
	return append(key, data...), nil
}

// segKey returns the key of ref's segment in path.
func segKey(ref *block.BlockRef, path string) ([]byte, error) {
	prefix, err := refPrefix(ref)
	if err != nil {
		return nil, err
	}
	return append(prefix, path...), nil
}
