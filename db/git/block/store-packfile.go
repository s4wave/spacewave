package git_block

import (
	"bytes"
	"container/list"
	"context"
	"crypto"
	"io"
	"slices"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	go_git_packfile "github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// openPackReaderLimit caps retained chunk data near 32 MiB (four readers at
// packfileChunkCacheLimit) while decoded indexes stay cached.
const openPackReaderLimit = 4

// PackfileWriter returns a writer for preserving an incoming Git packfile.
func (r *Store) PackfileWriter() (io.WriteCloser, error) {
	return &storePackfileWriter{store: r}, nil
}

// ObjectPacks returns hashes of object packs in this store.
func (r *Store) ObjectPacks() ([]plumbing.Hash, error) {
	if r.packTree == nil {
		return nil, nil
	}
	out := make([]plumbing.Hash, 0)
	it := r.packTree.BlockIterate(r.ctx, nil, false, false)
	defer it.Close()
	for it.Next() {
		pack, err := unmarshalPackfileCursor(r.ctx, it.ValueCursor())
		if err != nil {
			return nil, err
		}
		h, err := FromHash(pack.GetPackHash())
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteOldObjectPackAndIndex deletes an object pack and index if old enough.
func (r *Store) DeleteOldObjectPackAndIndex(ph plumbing.Hash, t time.Time) error {
	if !t.IsZero() {
		return nil
	}
	// Release the cached reader before dropping its metadata.
	if entry := r.packCache[ph]; entry != nil {
		if entry.pack != nil {
			if err := entry.pack.Close(); err != nil {
				return err
			}
			r.packLRU.Remove(entry.recent)
		}
		delete(r.packCache, ph)
	}
	return r.packTree.Delete(r.ctx, slices.Clone(ph.Bytes()))
}

func (r *Store) setPackfile(packData []byte, idxData []byte, idx *idxfile.MemoryIndex) error {
	if len(packData) == 0 {
		return go_git_packfile.ErrEmptyPackfile
	}
	if r.packTree == nil {
		return errors.New("packfile tree is unavailable")
	}

	packHash := idx.PackfileChecksum
	packStoreHash, err := NewHash(packHash)
	if err != nil {
		return err
	}
	count, err := idx.Count()
	if err != nil {
		return err
	}

	key := slices.Clone(packHash.Bytes())
	if entry := r.packCache[packHash]; entry != nil {
		if entry.pack != nil {
			if err := entry.pack.Close(); err != nil {
				return err
			}
			r.packLRU.Remove(entry.recent)
		}
		delete(r.packCache, packHash)
	}
	packCs := r.packTree.GetCursor().Detach(false)
	packCs.ClearAllRefs()
	pack := &Packfile{
		PackHash:    packStoreHash,
		ObjectCount: uint64(count),         //nolint:gosec
		PackSize:    uint64(len(packData)), //nolint:gosec
		IdxSize:     uint64(len(idxData)),  //nolint:gosec
	}
	packCs.SetBlock(pack, true)

	if err := r.packTree.SetCursorAtKey(r.ctx, key, packCs, false); err != nil {
		return err
	}

	opts, err := r.packBlobBuildOpts()
	if err != nil {
		return err
	}
	pack.PackBlob, err = blob.BuildBlob(
		r.ctx,
		int64(len(packData)),
		bytes.NewReader(packData),
		packCs.FollowSubBlock(1),
		opts,
	)
	if err != nil {
		return err
	}
	clearBlobChunkerArgs(pack.PackBlob)

	pack.IdxBlob, err = blob.BuildBlob(
		r.ctx,
		int64(len(idxData)),
		bytes.NewReader(idxData),
		packCs.FollowSubBlock(2),
		opts,
	)
	if err != nil {
		return err
	}
	clearBlobChunkerArgs(pack.IdxBlob)

	return pack.Validate()
}

func (r *Store) lookupPackedObject(ot plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	if r.packTree == nil {
		return nil, plumbing.ErrObjectNotFound
	}
	it := r.packTree.BlockIterate(r.ctx, nil, false, false)
	defer it.Close()
	for it.Next() {
		obj, err := r.lookupPackedObjectInCursor(it.ValueCursor(), h)
		if err != nil {
			if err == plumbing.ErrObjectNotFound {
				continue
			}
			return nil, err
		}
		if ot != plumbing.AnyObject && ot != 0 && obj.Type() != ot {
			return nil, plumbing.ErrObjectNotFound
		}
		return obj, nil
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return nil, plumbing.ErrObjectNotFound
}

func (r *Store) iterPackedObjects(ot plumbing.ObjectType, seen map[plumbing.Hash]struct{}) ([]plumbing.EncodedObject, error) {
	if r.packTree == nil {
		return nil, nil
	}
	out := make([]plumbing.EncodedObject, 0)
	it := r.packTree.BlockIterate(r.ctx, nil, false, false)
	defer it.Close()
	for it.Next() {
		pack, err := r.buildPackfileReader(it.ValueCursor())
		if err != nil {
			return nil, err
		}
		packIter, err := pack.GetByType(ot)
		if err != nil {
			return nil, err
		}
		err = packIter.ForEach(func(obj plumbing.EncodedObject) error {
			h := obj.Hash()
			if _, ok := seen[h]; ok {
				return nil
			}
			seen[h] = struct{}{}
			out = append(out, obj)
			return nil
		})
		packIter.Close()
		if err != nil {
			return nil, err
		}
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// lookupPackedObjectInCursor checks a pack's index before opening its data reader.
func (r *Store) lookupPackedObjectInCursor(cs *block.Cursor, h plumbing.Hash) (plumbing.EncodedObject, error) {
	// Decode or reuse the pack index to test membership without reading pack data.
	entry, err := r.packCacheEntry(cs)
	if err != nil {
		return nil, err
	}
	contains, err := entry.idx.Contains(h)
	if err != nil {
		return nil, err
	}
	if !contains {
		return nil, plumbing.ErrObjectNotFound
	}

	// Open only the pack that contains the object.
	pack, err := r.openPackfileReader(entry, cs)
	if err != nil {
		return nil, err
	}
	return pack.Get(h)
}

// buildPackfileReader returns a reader for a pack selected by its cursor.
func (r *Store) buildPackfileReader(cs *block.Cursor) (*go_git_packfile.Packfile, error) {
	entry, err := r.packCacheEntry(cs)
	if err != nil {
		return nil, err
	}
	return r.openPackfileReader(entry, cs)
}

// openPackfileReader retains a bounded set of open pack data readers.
func (r *Store) openPackfileReader(entry *storePackCacheEntry, cs *block.Cursor) (*go_git_packfile.Packfile, error) {
	// Reuse an open reader and mark it as recently used.
	if entry.pack != nil {
		r.packLRU.MoveToFront(entry.recent)
		return entry.pack, nil
	}

	// Attach the shared object cache and the pack's decoded index to a new reader.
	file, err := NewPackfileFile(r.ctx, "pack-"+entry.hash.String()+".pack", cs.FollowSubBlock(1))
	if err != nil {
		return nil, err
	}
	packReader := go_git_packfile.NewPackfile(
		file,
		go_git_packfile.WithIdx(entry.idx),
		go_git_packfile.WithCache(r.objectCache),
		go_git_packfile.WithObjectIDSize(entry.hash.Size()),
	)
	entry.pack = packReader
	entry.recent = r.packLRU.PushFront(entry)

	// Close the least recently used data reader when the retained set is full.
	if r.packLRU.Len() > openPackReaderLimit {
		oldest := r.packLRU.Back()
		old := oldest.Value.(*storePackCacheEntry)
		closeErr := old.pack.Close()
		old.pack = nil
		r.packLRU.Remove(oldest)
		old.recent = nil
		if closeErr != nil {
			return nil, closeErr
		}
	}
	return packReader, nil
}

// packCacheEntry decodes a pack index once for the Store lifetime.
func (r *Store) packCacheEntry(cs *block.Cursor) (*storePackCacheEntry, error) {
	// Read the pack identity and reuse its cached index when available.
	pack, err := unmarshalPackfileCursor(r.ctx, cs)
	if err != nil {
		return nil, err
	}
	packHash, err := FromHash(pack.GetPackHash())
	if err != nil {
		return nil, err
	}
	if entry := r.packCache[packHash]; entry != nil {
		return entry, nil
	}

	// Decode the index without materializing its backing blob.
	idxReader, err := NewPackfileFile(r.ctx, "idx-"+packHash.String()+".idx", cs.FollowSubBlock(2))
	if err != nil {
		return nil, err
	}
	defer idxReader.Close()
	idx := idxfile.NewMemoryIndex(packHash.Size())
	if err := idxfile.NewDecoder(idxReader, hash.New(crypto.SHA1)).Decode(idx); err != nil {
		return nil, err
	}

	entry := &storePackCacheEntry{hash: packHash, idx: idx}
	r.packCache[packHash] = entry
	return entry, nil
}

func (r *Store) packBlobBuildOpts() (*blob.BuildBlobOpts, error) {
	if r.root.EncodedObjectStore == nil {
		r.root.EncodedObjectStore = &EncodedObjectStore{}
	}
	chunkerArgs, err := r.root.EncodedObjectStore.getOrGenerateChunkerArgs()
	if err != nil {
		return nil, err
	}
	return &blob.BuildBlobOpts{ChunkerArgs: chunkerArgs}, nil
}

func unmarshalPackfileCursor(ctx context.Context, cs *block.Cursor) (*Packfile, error) {
	packi, err := cs.Unmarshal(ctx, NewPackfileBlock)
	if err != nil {
		return nil, err
	}
	pack, ok := packi.(*Packfile)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return pack, nil
}

func clearBlobChunkerArgs(bl *blob.Blob) {
	if bl == nil || bl.ChunkIndex == nil {
		return
	}
	bl.ChunkIndex.ChunkerArgs = nil
}

type storePackfileWriter struct {
	store *Store
	buf   bytes.Buffer
}

// storePackCacheEntry keeps an index while its data reader enters and leaves the LRU.
type storePackCacheEntry struct {
	// hash names the pack and its blobs.
	hash plumbing.Hash
	// idx remains available after the data reader is evicted.
	idx *idxfile.MemoryIndex
	// pack is the open data reader, when retained.
	pack *go_git_packfile.Packfile
	// recent locates pack in the open reader LRU.
	recent *list.Element
}

func (w *storePackfileWriter) Write(p []byte) (int, error) {
	if err := w.store.ctx.Err(); err != nil {
		return 0, err
	}
	return w.buf.Write(p)
}

func (w *storePackfileWriter) Close() error {
	if err := w.store.ctx.Err(); err != nil {
		return err
	}
	idxWriter := &idxfile.Writer{}
	parser := go_git_packfile.NewParser(
		bytes.NewReader(w.buf.Bytes()),
		go_git_packfile.WithScannerObservers(idxWriter),
		go_git_packfile.WithHighMemoryMode(),
	)
	if _, err := parser.Parse(); err != nil {
		return err
	}
	idx, err := idxWriter.Index()
	if err != nil {
		return err
	}

	var idxBuf bytes.Buffer
	if err := idxfile.Encode(&idxBuf, hash.New(crypto.SHA1), idx); err != nil {
		return err
	}
	return w.store.setPackfile(w.buf.Bytes(), idxBuf.Bytes(), idx)
}

// _ is a type assertion
var (
	_ storer.PackfileWriter     = (*Store)(nil)
	_ storer.PackedObjectStorer = (*Store)(nil)
)
