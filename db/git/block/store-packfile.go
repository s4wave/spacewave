package git_block

import (
	"bytes"
	"cmp"
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

// PackfileWriter returns a writer that stores an incoming Git packfile.
//
// The pack streams into a chunked blob as it arrives, so the writer holds a
// few chunks, not the pack. Close indexes the stored blob by reading it back
// and attaches the pack under its checksum. A writer that fails before Close
// leaves only unreferenced blocks in the transaction. Writes run on a builder
// goroutine that owns the Store's transaction until Close returns, so the
// caller must not use the Store between PackfileWriter and Close.
func (r *Store) PackfileWriter() (io.WriteCloser, error) {
	// Require the packfile tree that will hold the pack.
	if r.packTree == nil {
		return nil, errors.New("packfile tree is unavailable")
	}
	opts, err := r.packBlobBuildOpts()
	if err != nil {
		return nil, err
	}

	// Build the pack blob under detached metadata while the caller writes.
	packCs := r.packTree.GetCursor().Detach(false)
	packCs.ClearAllRefs()
	pack := &Packfile{}
	packCs.SetBlock(pack, true)
	pr, pw := io.Pipe()
	w := &storePackfileWriter{store: r, opts: opts, packCs: packCs, pack: pack, pw: pw, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		pack.PackBlob, w.buildErr = blob.BuildBlobWithReader(r.ctx, pr, packCs.FollowSubBlock(1), opts)
		// Fail the caller's next Write instead of blocking it on a dead reader.
		_ = pr.CloseWithError(cmp.Or(w.buildErr, io.ErrClosedPipe))
	}()
	return w, nil
}

// ObjectPacks returns hashes of object packs in this store.
func (r *Store) ObjectPacks() ([]plumbing.Hash, error) {
	// Return no pack hashes when the Store has no packfile tree.
	if r.packTree == nil {
		return nil, nil
	}

	// Read the stored packfile hashes through the metadata iterator.
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

	// Report any failure while traversing the packfile metadata.
	if err := it.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteOldObjectPackAndIndex deletes an object pack and index if old enough.
func (r *Store) DeleteOldObjectPackAndIndex(ph plumbing.Hash, t time.Time) error {
	// Keep packfiles when the caller supplied an age threshold.
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

func (r *Store) lookupPackedObject(ot plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	// Report a missing object when the Store has no packfile tree.
	if r.packTree == nil {
		return nil, plumbing.ErrObjectNotFound
	}

	// Search stored packfiles for the requested Git object.
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

	// Report any failure while searching the packfile metadata.
	if err := it.Err(); err != nil {
		return nil, err
	}
	return nil, plumbing.ErrObjectNotFound
}

func (r *Store) iterPackedObjects(ot plumbing.ObjectType, seen map[plumbing.Hash]struct{}) ([]plumbing.EncodedObject, error) {
	// Return no packed objects when the Store has no packfile tree.
	if r.packTree == nil {
		return nil, nil
	}

	// Collect objects from the stored packfiles.
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
			// Ignore packed Git objects already encountered in another storage location.
			h := obj.Hash()
			if _, ok := seen[h]; ok {
				return nil
			}

			// Retain each newly encountered packed Git object.
			seen[h] = struct{}{}
			out = append(out, obj)
			return nil
		})
		packIter.Close()
		if err != nil {
			return nil, err
		}
	}

	// Report any failure while traversing the packfile metadata.
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

	// Retain the decoded pack index for later Store lookups.
	entry := &storePackCacheEntry{hash: packHash, idx: idx}
	r.packCache[packHash] = entry
	return entry, nil
}

