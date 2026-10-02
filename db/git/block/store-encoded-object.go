package git_block

import (
	"bytes"
	"io"

	"github.com/aperturerobotics/util/iocloser"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// StoreEncodedObject is an encoded object attached to a store.
type StoreEncodedObject struct {
	// r owns the object storage and context.
	r *Store
	// bcs is the block cursor, nil if not attached to tree.
	bcs *block.Cursor
	// fetched indicates if the blob has been fetched to buf or not.
	fetched bool
	// buf contains the fetched data.
	buf bytes.Buffer
	// objType is the object type, pending write.
	objType plumbing.ObjectType
	// size is the expected size, or zero unless SetSize is called.
	size int64
}

// NewStoreEncodedObject constructs a new StoreEncodedObject.
// A nil bcs indicates that the object has not been committed.
func NewStoreEncodedObject(r *Store, bcs *block.Cursor) *StoreEncodedObject {
	return &StoreEncodedObject{r: r, bcs: bcs}
}

// NewEncodedObject returns a new plumbing.EncodedObject, the real type
// of the object can be a custom implementation or the default one,
// plumbing.MemoryObject.
func (r *Store) NewEncodedObject() plumbing.EncodedObject {
	return NewStoreEncodedObject(r, nil)
}

// rawObjectCloser wraps a writer to store the object on close.
type rawObjectCloser struct {
	store  *Store
	obj    plumbing.EncodedObject
	closer io.Closer
}

// Close closes the writer and stores the object.
func (c *rawObjectCloser) Close() error {
	if err := c.closer.Close(); err != nil {
		return err
	}
	_, err := c.store.SetEncodedObject(c.obj)
	return err
}

// RawObjectWriter returns a writer for writing a raw object.
func (r *Store) RawObjectWriter(typ plumbing.ObjectType, sz int64) (io.WriteCloser, error) {
	// Initialize the object and open its data writer.
	obj := r.NewEncodedObject()
	obj.SetType(typ)
	obj.SetSize(sz)
	w, err := obj.Writer()
	if err != nil {
		return nil, err
	}

	// Store the completed object when the caller closes the writer.
	return &struct {
		io.Writer
		io.Closer
	}{
		Writer: w,
		Closer: &rawObjectCloser{store: r, obj: obj, closer: w},
	}, nil
}

// SetEncodedObject saves an object into the storage.
func (r *Store) SetEncodedObject(eoi plumbing.EncodedObject) (plumbing.Hash, error) {
	// Copy objects whose data is not already buffered by this store.
	var h plumbing.Hash
	origHash := (*plumbing.Hash)(nil)
	eo, ok := eoi.(*StoreEncodedObject)
	if !ok || eo.r != r || eo.bcs != nil || !eo.fetched {
		eoh := eoi.Hash()
		origHash = &eoh
		eo = r.NewEncodedObject().(*StoreEncodedObject)
		eo.SetType(eoi.Type())
		rc, err := eoi.Reader()
		if err != nil {
			return h, err
		}
		defer rc.Close()
		eo.fetched = true
		wc, err := eo.Writer()
		if err != nil {
			return h, err
		}
		if _, err := io.Copy(wc, rc); err != nil {
			return h, err
		}
	}

	// Preserve the source hash when converting between storage implementations.
	h = eo.Hash()
	if origHash != nil {
		if !bytes.Equal(h.Bytes(), origHash.Bytes()) {
			return h, errors.Errorf(
				"hash mismatch when converting: %v != expected %v",
				(*origHash).String(),
				h.String(),
			)
		}
	}

	// Validate the declared plaintext size before writing blocks.
	writeBuf := &eo.buf
	writeLen := int64(writeBuf.Len())
	if eo.size != 0 && eo.size != writeLen {
		return h, errors.Wrapf(
			ErrSizeInvalid,
			"expected %d got %d",
			eo.size, writeLen,
		)
	}

	// Reuse the object store's chunking parameters for the data blob.
	buildBlobOpts := &blob.BuildBlobOpts{}
	if r.root.EncodedObjectStore == nil {
		r.root.EncodedObjectStore = &EncodedObjectStore{}
	}
	var err error
	buildBlobOpts.ChunkerArgs, err = r.root.EncodedObjectStore.getOrGenerateChunkerArgs()
	if err != nil {
		return h, err
	}

	// Resolve the object's tree key and transaction context.
	key, err := r.buildEncodedObjectKey(eo.Type(), h)
	if err != nil {
		return h, err
	}
	ctx := r.ctx

	// Bulk mode: write each object via a per-object mini-transaction,
	// accumulate refs for deferred IAVL tree construction at Commit.
	if r.storeOps != nil {
		tx, encObjCs := block.NewTransaction(r.storeOps, r.bulkXfrm, nil, r.bulkPutOpts)
		encObjCs.ClearAllRefs()
		encObjBlk := &EncodedObject{}
		encObjBlk.DataHash, err = NewHash(h)
		if err != nil {
			return h, err
		}
		encObjBlk.EncodedObjectType = NewEncodedObjectType(eo.Type())
		if err = encObjBlk.Validate(); err != nil {
			return h, err
		}
		encObjCs.SetBlock(encObjBlk, true)

		dataBlobCs := encObjCs.FollowRef(1, nil)
		encObjBlk.DataBlob, err = blob.BuildBlob(ctx, writeLen, writeBuf, dataBlobCs, buildBlobOpts)
		if err != nil {
			return h, err
		}
		if ci := encObjBlk.DataBlob.ChunkIndex; ci != nil {
			encObjBlk.DataBlob.ChunkIndex.ChunkerArgs = nil
		}

		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			return h, err
		}

		r.objIndex[h] = ref
		r.objKeys = append(r.objKeys, bulkEntry{key: key, ref: ref})

		eo.bcs = nil
		eo.fetched = false
		eo.buf.Reset()
		return h, nil
	}

	// Build the object metadata for insertion into the IAVL tree.
	encTree := r.objTree
	rootCursor := r.objTree.GetCursor()
	encObjCs := rootCursor.Detach(false)
	encObjCs.ClearAllRefs()
	encObjBlk := &EncodedObject{}
	encObjBlk.DataHash, err = NewHash(h)
	if err != nil {
		return h, err
	}
	encObjBlk.EncodedObjectType = NewEncodedObjectType(eo.Type())

	// Validate and insert the metadata before attaching its data blob.
	err = encObjBlk.Validate()
	if err != nil {
		return h, err
	}
	encObjCs.SetBlock(encObjBlk, true)
	err = encTree.SetCursorAtKey(ctx, key, encObjCs, false)
	if err != nil {
		return h, err
	}

	// Store the plaintext as a blob beneath the object metadata.
	dataBlobCs := encObjCs.FollowRef(1, nil)
	encObjBlk.DataBlob, err = blob.BuildBlob(
		ctx,
		writeLen,
		writeBuf,
		dataBlobCs,
		buildBlobOpts,
	)
	if err != nil {
		return h, err
	}
	if ci := encObjBlk.DataBlob.ChunkIndex; ci != nil {
		encObjBlk.DataBlob.ChunkIndex.ChunkerArgs = nil
	}

	// Release the plaintext buffer after its blocks have been attached.
	eo.bcs = encObjCs
	eo.fetched = false
	eo.buf.Reset()
	return h, nil
}

