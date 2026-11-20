// Package block_store_filesegment serves blocks from byte ranges of files on
// this Device. A range is a block when encoding its bytes with the Space's
// transform hashes to the block's ref; the store checks that on every read.
package block_store_filesegment

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"strings"

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

// versionKeyPrefix prefixes the key of a two-way linked folder's
// FolderVersion of one file: the prefix, the folder, a zero byte, and the
// file's path relative to the folder with its components joined by slashes.
// A file name holds neither a zero byte nor a slash, so the versions of one
// directory's files and subdirectories share a key prefix.
const versionKeyPrefix = "v/"

// Index is the SegmentIndex of one Space: one entry per ref and path, holding
// the linked file range that held the ref's block, and one entry per path
// holding the refs of its ranges, the file-system identity of each linked
// folder's root, and the FolderVersion of each file of a two-way linked
// folder. It lives in a key/value store on this Device, so it outlives the
// process.
//
// A ref's ranges are the keys under its prefix, so linking or dropping one
// path writes only that path's entries, however many other files share its
// blocks.
type Index struct {
	// store holds the entries.
	store kvtx.Store
	// changed runs in each SetPath that changes the refs of a path, or is nil.
	changed RefsHook
}

// RefsHook runs inside the write transaction of a SetPath that changes the
// refs of a path. gained holds the refs the path holds after the write and did
// not hold before, in path order; lost holds the refs the path held before
// that no path holds after the write. It may write to tx keys outside the
// Index's prefixes, which commit with the path. It returns a func to run after
// the commit, or nil; when the hook fails, the SetPath fails and writes
// nothing.
type RefsHook func(ctx context.Context, tx kvtx.Tx, gained, lost []*block.BlockRef) (func(), error)

// NewIndex constructs the Index kept in store, which holds no other keys
// than the Index's own and those changed writes. changed may be nil.
func NewIndex(store kvtx.Store, changed RefsHook) *Index {
	return &Index{store: store, changed: changed}
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
// reads the path's entry and touches only the path's own keys, plus, with a
// RefsHook, one seek per dropped ref to learn whether another path holds it.
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

	// Write the new segments, one per ref, noting the refs the path gained.
	held := make(map[string]struct{}, len(old.GetRefs()))
	if x.changed != nil {
		for _, ref := range old.GetRefs() {
			key, err := segKey(ref, path)
			if err != nil {
				return err
			}
			held[string(key)] = struct{}{}
		}
	}
	var gained []*block.BlockRef
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
		if _, ok := held[string(key)]; x.changed != nil && !ok {
			gained = append(gained, ref)
		}
	}

	// Delete the old segments the new ones did not replace, noting the refs
	// no path holds any more.
	var lost []*block.BlockRef
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
		if x.changed == nil {
			continue
		}
		indexed, err := refIndexed(ctx, tx, ref)
		if err != nil {
			return err
		}
		if !indexed {
			lost = append(lost, ref)
		}
	}

	// Write the path and what the hook writes for its changed refs, then
	// commit and run the hook's follow-up.
	if err := putOrDelete(ctx, tx, pkey, next, len(next.GetRefs()) == 0); err != nil {
		return err
	}
	var after func()
	if len(gained) != 0 || len(lost) != 0 {
		if after, err = x.changed(ctx, tx, gained, lost); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if after != nil {
		after()
	}
	return nil
}