func (r *Store) packBlobBuildOpts() (*blob.BuildBlobOpts, error) {
	// Prepare the object store chunker for packfile blobs.
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
	// Decode the cursor and require a packfile metadata block.
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

// storePackfileWriter streams one incoming pack into the Store.
type storePackfileWriter struct {
	store  *Store
	opts   *blob.BuildBlobOpts
	packCs *block.Cursor
	pack   *Packfile
	// pw feeds the builder goroutine, which closes done when the pack blob is
	// built or has failed with buildErr.
	pw       *io.PipeWriter
	done     chan struct{}
	buildErr error
}

// storePackCacheEntry keeps an index while its data reader enters and leaves the LRU.
type storePackCacheEntry struct {
	// hash identifies the pack and its blobs.
	hash plumbing.Hash
	// idx remains available after the data reader is evicted.
	idx *idxfile.MemoryIndex
	// pack is the open data reader, when retained.
	pack *go_git_packfile.Packfile
	// recent locates pack in the open reader LRU.
	recent *list.Element
}

// Write passes p to the pack blob builder.
func (w *storePackfileWriter) Write(p []byte) (int, error) {
	return w.pw.Write(p)
}

// Close finishes the pack blob, indexes it and attaches it to the Store.
func (w *storePackfileWriter) Close() error {
	// End the stream and wait for the builder to release the transaction.
	_ = w.pw.Close()
	<-w.done
	if w.buildErr != nil {
		return w.buildErr
	}
	if err := w.store.ctx.Err(); err != nil {
		return err
	}
	if w.pack.GetPackBlob().GetTotalSize() == 0 {
		return go_git_packfile.ErrEmptyPackfile
	}
	clearBlobChunkerArgs(w.pack.PackBlob)

	// Index the stored pack through a seekable reader: the parser rereads
	// objects from the blob instead of holding them.
	idx, err := w.indexPack()
	if err != nil {
		return err
	}
	var idxBuf bytes.Buffer
	if err := idxfile.Encode(&idxBuf, hash.New(crypto.SHA1), idx); err != nil {
		return err
	}

	// Read the pack checksum and object count from the index.
	r := w.store
	packHash := idx.PackfileChecksum
	packStoreHash, err := NewHash(packHash)
	if err != nil {
		return err
	}
	count, err := idx.Count()
	if err != nil {
		return err
	}

	// Fill the metadata from the index and the stored sizes.
	w.pack.PackHash = packStoreHash
	w.pack.ObjectCount = uint64(count) //nolint:gosec
	w.pack.PackSize = w.pack.GetPackBlob().GetTotalSize()
	w.pack.IdxSize = uint64(idxBuf.Len()) //nolint:gosec

	// Discard the cached reader and index for a replaced pack.
	if entry := r.packCache[packHash]; entry != nil {
		if entry.pack != nil {
			if err := entry.pack.Close(); err != nil {
				return err
			}
			r.packLRU.Remove(entry.recent)
		}
		delete(r.packCache, packHash)
	}

	// Store the metadata under the pack checksum, then its index blob.
	key := slices.Clone(packHash.Bytes())
	if err := r.packTree.SetCursorAtKey(r.ctx, key, w.packCs, false); err != nil {
		return err
	}
	w.pack.IdxBlob, err = blob.BuildBlob(
		r.ctx,
		int64(idxBuf.Len()),
		&idxBuf,
		w.packCs.FollowSubBlock(2),
		w.opts,
	)
	if err != nil {
		return err
	}
	clearBlobChunkerArgs(w.pack.IdxBlob)
	return w.pack.Validate()
}

// indexPack parses the stored pack blob and returns its object index.
func (w *storePackfileWriter) indexPack() (*idxfile.MemoryIndex, error) {
	// Open the stored blob as a seekable pack file.
	file, err := NewPackfileFile(w.store.ctx, "pack-incoming.pack", w.packCs.FollowSubBlock(1))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Scan the pack, letting the index writer observe each object.
	idxWriter := &idxfile.Writer{}
	parser := go_git_packfile.NewParser(file, go_git_packfile.WithScannerObservers(idxWriter))
	if _, err := parser.Parse(); err != nil {
		return nil, err
	}
	return idxWriter.Index()
}

// _ is a type assertion
var (
	_ storer.PackfileWriter     = (*Store)(nil)
	_ storer.PackedObjectStorer = (*Store)(nil)
)