// EncodedObject gets an object by hash with the given plumbing.ObjectType.
// Implementors should return (nil, plumbing.ErrObjectNotFound) if an object
// doesn't exist with both the given hash and object type.
//
// Valid plumbing.ObjectType values are CommitObject, BlobObject, TagObject,
// TreeObject and AnyObject. If plumbing.AnyObject is given, the object must be
// looked up regardless of its type.
func (r *Store) EncodedObject(ot plumbing.ObjectType, oh plumbing.Hash) (plumbing.EncodedObject, error) {
	// Resolve untyped lookups through the hash-only interface.
	if ot == plumbing.AnyObject || ot == 0 {
		return r.EncodedObjectByHash(oh)
	}

	// Check bulk index for objects written but not yet committed to IAVL.
	if r.objIndex != nil {
		if cs := r.lookupBulkObject(oh); cs != nil {
			encObj, err := block.UnmarshalBlock[*EncodedObject](r.ctx, cs, NewEncodedObjectBlock)
			if err != nil {
				return nil, err
			}
			if EncodedObjectType(ot) != encObj.GetEncodedObjectType() {
				return nil, plumbing.ErrObjectNotFound
			}
			return NewStoreEncodedObject(r, cs), nil
		}
	}

	// Find a loose object by type, falling back to packed storage.
	key, err := r.buildEncodedObjectKey(ot, oh)
	if err != nil {
		return nil, err
	}
	encObj, encObjCs, err := r.lookupEncodedObject(key)
	if err != nil || encObj == nil {
		if err == plumbing.ErrObjectNotFound {
			return r.lookupPackedObject(ot, oh)
		}
		return nil, err
	}

	// Check the stored metadata against the requested hash and type.
	ph, err := FromHash(encObj.GetDataHash())
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(ph.Bytes(), oh.Bytes()) {
		return nil, errors.Wrapf(
			ErrHashMismatch,
			"expected %v got %v",
			oh.String(),
			ph.String(),
		)
	}
	encObjType := encObj.GetEncodedObjectType()
	if eot := EncodedObjectType(ot); eot != encObjType {
		return nil, errors.Errorf(
			"storage: expected object type %s but got %s",
			eot.String(),
			encObjType.String(),
		)
	}
	return NewStoreEncodedObject(r, encObjCs), nil
}