// Refs returns up to limit refs that have a segment, each once, in key order,
// starting after the ref after, or at the first when after is nil. It reads
// one transaction and decodes only keys, so a caller pages through every ref
// with no transaction held between pages.
func (x *Index) Refs(ctx context.Context, after *block.BlockRef, limit int) ([]*block.BlockRef, error) {
	// Position the iterator at after's keys, or at the first key.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, []byte(segKeyPrefix), true, false)
	defer it.Close()
	var skip []byte
	ok := false
	if after != nil {
		if skip, err = refPrefix(after); err != nil {
			return nil, err
		}
		if err := it.Seek(skip); err != nil {
			return nil, err
		}
		ok = it.Valid()
	} else {
		ok = it.Next()
	}

	// Take each ref's prefix once, skipping its other paths' keys.
	var refs []*block.BlockRef
	for ; ok && len(refs) < limit; ok = it.Next() {
		key := it.Key()
		if skip != nil && bytes.HasPrefix(key, skip) {
			continue
		}
		rest := key[len(segKeyPrefix):]
		n, w := binary.Uvarint(rest)
		if w <= 0 || uint64(len(rest)-w) < n { //nolint:gosec // w is at most len(rest).
			return nil, errors.Errorf("invalid segment key %x", key)
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(rest[w : w+int(n)]); err != nil { //nolint:gosec // n is at most len(rest).
			return nil, err
		}
		refs = append(refs, ref)
		skip = slices.Clone(key[:len(segKeyPrefix)+w+int(n)]) //nolint:gosec // n is at most len(rest).
	}
	return refs, it.Err()
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

// FolderVersions returns the versions recorded for the files directly in the
// directory dir, a path relative to the linked folder at folder, by name, and
// the names of its subdirectories holding recorded versions. It reads one key
// per file and one per subdirectory, seeking past each subdirectory's keys.
func (x *Index) FolderVersions(ctx context.Context, folder string, dir []string) (map[string]*FolderVersion, []string, error) {
	// Iterate the directory's keys in order.
	prefix := versionDirPrefix(folder, dir)
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Take each file's version, and each subdirectory's name once.
	files := make(map[string]*FolderVersion)
	var dirs []string
	for ok := it.Next(); ok; {
		rest := string(it.Key()[len(prefix):])
		if name, _, sub := strings.Cut(rest, "/"); sub {
			dirs = append(dirs, name)
			if err := it.Seek(append(append(slices.Clone(prefix), name...), '/'+1)); err != nil {
				return nil, nil, err
			}
			ok = it.Valid()
			continue
		}
		data, err := it.Value()
		if err != nil {
			return nil, nil, err
		}
		v := &FolderVersion{}
		if err := v.UnmarshalVT(data); err != nil {
			return nil, nil, err
		}
		files[rest] = v
		ok = it.Next()
	}
	return files, dirs, it.Err()
}

// FolderVersionsUnder calls cb with the path relative to the linked folder at
// folder, and the version, of each file recorded under the directory dir, at
// any depth, in one scan.
func (x *Index) FolderVersionsUnder(ctx context.Context, folder string, dir []string, cb func(rel []string, v *FolderVersion) error) error {
	// Scan the directory's keys.
	prefix := versionDirPrefix(folder, dir)
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Decode each version and its path.
	base := len(versionDirPrefix(folder, nil))
	return tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		v := &FolderVersion{}
		if err := v.UnmarshalVT(value); err != nil {
			return err
		}
		return cb(strings.Split(string(key[base:]), "/"), v)
	})
}

// SetFolderVersions records the versions in set, by path relative to the
// linked folder at folder with its components joined by slashes, and drops
// the versions at the paths drop, in one write.
func (x *Index) SetFolderVersions(ctx context.Context, folder string, set map[string]*FolderVersion, drop [][]string) error {
	// Skip an empty write.
	if len(set) == 0 && len(drop) == 0 {
		return nil
	}
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Drop, then set, so a path in both keeps its new version.
	for _, rel := range drop {
		if err := tx.Delete(ctx, versionKey(folder, strings.Join(rel, "/"))); err != nil {
			return err
		}
	}
	for rel, v := range set {
		if err := putOrDelete(ctx, tx, versionKey(folder, rel), v, false); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// versionDirPrefix returns the key prefix of the versions under the directory
// dir of the linked folder at folder.
func versionDirPrefix(folder string, dir []string) []byte {
	key := []byte(versionKeyPrefix + folder + "\x00")
	for _, name := range dir {
		key = append(append(key, name...), '/')
	}
	return key
}

// versionKey returns the key of the version of the file at rel, joined by
// slashes, in the linked folder at folder.
func versionKey(folder, rel string) []byte {
	return []byte(versionKeyPrefix + folder + "\x00" + rel)
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

// refIndexed reports whether tx holds a segment of ref in any path, with one
// seek of the ref's prefix.
func refIndexed(ctx context.Context, tx kvtx.Tx, ref *block.BlockRef) (bool, error) {
	// Seek the first key under the ref's prefix.
	prefix, err := refPrefix(ref)
	if err != nil {
		return false, err
	}
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	found := it.Next()
	return found, it.Err()
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