// EncodedObjectByHash looks up an encoded object by hash only.
// Returns plumbing.ErrObjectNotFound if not found.
func (r *Store) EncodedObjectByHash(ph plumbing.Hash) (plumbing.EncodedObject, error) {
	// Check bulk index for objects written but not yet committed to IAVL.
	if r.objIndex != nil {
		if cs := r.lookupBulkObject(ph); cs != nil {
			return NewStoreEncodedObject(r, cs), nil
		}
	}

	for i := EncodedObjectType(1); i <= EncodedObjectType_EncodedObjectType_MAX; i++ {
		encObj, err := r.EncodedObject(i.ToObjectType(), ph)
		if err != nil {
			if err != plumbing.ErrObjectNotFound {
				return nil, err
			}
		} else if encObj != nil {
			return encObj, nil
		}
	}
	return r.lookupPackedObject(plumbing.AnyObject, ph)
}

// IterEncodedObjects returns loose and packed objects of the requested type.
// AnyObject and InvalidObject include every type; duplicate hashes appear once.
func (r *Store) IterEncodedObjects(ph plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	// Open the loose-object iterator for the requested type.
	var prefix []byte
	if ph != plumbing.AnyObject && ph != plumbing.InvalidObject {
		prefix = []byte{byte(ph)} //nolint:gosec
	}
	treeTx := r.objTree
	ktxIterator := treeTx.BlockIterate(r.ctx, prefix, false, false)
	looseIter := NewEncodedObjectIter(r, ktxIterator)
	if r.packTree == nil {
		return looseIter, nil
	}

	// Collect loose objects and remember their hashes for packed-object deduplication.
	seen := make(map[plumbing.Hash]struct{})
	objects := make([]plumbing.EncodedObject, 0)
	err := looseIter.ForEach(func(obj plumbing.EncodedObject) error {
		seen[obj.Hash()] = struct{}{}
		objects = append(objects, obj)
		return nil
	})
	looseIter.Close()
	if err != nil {
		return nil, err
	}

	// Append packed objects that were not already returned by the loose store.
	packed, err := r.iterPackedObjects(ph, seen)
	if err != nil {
		return nil, err
	}
	objects = append(objects, packed...)
	return newSliceEncodedObjectIter(objects), nil
}

// HasEncodedObject checks loose-object keys and pack indexes without reading object data.
// It returns plumbing.ErrObjectNotFound when the hash is absent.
func (r *Store) HasEncodedObject(ph plumbing.Hash) error {
	// Include objects pending the bulk index commit.
	if _, ok := r.objIndex[ph]; ok {
		return nil
	}

	// Test loose-object index keys without materializing their blobs.
	for typ := EncodedObjectType(1); typ <= EncodedObjectType_EncodedObjectType_MAX; typ++ {
		key, err := r.buildEncodedObjectKey(typ.ToObjectType(), ph)
		if err != nil {
			return err
		}
		cs, err := r.objTree.GetCursorAtKey(r.ctx, key)
		if err != nil {
			return err
		}
		if cs != nil {
			return nil
		}
	}

	// Search pack indexes while keeping their data readers closed.
	if r.packTree == nil {
		return plumbing.ErrObjectNotFound
	}
	it := r.packTree.BlockIterate(r.ctx, nil, false, false)
	defer it.Close()
	for it.Next() {
		entry, err := r.packCacheEntry(it.ValueCursor())
		if err != nil {
			return err
		}
		contains, err := entry.idx.Contains(ph)
		if err != nil {
			return err
		}
		if contains {
			return nil
		}
	}

	// Propagate index traversal failures before reporting an absent hash.
	if err := it.Err(); err != nil {
		return err
	}
	return plumbing.ErrObjectNotFound
}

// EncodedObjectSize returns the plaintext size of the encoded object.
func (r *Store) EncodedObjectSize(ph plumbing.Hash) (int64, error) {
	encObj, err := r.EncodedObjectByHash(ph)
	if err != nil {
		return 0, err
	}
	return encObj.Size(), nil
}

// AddAlternate rejects alternate object stores, which this storage does not support.
func (r *Store) AddAlternate(remote string) error {
	return git.ErrAlternatePathNotSupported
}

// Hash returns the hash of the encoded object.
// Returns empty hash if any errors.
func (o *StoreEncodedObject) Hash() plumbing.Hash {
	h, _ := o.StoreHash()
	oh, _ := FromHash(h)
	return oh
}

// StoreHash returns the hash of the encoded object in storage format.
func (o *StoreEncodedObject) StoreHash() (*hash.Hash, error) {
	if o.fetched || o.bcs == nil {
		// Hash data that has not been written to object metadata.
		data := (&o.buf).Bytes()
		oh, _ := gitObjectHasher.Compute(o.objType, data)
		return NewHash(oh)
	}
	encObj, err := o.unmarshalEncodedObject()
	if err != nil {
		return nil, err
	}
	return encObj.GetDataHash(), nil
}

// Type returns the Git object type, or InvalidObject if metadata cannot be read.
func (o *StoreEncodedObject) Type() plumbing.ObjectType {
	// Prefer the cached type and reject objects without stored metadata.
	if o.objType != 0 {
		return o.objType
	}
	if o.bcs == nil {
		return plumbing.InvalidObject
	}

	// Cache the type from the stored object metadata.
	encObjBlk, err := o.unmarshalEncodedObject()
	if err != nil {
		return plumbing.InvalidObject
	}
	o.objType = encObjBlk.GetEncodedObjectType().ToObjectType()
	return o.objType
}

// SetType sets the git object type.
func (o *StoreEncodedObject) SetType(ot plumbing.ObjectType) {
	o.objType = ot
}

// Size returns the size of the encoded object.
// Note: this is the size of the stored data, not the block graph.
func (o *StoreEncodedObject) Size() int64 {
	if o.fetched || o.bcs == nil {
		return int64((&o.buf).Len())
	}
	encObjBlk, err := o.unmarshalEncodedObject()
	if err != nil {
		return 0
	}
	return int64(encObjBlk.GetDataBlob().GetTotalSize()) //nolint:gosec
}

// SetSize sets the total expected size of the object data.
func (o *StoreEncodedObject) SetSize(s int64) {
	if s < 0 {
		s = 0
	}
	o.size = s
}

// Reader returns the data reader.
func (o *StoreEncodedObject) Reader() (io.ReadCloser, error) {
	// Read buffered data directly and reject objects with no data source.
	if o.fetched {
		return iocloser.NewReadCloser(&o.buf, nil), nil
	}
	if o.bcs == nil {
		return nil, io.EOF
	}

	// Open a lazy reader over the stored data blob.
	blk, err := block.UnmarshalBlock[*EncodedObject](o.r.ctx, o.bcs, NewEncodedObjectBlock)
	if err != nil {
		return nil, err
	}
	br, err := blk.BuildDataBlobReader(o.r.ctx, o.bcs)
	if err != nil {
		return nil, err
	}
	return br, nil
}

// Writer returns the data writer.
func (o *StoreEncodedObject) Writer() (io.WriteCloser, error) {
	o.fetched = true
	return iocloser.NewWriteCloser(&o.buf, nil), nil
}

// buildEncodedObjectKey builds the key for an encoded object.
func (r *Store) buildEncodedObjectKey(ot plumbing.ObjectType, h plumbing.Hash) ([]byte, error) {
	if ot == 0 || ot > 7 {
		return nil, errors.Wrapf(ErrObjectTypeInvalid, "%v", ot)
	}
	if h.IsZero() {
		return nil, errors.New("encoded object hash cannot be empty")
	}
	// Prefix the hash with its object type.
	return append([]byte{byte(ot)}, h.Bytes()...), nil //nolint:gosec
}

// unmarshalEncodedObject unmarshals the EncodedObject block.
// It returns nil, nil for an empty cursor.
func (o *StoreEncodedObject) unmarshalEncodedObject() (*EncodedObject, error) {
	return block.UnmarshalBlock[*EncodedObject](o.r.ctx, o.bcs, NewEncodedObjectBlock)
}

// lookupEncodedObject tries to build the EncodedObject from a key.
func (r *Store) lookupEncodedObject(key []byte) (*EncodedObject, *block.Cursor, error) {
	// Resolve the object cursor from its index key.
	encTree := r.objTree
	nodCs, err := encTree.GetCursorAtKey(r.ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if nodCs == nil {
		return nil, nil, plumbing.ErrObjectNotFound
	}

	// Decode and validate the object metadata at the cursor.
	encObji, err := nodCs.Unmarshal(r.ctx, NewEncodedObjectBlock)
	if err != nil {
		return nil, nil, err
	}
	encObjBlk, ok := encObji.(*EncodedObject)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return encObjBlk, nodCs, nil
}

// Compile-time interface assertions.
var (
	_ storer.EncodedObjectStorer = (*Store)(nil)
	_ plumbing.EncodedObject     = (*StoreEncodedObject)(nil)
)
